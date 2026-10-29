package main

import (
	"context"
	"net/url"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spareeye/internal/app"
	"spareeye/internal/detector"
)

// App is the surface bound into the webview. It holds no state of its own: the
// service layer owns the detector, the notifier, and the settings, and this
// type exists only to choose which of that is reachable from JavaScript.
type App struct {
	service *app.Service

	// windowContext is handed over by Wails at startup and is what the runtime
	// package needs to act on the window.
	windowContext context.Context
}

func NewApp(service *app.Service) *App {
	return &App{service: service}
}

func (a *App) startup(windowContext context.Context) {
	a.windowContext = windowContext
	a.service.Startup(windowContext)
}

func (a *App) shutdown(windowContext context.Context) {
	a.service.Shutdown(windowContext)
}

// Settings returns the saved configuration for the Setup and tuning screens.
func (a *App) Settings() app.SettingsView { return a.service.Settings() }

// Status is polled by the arm/disarm screen.
func (a *App) Status() app.StatusView { return a.service.Status() }

// Cameras lists capture devices confirmed to deliver frames.
func (a *App) Cameras() []detector.Camera { return a.service.Cameras() }

// RescanCameras looks for devices again, on the user's explicit request.
func (a *App) RescanCameras() []detector.Camera { return a.service.RescanCameras() }

// Every action returns an ActionResult rather than an error. A rejected Wails
// promise carries only the Go error's text, and that text is written for a log,
// not for the person who has to decide what to do next.
func (a *App) Arm() app.ActionResult    { return a.service.Arm() }
func (a *App) Disarm() app.ActionResult { return a.service.Disarm() }

func (a *App) SaveTelegram(form app.TelegramForm) app.ActionResult {
	return a.service.SaveTelegram(form)
}

func (a *App) TestTelegram(form app.TelegramForm) app.ActionResult {
	return a.service.TestTelegram(form)
}

func (a *App) SelectCamera(index int) app.ActionResult { return a.service.SelectCamera(index) }

func (a *App) TestCameraURL(address string) app.ActionResult {
	return a.service.TestCameraURL(address)
}

func (a *App) SelectCameraURL(address string) app.ActionResult {
	return a.service.SelectCameraURL(address)
}

func (a *App) SaveTuning(form app.TuningForm) app.ActionResult { return a.service.SaveTuning(form) }

func (a *App) SaveLiveView(form app.LiveViewForm) app.ActionResult {
	return a.service.SaveLiveView(form)
}

func (a *App) SaveStartup(form app.StartupForm) app.ActionResult {
	return a.service.SaveStartup(form)
}

func (a *App) StartManualRecording() app.ActionResult { return a.service.StartManualRecording() }
func (a *App) StopManualRecording() app.ActionResult  { return a.service.StopManualRecording() }

func (a *App) Clips() ([]app.RecordingView, error)     { return a.service.Clips() }
func (a *App) Snapshots() ([]app.RecordingView, error) { return a.service.Snapshots() }

func (a *App) DeleteRecording(path string) app.ActionResult { return a.service.DeleteRecording(path) }
func (a *App) SendRecording(path string) app.ActionResult   { return a.service.SendRecording(path) }

// RecentEvents returns the detector's event history, newest first.
func (a *App) RecentEvents() []app.EventView { return a.service.Events() }

// LogLines returns recent log records. A windowed app has no terminal, so this
// is the only way a user sees them without hunting for the file.
func (a *App) LogLines() []string { return a.service.LogLines() }

// OpenExternally hands a recording to the desktop's default player.
//
// Clips are written with the mp4v codec, which WebKit does not decode, so the
// in-app player cannot be the only way to watch one.
func (a *App) OpenExternally(path string) app.ActionResult {
	if !filepath.IsAbs(path) {
		return app.ActionResult{
			Headline: "That recording could not be opened",
			NextStep: "Press Refresh to reload the list, then try again.",
		}
	}

	runtime.BrowserOpenURL(a.windowContext, (&url.URL{Scheme: "file", Path: path}).String())

	return app.ActionResult{OK: true, Headline: "Opening in your video player"}
}
