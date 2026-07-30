// Package config persists user settings between runs. Everything the old
// build hardcoded as a Go constant lives here instead, so the UI can change
// detection behaviour without a recompile.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	applicationDirectoryName = "spareeye"
	legacyDirectoryName      = "kaa-rada"
	configFileName           = "config.json"
)

// Config is the complete persisted state of one installation.
type Config struct {
	Telegram  TelegramSettings  `json:"telegram"`
	Detection DetectionSettings `json:"detection"`
	Storage   StorageSettings   `json:"storage"`
	Startup   StartupSettings   `json:"startup"`
}

// StartupSettings decide what happens without anybody pressing anything.
//
// The two are independent because opening the app and pointing a camera at a
// room are separate commitments: somebody who wants the window waiting for them
// after a reboot has not thereby agreed to be recorded from the moment they log
// in.
type StartupSettings struct {
	// LaunchAtLogin is registered with the operating system rather than acted
	// on here, so the value only records what the user asked for. What the
	// system will actually do is whatever internal/autostart reports.
	LaunchAtLogin bool `json:"launchAtLogin"`

	// WatchOnLaunch arms the detector during startup, however the app was
	// started.
	WatchOnLaunch bool `json:"watchOnLaunch"`
}

// TelegramSettings holds credentials the user supplies through the UI. They are
// never compiled into the binary and never shipped in an installer.
type TelegramSettings struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatID"`
	Enabled  bool   `json:"enabled"`
}

// DetectionSettings are the tuning values a user can change without a rebuild.
type DetectionSettings struct {
	CameraIndex int `json:"cameraIndex"`

	// CameraURL is a network stream, RTSP or HTTP, and takes precedence over
	// CameraIndex when it is set. The two are separate fields rather than one
	// overloaded string because a single value would leave every reader
	// guessing whether "0" names a device or a file, and because a config.json
	// written before streams existed simply has no cameraURL key.
	CameraURL string `json:"cameraURL"`

	// CameraChosen records that somebody picked the camera above, as opposed to
	// its being the shipped default. Nothing is guessed on a first run: a laptop
	// whose index 0 is a virtual webcam would otherwise be watched by a device
	// that never sends a picture, and a household would have no idea which
	// camera the app had helped itself to.
	CameraChosen bool `json:"cameraChosen"`

	Day   SensitivityProfile `json:"day"`
	Night SensitivityProfile `json:"night"`

	// NightStart and NightEnd bound the night profile as minutes since
	// midnight, so a boundary that is not on the hour stays expressible.
	NightStartMinutes int `json:"nightStartMinutes"`
	NightEndMinutes   int `json:"nightEndMinutes"`

	// MotionPersist is how long motion must continue before a clip starts.
	MotionPersist Duration `json:"motionPersist"`

	// PrebufferLead is how much footage from *before* the trigger to prepend
	// to a clip. It is a duration rather than a frame count so that a "15
	// second" clip does not grow an unpredictable tail of history when the
	// camera runs faster than expected.
	PrebufferLead Duration `json:"prebufferLead"`

	ClipDuration Duration `json:"clipDuration"`

	// PreviewFPS caps how often a frame is re-encoded for the live view. It is
	// a ceiling, not a rate: the check happens inside the capture loop, so a
	// camera delivering 10 FPS is previewed at 10 FPS however high this is set.
	// Raising it past the camera's rate costs nothing; lowering it below saves
	// CPU at the cost of a choppier picture.
	PreviewFPS int `json:"previewFPS"`

	// PreviewQuality is the JPEG quality of live-view frames, 30 to 95. It
	// affects only the preview: clips and snapshots written to disk are always
	// encoded at full quality. Lower values reach the window sooner.
	PreviewQuality int `json:"previewQuality"`
}

// SensitivityProfile is one lighting condition's detection thresholds.
type SensitivityProfile struct {
	// MinimumContourArea is the pixel area a moving region must exceed to
	// count as motion. Higher values ignore smaller movement.
	MinimumContourArea float64 `json:"minimumContourArea"`

	// ThresholdValue is the per-pixel brightness change treated as movement.
	ThresholdValue float64 `json:"thresholdValue"`

	// SampleInterval is the delay between examined frames while idle.
	SampleInterval Duration `json:"sampleInterval"`

	// AlertCooldown is the minimum gap between snapshot alerts.
	AlertCooldown Duration `json:"alertCooldown"`
}

// StorageSettings locates generated media. It defaults to a per-user directory
// because an installed binary cannot write beside itself on Windows or macOS.
type StorageSettings struct {
	MediaDirectory string `json:"mediaDirectory"`

	// RetentionDays discards media older than this. Zero keeps everything,
	// which will fill the disk of anything left running for months.
	RetentionDays int `json:"retentionDays"`
}

// Duration is a time.Duration that serialises as a readable string such as
// "15s", keeping config.json editable by hand.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("duration must be a string like \"15s\": %w", err)
	}

	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("parsing duration %q: %w", text, err)
	}

	*d = Duration(parsed)
	return nil
}

func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

// Default returns the settings a fresh installation starts from.
func Default() Config {
	return Config{
		Telegram: TelegramSettings{Enabled: true},
		Detection: DetectionSettings{
			CameraIndex: 0,
			Day: SensitivityProfile{
				MinimumContourArea: 8000,
				ThresholdValue:     30,
				SampleInterval:     Duration(time.Second),
				AlertCooldown:      Duration(5 * time.Minute),
			},
			Night: SensitivityProfile{
				MinimumContourArea: 9000,
				ThresholdValue:     30,
				SampleInterval:     Duration(time.Second),
				AlertCooldown:      Duration(5 * time.Minute),
			},
			// A round evening default rather than one person's actual routine:
			// a shipped default is public, and an oddly specific one advertises
			// when whoever chose it is usually out.
			NightStartMinutes: 19 * 60,
			NightEndMinutes:   6 * 60,
			MotionPersist:     Duration(5 * time.Second),
			PrebufferLead:     Duration(3 * time.Second),
			ClipDuration:      Duration(15 * time.Second),
			PreviewFPS:        12,
			PreviewQuality:    80,
		},
		Storage: StorageSettings{RetentionDays: 30},
		// Both startup choices stay off until they are asked for. A fresh
		// install that registered itself with the operating system, or reached
		// for the webcam on its own, would be doing something nobody agreed to.
		Startup: StartupSettings{LaunchAtLogin: false, WatchOnLaunch: false},
	}
}

// Bounds on the live-view settings. They live here rather than in the detector
// because both the engine and the settings screen have to agree on them, and
// duplicating the numbers in two packages is how they drift apart.
const (
	MinPreviewFPS     = 1
	MaxPreviewFPS     = 60
	MinPreviewQuality = 30
	MaxPreviewQuality = 95
)

// Directory returns the per-user configuration directory, creating it if needed.
func Directory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config directory: %w", err)
	}

	directory := filepath.Join(base, applicationDirectoryName)
	legacyDirectory := filepath.Join(base, legacyDirectoryName)

	// The product rename must not make an upgrade look like a fresh install.
	// Move the complete old directory so credentials, recordings, and custom
	// settings remain together. A new SpareEye directory always wins, making
	// this migration both one-time and safe if somebody deliberately keeps the
	// legacy directory as a backup.
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyDirectory); legacyErr == nil {
			if err := os.Rename(legacyDirectory, directory); err != nil {
				return "", fmt.Errorf("migrating %s to %s: %w", legacyDirectory, directory, err)
			}
		} else if !errors.Is(legacyErr, os.ErrNotExist) {
			return "", fmt.Errorf("checking legacy config directory %s: %w", legacyDirectory, legacyErr)
		}
	} else if err != nil {
		return "", fmt.Errorf("checking config directory %s: %w", directory, err)
	}

	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("creating %s: %w", directory, err)
	}

	return directory, nil
}

// Load reads saved settings, returning defaults on a first run. Values absent
// from the file keep their default, so an older config.json stays loadable.
func Load() (Config, error) {
	directory, err := Directory()
	if err != nil {
		return Config{}, err
	}

	contents, err := os.ReadFile(filepath.Join(directory, configFileName))
	if errors.Is(err, os.ErrNotExist) {
		return withResolvedPaths(Default())
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	settings := Default()
	if err := json.Unmarshal(contents, &settings); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	// The live-view bounds are enforced here so that a hand-edited file heals
	// itself. The settings screen rejects an out-of-range value, and a stored
	// zero would otherwise make it refuse every save, including saves of
	// unrelated fields, over a control the user may never have opened.
	settings.Detection.PreviewFPS = clamp(settings.Detection.PreviewFPS, MinPreviewFPS, MaxPreviewFPS)
	settings.Detection.PreviewQuality = clamp(settings.Detection.PreviewQuality, MinPreviewQuality, MaxPreviewQuality)

	// An installation that predates being asked which camera to use has an index
	// somebody has been running with, and sending it back to the Setup screen to
	// re-pick the device it is already watching would be a regression dressed up
	// as a prompt.
	if !mentionsCameraChoice(contents) {
		settings.Detection.CameraChosen = true
	}

	// Before the product rename, the resolved default was persisted as an
	// absolute path. Directory has already moved that tree to SpareEye, so keep
	// the saved value in step or withResolvedPaths would recreate the old tree
	// and make the migrated recordings appear to be missing. Custom locations
	// are deliberately left alone.
	legacyMediaDirectory := filepath.Join(filepath.Dir(directory), legacyDirectoryName, "media")
	if filepath.Clean(settings.Storage.MediaDirectory) == filepath.Clean(legacyMediaDirectory) {
		settings.Storage.MediaDirectory = filepath.Join(directory, "media")
	}

	return withResolvedPaths(settings)
}

// mentionsCameraChoice reports whether the file carries the cameraChosen key at
// all, which is the only way to tell an upgrade from a fresh install that has
// saved something else, a bot token say, before choosing a camera.
func mentionsCameraChoice(contents []byte) bool {
	var probe struct {
		Detection struct {
			CameraChosen *bool `json:"cameraChosen"`
		} `json:"detection"`
	}

	if err := json.Unmarshal(contents, &probe); err != nil {
		return false
	}

	return probe.Detection.CameraChosen != nil
}

func clamp(value, lowest, highest int) int {
	if value < lowest {
		return lowest
	}
	if value > highest {
		return highest
	}
	return value
}

// Save writes settings back to disk with owner-only permissions, since the file
// contains a bot token.
func Save(settings Config) error {
	directory, err := Directory()
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temporary file and rename, so an interrupted save cannot
	// leave the user with a truncated config and no credentials.
	path := filepath.Join(directory, configFileName)
	temporary := path + ".tmp"

	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}

	return nil
}

func withResolvedPaths(settings Config) (Config, error) {
	if settings.Storage.MediaDirectory == "" {
		directory, err := Directory()
		if err != nil {
			return Config{}, err
		}
		settings.Storage.MediaDirectory = filepath.Join(directory, "media")
	}

	for _, sub := range []string{"snapshots", "videos"} {
		if err := os.MkdirAll(filepath.Join(settings.Storage.MediaDirectory, sub), 0o700); err != nil {
			return Config{}, fmt.Errorf("creating media directory: %w", err)
		}
	}

	return settings, nil
}

// SnapshotDirectory and VideoDirectory locate generated media on disk.
func (c Config) SnapshotDirectory() string {
	return filepath.Join(c.Storage.MediaDirectory, "snapshots")
}

func (c Config) VideoDirectory() string {
	return filepath.Join(c.Storage.MediaDirectory, "videos")
}

// ConfiguredForAlerts reports whether Telegram can actually be used, which the
// UI checks before offering to arm the camera.
func (c Config) ConfiguredForAlerts() bool {
	return c.Telegram.Enabled && c.Telegram.BotToken != "" && c.Telegram.ChatID != ""
}
