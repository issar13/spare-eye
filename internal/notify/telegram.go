package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const telegramAPIRoot = "https://api.telegram.org"

// Telegram delivers alerts through a user-supplied bot. Each installation uses
// its own bot token and its own chat, so one user's bot never reaches another's.
type Telegram struct {
	botToken   string
	chatID     string
	apiRoot    string
	httpClient *http.Client

	lastUpdateID int
}

// NewTelegram returns a notifier for the given bot credentials. It does not
// contact Telegram; call Verify to confirm the credentials work.
//
// Credentials are trimmed because they arrive from a file or environment a user
// edited by hand. A .env written on Windows leaves a carriage return on every
// value, which is invisible in an editor but makes the request URL invalid.
func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		botToken:   strings.TrimSpace(botToken),
		chatID:     strings.TrimSpace(chatID),
		apiRoot:    telegramAPIRoot,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *Telegram) Name() string { return "Telegram" }

func (t *Telegram) Close() error { return nil }

func (t *Telegram) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", t.apiRoot, t.botToken, method)
}

// DiscoverChatID finds the conversation that has messaged this bot. This keeps
// Telegram's internal numeric identifier out of setup: the person only needs to
// open their new bot, press Start, and return to Kaa Rada. Refusing an ambiguous
// result is important; silently choosing one of several chats could send camera
// footage to the wrong person or group.
func (t *Telegram) DiscoverChatID(ctx context.Context) (string, error) {
	updates, err := t.fetchUpdates(ctx)
	if err != nil {
		return "", err
	}

	chats := make(map[string]struct{})
	for _, update := range updates {
		if id := update.Message.Chat.ID.String(); id != "" {
			chats[id] = struct{}{}
		}
	}

	if len(chats) == 0 {
		return "", fmt.Errorf("no Telegram conversation found")
	}
	if len(chats) > 1 {
		return "", fmt.Errorf("multiple Telegram conversations found")
	}

	for id := range chats {
		t.chatID = id
		return id, nil
	}
	return "", fmt.Errorf("no Telegram conversation found")
}

// Verify confirms the token is valid and the chat is reachable, so the UI can
// report a bad setup immediately rather than silently failing at the first alert.
func (t *Telegram) Verify(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint("getMe"), nil)
	if err != nil {
		return err
	}

	response, err := t.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("reaching Telegram: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("bot token rejected by Telegram")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram returned status %d", response.StatusCode)
	}

	return t.Send(ctx, "✅ Motion detector connected", nil)
}

func (t *Telegram) Send(ctx context.Context, message string, attachment *Attachment) error {
	if attachment == nil {
		return t.sendText(ctx, message)
	}
	return t.sendAttachment(ctx, message, *attachment)
}

func (t *Telegram) sendText(ctx context.Context, message string) error {
	payload, err := json.Marshal(map[string]string{
		"chat_id": t.chatID,
		"text":    message,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	return t.doAndDiscard(request)
}

func (t *Telegram) sendAttachment(ctx context.Context, caption string, attachment Attachment) error {
	file, err := os.Open(attachment.Path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", attachment.Path, err)
	}
	defer file.Close()

	method, formField := "sendPhoto", "photo"
	if attachment.Kind == AttachmentVideo {
		method, formField = "sendVideo", "video"
	}

	// Telegram requires multipart upload for media, so the file is streamed
	// into a buffer the request body reads from.
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)

	part, err := form.CreateFormFile(formField, filepath.Base(attachment.Path))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("reading %s: %w", attachment.Path, err)
	}
	if err := form.WriteField("chat_id", t.chatID); err != nil {
		return err
	}
	if caption != "" {
		if err := form.WriteField("caption", caption); err != nil {
			return err
		}
	}
	if err := form.Close(); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(method), body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())

	return t.doAndDiscard(request)
}

func (t *Telegram) doAndDiscard(request *http.Request) error {
	response, err := t.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("Telegram status %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}

	io.Copy(io.Discard, response.Body)
	return nil
}

// Commands polls Telegram for inbound messages until ctx is cancelled. Messages
// originating from any chat other than the configured one are discarded: the bot
// token alone must not be enough to drive someone's camera.
func (t *Telegram) Commands(ctx context.Context) (<-chan Incoming, error) {
	incoming := make(chan Incoming)

	go func() {
		defer close(incoming)

		for ctx.Err() == nil {
			updates, err := t.fetchUpdates(ctx)
			if err != nil {
				// Network blips are expected; back off and retry.
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}

			for _, update := range updates {
				if update.Message.Chat.ID.String() != t.chatID {
					continue
				}
				parsed, recognised := parseCommand(update.Message.Text)
				if !recognised {
					continue
				}
				select {
				case incoming <- parsed:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return incoming, nil
}

// flexibleID accepts Telegram chat IDs whether the API encodes them as a JSON
// number or a string, so comparison against the configured value is reliable.
type flexibleID struct {
	value string
}

func (f *flexibleID) UnmarshalJSON(data []byte) error {
	f.value = strings.Trim(string(data), `"`)
	return nil
}

func (f flexibleID) String() string { return f.value }

type telegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Text string `json:"text"`
		Chat struct {
			ID flexibleID `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func (t *Telegram) fetchUpdates(ctx context.Context) ([]telegramUpdate, error) {
	url := fmt.Sprintf("%s?offset=%d&timeout=25", t.endpoint("getUpdates"), t.lastUpdateID+1)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	response, err := t.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var payload struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("Telegram reported not-ok")
	}

	// Advance past every update we were handed, including ones we discard, so
	// a message from a stranger cannot wedge the offset and stall the poller.
	for _, update := range payload.Result {
		if update.UpdateID > t.lastUpdateID {
			t.lastUpdateID = update.UpdateID
		}
	}

	return payload.Result, nil
}

func parseCommand(text string) (Incoming, bool) {
	// Whitespace has to go before the slash prefix: trimming the prefix first
	// leaves the slash intact on " /record" and nothing would match.
	normalised := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(text), "/"))

	switch normalised {
	case "record":
		return Incoming{Command: CommandStartRecording}, true
	case "stop":
		return Incoming{Command: CommandStopRecording}, true
	case "get", "list":
		return Incoming{Command: CommandListRecordings}, true
	}

	if index, err := strconv.Atoi(normalised); err == nil {
		return Incoming{Selection: &Selection{Index: index}}, true
	}

	return Incoming{}, false
}
