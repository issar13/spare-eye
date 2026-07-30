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
// is written for whoever reads the log, as in "opening camera 1: VideoCapture
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

// noCameraChosen is what a first run says. The app deliberately does not pick
// for the user: the lowest working index on a laptop is as often a virtual
// webcam as the built-in one, and a security camera nobody chose is watching a
// room nobody agreed to have watched.
func noCameraChosen() Problem {
	return Problem{
		Headline: "You have not chosen a camera yet",
		NextStep: "Go to Setup and pick one from the list, or enter the address of a camera on your network.",
	}
}

// chosenCameraMissing is what a camera that used to work says once it stops.
// Quietly moving to a different one would point the app at another room without
// anybody being told.
func chosenCameraMissing() Problem {
	return Problem{
		Headline: "The camera you chose is not responding",
		NextStep: "It may have been unplugged, or another program may have it. Close Zoom, Teams or OBS, " +
			"then go to Setup and press Look again. You can also pick a different camera there.",
	}
}

// streamUnavailable covers every way a network camera can refuse. The reasons
// (wrong address, wrong password, camera asleep, nothing at that address) are
// not worth telling apart, because the backend reports most of them as the same
// silence and the first thing to check is the same in each case.
func streamUnavailable() Problem {
	return Problem{
		Headline: "That camera did not send a picture",
		NextStep: "Check the address, and that any user name and password in it are right. Make sure the " +
			"camera is switched on and this computer is on the same network as it.",
	}
}

// loginRegistrationRefused is the guidance for an operating system that would
// not take, or would not give up, the instruction to start this app at login.
//
// The reasons are not worth telling apart for a user: security software
// blocking the change and a locked-down account look the same from here, and
// the remedy, do it yourself or allow the app and try again, is the same.
func loginRegistrationRefused(wanted bool) Problem {
	if wanted {
		return Problem{
			Headline: "Kaa Rada could not be set to open by itself",
			NextStep: "Your choices were saved, but this computer would not let Kaa Rada add itself to " +
				"the programs that open when you log in. Security software often blocks that. Open Kaa " +
				"Rada yourself for now, and try the switch again once you have allowed it.",
		}
	}

	return Problem{
		Headline: "Kaa Rada could not remove itself from your login programs",
		NextStep: "Your choices were saved, but Kaa Rada may still open when you log in. Turn it off in " +
			"your computer's own startup settings, or try the switch again.",
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
	case strings.Contains(message, "bot token rejected"),
		strings.Contains(message, "Telegram returned status"),
		strings.Contains(message, "Telegram reported not-ok"):
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
			NextStep: "Check the chat ID, and make sure you have sent your bot at least one message first. " +
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
