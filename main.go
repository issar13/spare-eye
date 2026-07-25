// Command kaa-rada is the desktop build. The headless engine lives in
// ./cmd/kaa-rada-cli; this package exists at the repository root because Wails v2
// compiles the main package it finds there.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"kaa-rada/internal/app"
	"kaa-rada/internal/config"
	"kaa-rada/internal/detector"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}

	directory, err := config.Directory()
	if err != nil {
		return err
	}

	log, err := app.OpenLog(directory)
	if err != nil {
		return err
	}
	defer log.Close()

	log.Info("starting", "mediaDirectory", settings.Storage.MediaDirectory, "camera", settings.Detection.CameraIndex)

	// The claim is taken before the window opens, but a failure does not stop
	// the app: a window that explains why it cannot arm is more useful than a
	// process that exits with a message nobody sees.
	instanceLock, lockError := detector.AcquireInstanceLock(directory)
	if lockError == nil {
		defer instanceLock.Release()
	}

	service := app.NewService(log, settings, lockError)
	bound := NewApp(service)

	return wails.Run(&options.App{
		Title:  "Kaa Rada",
		Width:  1180,
		Height: 820,
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
