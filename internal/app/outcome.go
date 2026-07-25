package app

import (
	"strings"
)

// telegramSetupDoc is named in guidance whenever credentials are the problem.
// It is a path in the repository rather than a URL because the desktop build
// has no browser of its own and a user who cloned this project can open it.
const telegramSetupDoc = "docs/SETUP-TELEGRAM.md"

// ActionResult is what every UI action reports back.
//
// Actions deliberately do not return a Go error to the webview. An error's text
// is written for whoever reads the log — "opening camera 1: VideoCapture
// backend failed" tells a user nothing they can act on. Every failure here
// carries a headline in plain words and the next thing to try.
type ActionResult struct {
	OK       bool   `json:"ok"`
	Headline string `json:"headline"`
	NextStep string `json:"nextStep"`
	DocPath  string `json:"docPath"`
}

// Problem is a failure described for a person: what went wrong, and what to do.
type Problem struct {
	Headline string `json:"headline"`
	NextStep string `json:"nextStep"`
	DocPath  string `json:"docPath"`
}

func succeeded(headline string) ActionResult {
	return ActionResult{OK: true, Headline: headline}
}

func failed(headline, nextStep string) ActionResult {
	return ActionResult{Headline: headline, NextStep: nextStep}
}

func (r ActionResult) withDoc(path string) ActionResult {
	r.DocPath = path
	return r
}

func (p Problem) asResult() ActionResult {
	return ActionResult{Headline: p.Headline, NextStep: p.NextStep, DocPath: p.DocPath}
}

// cameraUnavailable is the guidance for every way a camera can refuse to work.
// They are not worth distinguishing to a user: the remedy is the same, and
// OpenCV reports a busy device and an absent one almost identically anyway.
func cameraUnavailable() Problem {
	return Problem{
		Headline: "Your camera is not available",
		NextStep: "Another program is probably using it. Close Zoom, Teams, OBS, or a second copy of " +
			"Kaa Rada, then try again. If you use a virtual webcam such as Iriun, start it first " +
			"or choose a different camera in Setup.",
	}
}

// describeTelegramFailure turns the notify package's error into advice.
//
// The match is on message text because notify returns plain fmt.Errorf values,
// so there is nothing typed to switch on. This is the one place that reads
// them, and it is the boundary where a third party's API becomes something a
// user is asked to fix.
func describeTelegramFailure(failure error) Problem {
	message := failure.Error()

	switch {
	case strings.Contains(message, "bot token rejected"):
		return Problem{
			Headline: "Telegram would not accept that bot token",
			NextStep: "Open Telegram, message @BotFather, and copy the token again. It looks like " +
				"123456789:AAE… Make sure nothing extra was copied with it.",
			DocPath: telegramSetupDoc,
		}

	case strings.Contains(message, "reaching Telegram"):
		return Problem{
			Headline: "Could not reach Telegram",
			NextStep: "This computer does not seem to be online. Check your internet connection and try again.",
		}

	case strings.Contains(message, "chat not found"), strings.Contains(message, "status 400"):
		return Problem{
			Headline: "Telegram could not find that chat",
			NextStep: "Check the chat ID, and make sure you have sent your bot at least one message first — " +
				"a bot cannot start the conversation. @userinfobot will tell you your chat ID.",
			DocPath: telegramSetupDoc,
		}

	case strings.Contains(message, "status 403"):
		return Problem{
			Headline: "Your bot is not allowed to message that chat",
			NextStep: "Open the chat with your bot in Telegram and send it /start, then test again.",
			DocPath:  telegramSetupDoc,
		}
	}

	return Problem{
		Headline: "Telegram did not respond as expected",
		NextStep: "Check the bot token and chat ID, then try again.",
		DocPath:  telegramSetupDoc,
	}
}
