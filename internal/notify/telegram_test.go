package notify

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseCommandRecognisesBothPrefixedAndBareForms(t *testing.T) {
	cases := []struct {
		input      string
		expect     Command
		recognised bool
	}{
		{"/record", CommandStartRecording, true},
		{"record", CommandStartRecording, true},
		{"RECORD", CommandStartRecording, true},
		{"  /Record  ", CommandStartRecording, true},
		{"/stop", CommandStopRecording, true},
		{"stop", CommandStopRecording, true},
		{"/get", CommandListRecordings, true},
		{"list", CommandListRecordings, true},
		{"hello", "", false},
		{"", "", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.input, func(t *testing.T) {
			parsed, recognised := parseCommand(testCase.input)

			if recognised != testCase.recognised {
				t.Fatalf("parseCommand(%q) recognised = %v, want %v",
					testCase.input, recognised, testCase.recognised)
			}
			if recognised && parsed.Command != testCase.expect {
				t.Errorf("parseCommand(%q) = %q, want %q",
					testCase.input, parsed.Command, testCase.expect)
			}
		})
	}
}

// TestNewTelegramTrimsCredentials covers a .env written on Windows: the trailing
// carriage return on every value is invisible in an editor but produces
// "invalid control character in URL" on the first request.
func TestNewTelegramTrimsCredentials(t *testing.T) {
	subject := NewTelegram("8310685887:EXAMPLE\r\n", " 219875764\r")

	if strings.ContainsAny(subject.botToken, "\r\n ") {
		t.Errorf("bot token retained whitespace: %q", subject.botToken)
	}
	if subject.chatID != "219875764" {
		t.Errorf("chat ID = %q, want %q", subject.chatID, "219875764")
	}

	endpoint := subject.endpoint("getMe")
	if strings.ContainsAny(endpoint, "\r\n") {
		t.Errorf("endpoint contains a control character: %q", endpoint)
	}
	if _, err := url.Parse(endpoint); err != nil {
		t.Errorf("endpoint did not parse as a URL: %v", err)
	}
}

func TestParseCommandTreatsDigitsAsSelection(t *testing.T) {
	parsed, recognised := parseCommand("3")

	if !recognised {
		t.Fatal("a bare number should be recognised as a listing selection")
	}
	if parsed.Selection == nil {
		t.Fatal("selection was not populated")
	}
	if parsed.Selection.Index != 3 {
		t.Errorf("selection index = %d, want 3", parsed.Selection.Index)
	}
	if parsed.Command != "" {
		t.Errorf("selection should not also carry a command, got %q", parsed.Command)
	}
}

// TestFlexibleIDAcceptsNumberOrString covers the reason the chat authentication
// check needs a custom type: Telegram encodes chat IDs inconsistently, and a
// mismatch here would silently disable the check that keeps strangers out.
func TestFlexibleIDAcceptsNumberOrString(t *testing.T) {
	cases := map[string]string{
		`219875764`:   "219875764",
		`"219875764"`: "219875764",
		`-1001234567`: "-1001234567",
	}

	for input, want := range cases {
		var identifier flexibleID
		if err := identifier.UnmarshalJSON([]byte(input)); err != nil {
			t.Fatalf("UnmarshalJSON(%s) returned %v", input, err)
		}
		if identifier.String() != want {
			t.Errorf("UnmarshalJSON(%s) = %q, want %q", input, identifier.String(), want)
		}
	}
}
