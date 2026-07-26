// Command kaa-rada is the desktop build. The headless engine lives in
// ./cmd/kaa-rada-cli; this package exists at the repository root because Wails v2
// compiles the main package it finds there.
package main

import (
	"embed"
	"fmt"
	"os"
	"slices"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"kaa-rada/internal/app"
	"kaa-rada/internal/autostart"
	"kaa-rada/internal/config"
	"kaa-rada/internal/detector"
)

//go:embed all:frontend/dist
var assets embed.FS

// main opens the log before any startup work, then reports whatever that work
// does to both the log and the user.
//
// The order matters on Windows, where this binary is linked for the GUI
// subsystem: os.Stderr is not connected to anything, so a failure before the
// window exists is invisible unless it has already been given a file to go to.
func main() {
	// Scan mode is answered before anything else, including the log. This
	// process is a child of another copy of itself, spawned only to open capture
	// devices somewhere a driver can end it harmlessly; it reports through
	// stdout, and appending to the log file its parent already holds open would
	// interleave two processes' records in one file.
	if startIndex, scanning := detector.CameraScanStart(os.Args[1:]); scanning {
		detector.WriteCameraScan(os.Stdout, startIndex)
		return
	}

	if address, testing := detector.CameraTestSource(os.Args[1:]); testing {
		detector.WriteCameraTest(os.Stdout, address)
		return
	}

	log, err := app.OpenEarlyLog()
	if err != nil {
		app.ReportFatalError(err.Error(), "")
		os.Exit(1)
	}
	defer log.Close()

	// Deferred before run so it unwinds a panic from any stage below, including
	// one raised inside Wails while the window is being created.
	defer func() {
		if panicked := recover(); panicked != nil {
			log.Panicked("desktop startup", panicked)
			app.ReportFatalError(fmt.Sprintf("Kaa Rada stopped unexpectedly: %v", panicked), log.Path())
			os.Exit(1)
		}
	}()

	if err := run(log); err != nil {
		log.Error("startup failed", "error", err)
		app.ReportFatalError(err.Error(), log.Path())
		os.Exit(1)
	}
}

// startedAtLogin reports whether the operating system launched this process,
// which the autostart registration signals by adding a flag to the command it
// registers.
//
// The arguments are scanned rather than parsed with the flag package, because
// flag.Parse exits the process on anything it does not recognise and a webview
// host is entitled to pass arguments of its own.
func startedAtLogin() bool {
	return slices.Contains(os.Args[1:], autostart.StartedAtLoginFlag)
}

// run carries out startup, logging each stage as it is reached. The stages are
// what makes a crash inside a dependency (OpenCV aborting during a camera
// probe, a webview refusing to initialise) identifiable from the last record
// in the log, which is all a native crash leaves behind.
func run(log *app.Log) error {
	log.Info("startup: loading settings")

	settings, err := config.Load()
	if err != nil {
		return err
	}

	directory, err := config.Directory()
	if err != nil {
		return err
	}

	log.Info("startup: settings loaded",
		"configDirectory", directory,
		"mediaDirectory", settings.Storage.MediaDirectory,
		"camera", settings.Detection.CameraIndex)

	// The claim is taken before the window opens, but a failure does not stop
	// the app: a window that explains why it cannot arm is more useful than a
	// process that exits with a message nobody sees.
	instanceLock, lockError := detector.AcquireInstanceLock(directory)
	if lockError == nil {
		defer instanceLock.Release()
	}

	service := app.NewService(log, settings, lockError)
	bound := NewApp(service)

	// A window that steals the screen during every login is the fastest way to
	// have the feature turned off again, so a session that started this app
	// begins in the taskbar. Somebody who opened it themselves is looking at it
	// on purpose.
	startedByTheSystem := startedAtLogin()

	windowStartState := options.Normal
	if startedByTheSystem {
		windowStartState = options.Minimised
	}

	log.Info("startup: opening the window", "startedAtLogin", startedByTheSystem)

	return wails.Run(&options.App{
		Title:            "Kaa Rada",
		Width:            1180,
		Height:           820,
		WindowStartState: windowStartState,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: service.AssetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 17, G: 19, B: 24, A: 1},
		OnStartup:        bound.startup,
		OnShutdown:       bound.shutdown,
		Bind:             []interface{}{bound},
	})
}
