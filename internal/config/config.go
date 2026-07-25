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
	applicationDirectoryName = "kaa-rada"
	configFileName           = "config.json"
)

// Config is the complete persisted state of one installation.
type Config struct {
	Telegram  TelegramSettings  `json:"telegram"`
	Detection DetectionSettings `json:"detection"`
	Storage   StorageSettings   `json:"storage"`
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
		},
		Storage: StorageSettings{RetentionDays: 30},
	}
}

// Directory returns the per-user configuration directory, creating it if needed.
func Directory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config directory: %w", err)
	}

	directory := filepath.Join(base, applicationDirectoryName)
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

	return withResolvedPaths(settings)
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
