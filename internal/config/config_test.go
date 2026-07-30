package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// isolateConfigDirectory points os.UserConfigDir at a per-test temporary
// directory. Every test touching Directory, Load or Save must call this first:
// without it the tests would read and overwrite the developer's real
// ~/.config/spareeye/config.json, destroying a saved bot token.
func isolateConfigDirectory(t *testing.T) string {
	t.Helper()

	temporaryHome := t.TempDir()

	// os.UserConfigDir reads a different variable per platform, and setting only
	// the Unix one let the Windows tests write to the real roaming profile.
	// Setting both keeps this helper honest everywhere.
	t.Setenv("XDG_CONFIG_HOME", temporaryHome)
	t.Setenv("AppData", temporaryHome)

	return temporaryHome
}

func TestDirectoryStaysInsideTheIsolatedConfigHome(t *testing.T) {
	// This test exists to prove the isolation helper actually works. If it ever
	// fails, every other test in this file is writing to the real home
	// directory and must not be trusted.
	temporaryHome := isolateConfigDirectory(t)

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}

	if !strings.HasPrefix(directory, temporaryHome) {
		t.Fatalf("Directory() = %q, which is outside the temporary home %q",
			directory, temporaryHome)
	}
	if filepath.Base(directory) != applicationDirectoryName {
		t.Errorf("Directory() = %q, want a directory named %q",
			directory, applicationDirectoryName)
	}

	info, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("Directory() did not create the directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("Directory() did not create a directory")
	}
}

func TestDirectoryMigratesTheLegacyProductDirectory(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)
	legacyDirectory := filepath.Join(temporaryHome, legacyDirectoryName)
	legacyMedia := filepath.Join(legacyDirectory, "media")
	if err := os.MkdirAll(filepath.Join(legacyMedia, "videos"), 0o700); err != nil {
		t.Fatalf("creating legacy directory: %v", err)
	}

	legacyConfig := []byte(fmt.Sprintf(`{"telegram":{"botToken":"kept"},"storage":{"mediaDirectory":%q}}`, legacyMedia))
	if err := os.WriteFile(filepath.Join(legacyDirectory, configFileName), legacyConfig, 0o600); err != nil {
		t.Fatalf("writing legacy config: %v", err)
	}
	recordingName := "motion-kept.mp4"
	if err := os.WriteFile(filepath.Join(legacyMedia, "videos", recordingName), []byte("recording"), 0o600); err != nil {
		t.Fatalf("writing legacy recording: %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}
	directory := filepath.Join(temporaryHome, applicationDirectoryName)

	contents, err := os.ReadFile(filepath.Join(directory, configFileName))
	if err != nil {
		t.Fatalf("reading migrated config: %v", err)
	}
	if string(contents) != string(legacyConfig) {
		t.Errorf("migrated config = %q, want %q", contents, legacyConfig)
	}
	if _, err := os.Stat(legacyDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("legacy directory still exists after migration: %v", err)
	}
	wantMedia := filepath.Join(directory, "media")
	if settings.Storage.MediaDirectory != wantMedia {
		t.Errorf("media directory = %q, want migrated path %q", settings.Storage.MediaDirectory, wantMedia)
	}
	if _, err := os.Stat(filepath.Join(settings.VideoDirectory(), recordingName)); err != nil {
		t.Errorf("migrated recording is not visible through loaded settings: %v", err)
	}
}

func TestLoadDoesNotRebaseAnExternalMediaDirectory(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)
	externalMedia := filepath.Join(t.TempDir(), "camera-media")
	directory := filepath.Join(temporaryHome, applicationDirectoryName)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("creating config directory: %v", err)
	}
	contents := []byte(fmt.Sprintf(`{"storage":{"mediaDirectory":%q}}`, externalMedia))
	if err := os.WriteFile(filepath.Join(directory, configFileName), contents, 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}
	if settings.Storage.MediaDirectory != externalMedia {
		t.Errorf("external media directory = %q, want %q", settings.Storage.MediaDirectory, externalMedia)
	}
}

func TestDurationMarshalsAsAHumanReadableString(t *testing.T) {
	// Duration exists only so config.json stays hand-editable, so the encoded
	// form has to be the readable string rather than a nanosecond count.
	cases := []struct {
		name     string
		duration Duration
		expected string
	}{
		{"seconds", Duration(15 * time.Second), `"15s"`},
		{"minutes", Duration(5 * time.Minute), `"5m0s"`},
		{"sub second", Duration(250 * time.Millisecond), `"250ms"`},
		{"zero", Duration(0), `"0s"`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.duration)
			if err != nil {
				t.Fatalf("Marshal returned %v", err)
			}
			if string(encoded) != testCase.expected {
				t.Errorf("Marshal(%v) = %s, want %s",
					time.Duration(testCase.duration), encoded, testCase.expected)
			}
		})
	}
}

func TestDurationUnmarshalsTheStringsItProduces(t *testing.T) {
	cases := []struct {
		name     string
		encoded  string
		expected time.Duration
	}{
		{"seconds", `"15s"`, 15 * time.Second},
		{"minutes", `"5m0s"`, 5 * time.Minute},
		{"compound", `"1h30m"`, 90 * time.Minute},
		{"sub second", `"250ms"`, 250 * time.Millisecond},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var decoded Duration
			if err := json.Unmarshal([]byte(testCase.encoded), &decoded); err != nil {
				t.Fatalf("Unmarshal(%s) returned %v", testCase.encoded, err)
			}
			if decoded.AsDuration() != testCase.expected {
				t.Errorf("Unmarshal(%s) = %v, want %v",
					testCase.encoded, decoded.AsDuration(), testCase.expected)
			}
		})
	}
}

func TestDurationRejectsValuesItCannotParse(t *testing.T) {
	// A hand-edited config.json is the expected source of these mistakes, so
	// the error has to surface rather than silently leaving a zero duration.
	cases := []struct {
		name    string
		encoded string
	}{
		{"a bare number is not a duration string", `15`},
		{"a float is not a duration string", `1.5`},
		{"a boolean is not a duration string", `true`},
		{"an object is not a duration string", `{"seconds":15}`},
		{"a string with no unit", `"15"`},
		{"an unrecognised unit", `"15 fortnights"`},
		{"an empty string", `""`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var decoded Duration
			if err := json.Unmarshal([]byte(testCase.encoded), &decoded); err == nil {
				t.Errorf("Unmarshal(%s) succeeded with %v, want an error",
					testCase.encoded, decoded.AsDuration())
			}
		})
	}
}

func TestDurationRoundTripsThroughJSON(t *testing.T) {
	original := Duration(90 * time.Second)

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal returned %v", err)
	}

	var decoded Duration
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal(%s) returned %v", encoded, err)
	}

	if decoded != original {
		t.Errorf("round trip produced %v, want %v",
			decoded.AsDuration(), original.AsDuration())
	}
}

// TestDefaultsAreTheDocumentedOnes pins every default that the README states, so
// the two cannot drift apart without a test failing.
func TestDefaultsAreTheDocumentedOnes(t *testing.T) {
	settings := Default()

	cases := []struct {
		name     string
		actual   time.Duration
		expected time.Duration
	}{
		{"day alert cooldown", settings.Detection.Day.AlertCooldown.AsDuration(), 5 * time.Minute},
		{"night alert cooldown", settings.Detection.Night.AlertCooldown.AsDuration(), 5 * time.Minute},
		{"day sample interval", settings.Detection.Day.SampleInterval.AsDuration(), time.Second},
		{"night sample interval", settings.Detection.Night.SampleInterval.AsDuration(), time.Second},
		{"motion persist", settings.Detection.MotionPersist.AsDuration(), 5 * time.Second},
		{"prebuffer lead", settings.Detection.PrebufferLead.AsDuration(), 3 * time.Second},
		{"clip duration", settings.Detection.ClipDuration.AsDuration(), 15 * time.Second},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.actual != testCase.expected {
				t.Errorf("%s = %v, want %v", testCase.name, testCase.actual, testCase.expected)
			}
		})
	}

	// The night window is 19:00 to 06:00, stored as minutes since midnight so
	// that a boundary off the hour stays expressible.
	if settings.Detection.NightStartMinutes != 19*60 {
		t.Errorf("night start = %d minutes, want %d", settings.Detection.NightStartMinutes, 19*60)
	}
	if settings.Detection.NightEndMinutes != 6*60 {
		t.Errorf("night end = %d minutes, want %d", settings.Detection.NightEndMinutes, 6*60)
	}

	// Night needs a higher contour area than day because sensor noise in low
	// light otherwise reads as movement.
	if settings.Detection.Night.MinimumContourArea <= settings.Detection.Day.MinimumContourArea {
		t.Errorf("night contour area %v should exceed day %v",
			settings.Detection.Night.MinimumContourArea, settings.Detection.Day.MinimumContourArea)
	}

	// Alerts are on by default but unusable until the user supplies credentials.
	if !settings.Telegram.Enabled {
		t.Error("Telegram should be enabled by default")
	}
	if settings.ConfiguredForAlerts() {
		t.Error("defaults carry no credentials, so alerts must not be considered configured")
	}

	// Defaults must never ship a token or chat ID compiled into the binary.
	if settings.Telegram.BotToken != "" || settings.Telegram.ChatID != "" {
		t.Errorf("defaults contain baked-in credentials: token=%q chatID=%q",
			settings.Telegram.BotToken, settings.Telegram.ChatID)
	}

	// The live view defaults are a compromise the README quotes: smooth enough to
	// read at a glance, cheap enough not to compete with detection for CPU.
	if settings.Detection.PreviewFPS != 12 {
		t.Errorf("preview FPS = %d, want 12", settings.Detection.PreviewFPS)
	}
	if settings.Detection.PreviewQuality != 80 {
		t.Errorf("preview quality = %d, want 80", settings.Detection.PreviewQuality)
	}

	if settings.Storage.RetentionDays != 30 {
		t.Errorf("retention = %d days, want 30", settings.Storage.RetentionDays)
	}

	// Default leaves the media directory empty; Load resolves it against the
	// per-user config directory.
	if settings.Storage.MediaDirectory != "" {
		t.Errorf("media directory = %q, want it unresolved in Default()",
			settings.Storage.MediaDirectory)
	}
}

func TestLoadOnAFirstRunReturnsDefaultsRatherThanAnError(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() on a first run returned %v, want defaults", err)
	}

	defaults := Default()
	if settings.Detection.ClipDuration != defaults.Detection.ClipDuration {
		t.Errorf("clip duration = %v, want the default %v",
			settings.Detection.ClipDuration.AsDuration(), defaults.Detection.ClipDuration.AsDuration())
	}
	if settings.Storage.RetentionDays != defaults.Storage.RetentionDays {
		t.Errorf("retention = %d, want the default %d",
			settings.Storage.RetentionDays, defaults.Storage.RetentionDays)
	}

	// A first run has to end up with a usable media directory, since nothing
	// else creates one.
	if !strings.HasPrefix(settings.Storage.MediaDirectory, temporaryHome) {
		t.Fatalf("media directory = %q, which is outside the temporary home %q",
			settings.Storage.MediaDirectory, temporaryHome)
	}
	for _, directory := range []string{settings.SnapshotDirectory(), settings.VideoDirectory()} {
		if _, err := os.Stat(directory); err != nil {
			t.Errorf("Load() did not create %s: %v", directory, err)
		}
	}
}

// TestLoadKeepsDefaultsForFieldsAbsentFromTheFile is the forward-compatibility
// guarantee: Load starts from Default() and unmarshals the file over it, so a
// config.json written by an older version stays loadable and newly added fields
// get their default instead of a zero value.
func TestLoadKeepsDefaultsForFieldsAbsentFromTheFile(t *testing.T) {
	isolateConfigDirectory(t)

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}

	partialConfig := `{
	  "telegram": {"botToken": "1234567890:EXAMPLE", "chatID": "987654321"},
	  "detection": {"clipDuration": "42s"}
	}`
	if err := os.WriteFile(filepath.Join(directory, configFileName), []byte(partialConfig), 0o600); err != nil {
		t.Fatalf("writing the partial config returned %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}

	// The values that were present must win.
	if settings.Telegram.BotToken != "1234567890:EXAMPLE" {
		t.Errorf("bot token = %q, want the value from the file", settings.Telegram.BotToken)
	}
	if settings.Detection.ClipDuration.AsDuration() != 42*time.Second {
		t.Errorf("clip duration = %v, want 42s from the file",
			settings.Detection.ClipDuration.AsDuration())
	}

	// Everything else must still be the default, not a zero value.
	defaults := Default()
	if settings.Detection.MotionPersist != defaults.Detection.MotionPersist {
		t.Errorf("motion persist = %v, want the default %v",
			settings.Detection.MotionPersist.AsDuration(), defaults.Detection.MotionPersist.AsDuration())
	}
	if settings.Detection.Day.AlertCooldown != defaults.Detection.Day.AlertCooldown {
		t.Errorf("day alert cooldown = %v, want the default %v",
			settings.Detection.Day.AlertCooldown.AsDuration(), defaults.Detection.Day.AlertCooldown.AsDuration())
	}
	if settings.Detection.Night.MinimumContourArea != defaults.Detection.Night.MinimumContourArea {
		t.Errorf("night contour area = %v, want the default %v",
			settings.Detection.Night.MinimumContourArea, defaults.Detection.Night.MinimumContourArea)
	}
	if settings.Detection.NightStartMinutes != defaults.Detection.NightStartMinutes {
		t.Errorf("night start = %d, want the default %d",
			settings.Detection.NightStartMinutes, defaults.Detection.NightStartMinutes)
	}
	if settings.Detection.MotionPersist != defaults.Detection.MotionPersist {
		t.Errorf("motion persist = %v, want the default %v",
			settings.Detection.MotionPersist, defaults.Detection.MotionPersist)
	}
	if settings.Storage.RetentionDays != defaults.Storage.RetentionDays {
		t.Errorf("retention = %d, want the default %d",
			settings.Storage.RetentionDays, defaults.Storage.RetentionDays)
	}

	// A config.json saved before the live view became configurable names neither
	// preview field. Landing on zero would give the settings screen a frame rate
	// it refuses to accept and a quality libjpeg treats as blank.
	if settings.Detection.PreviewFPS != defaults.Detection.PreviewFPS {
		t.Errorf("preview FPS = %d, want the default %d",
			settings.Detection.PreviewFPS, defaults.Detection.PreviewFPS)
	}
	if settings.Detection.PreviewQuality != defaults.Detection.PreviewQuality {
		t.Errorf("preview quality = %d, want the default %d",
			settings.Detection.PreviewQuality, defaults.Detection.PreviewQuality)
	}
}

func TestLoadReportsAConfigItCannotParse(t *testing.T) {
	isolateConfigDirectory(t)

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, configFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("writing the damaged config returned %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a damaged config, want an error so the user is told")
	}
}

func TestSaveThenLoadRoundTripsEverySetting(t *testing.T) {
	isolateConfigDirectory(t)

	original := Default()
	original.Telegram.BotToken = "1234567890:EXAMPLE"
	original.Telegram.ChatID = "987654321"
	original.Telegram.Enabled = false
	original.Detection.CameraIndex = 2
	original.Detection.NightStartMinutes = 20*60 + 15
	original.Detection.NightEndMinutes = 5 * 60
	original.Detection.MotionPersist = Duration(7 * time.Second)
	original.Detection.PrebufferLead = Duration(1500 * time.Millisecond)
	original.Detection.ClipDuration = Duration(90 * time.Second)
	original.Detection.Day.MinimumContourArea = 1234
	original.Detection.Day.SampleInterval = Duration(2 * time.Second)
	original.Detection.Night.AlertCooldown = Duration(90 * time.Minute)
	original.Detection.PreviewFPS = 24
	original.Detection.PreviewQuality = 45
	original.Storage.MediaDirectory = filepath.Join(t.TempDir(), "media")
	original.Storage.RetentionDays = 7
	original.Startup.LaunchAtLogin = true
	original.Startup.WatchOnLaunch = true

	if err := Save(original); err != nil {
		t.Fatalf("Save() returned %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}

	if reloaded != original {
		t.Errorf("round trip changed the settings:\n got %+v\nwant %+v", reloaded, original)
	}

	// Named checks for the nested Duration fields, because a whole-struct diff
	// is hard to read when only one of them is wrong.
	if reloaded.Detection.PrebufferLead.AsDuration() != 1500*time.Millisecond {
		t.Errorf("prebuffer lead = %v, want 1.5s", reloaded.Detection.PrebufferLead.AsDuration())
	}
	if reloaded.Detection.Night.AlertCooldown.AsDuration() != 90*time.Minute {
		t.Errorf("night alert cooldown = %v, want 1h30m", reloaded.Detection.Night.AlertCooldown.AsDuration())
	}

	// The live view dials are plain integers rather than Durations, so they are
	// easy to leave out of the encoded form without the whole-struct diff above
	// being obviously wrong about which field drifted.
	if reloaded.Detection.PreviewFPS != 24 {
		t.Errorf("preview FPS = %d, want 24", reloaded.Detection.PreviewFPS)
	}
	if reloaded.Detection.PreviewQuality != 45 {
		t.Errorf("preview quality = %d, want 45", reloaded.Detection.PreviewQuality)
	}

	if !reloaded.Startup.LaunchAtLogin || !reloaded.Startup.WatchOnLaunch {
		t.Errorf("startup settings = %+v, want both switched on", reloaded.Startup)
	}
}

// TestStartupIsOffUntilItIsAskedFor covers both a fresh install and an upgrade
// over a config.json written before these settings existed. Either turning
// itself on would mean a camera watching a room without anybody having said so.
func TestStartupIsOffUntilItIsAskedFor(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)

	if Default().Startup.LaunchAtLogin || Default().Startup.WatchOnLaunch {
		t.Errorf("Default().Startup = %+v, want both switched off", Default().Startup)
	}

	olderConfig := []byte(`{"telegram":{"enabled":true},"detection":{"cameraIndex":1}}`)
	path := filepath.Join(temporaryHome, applicationDirectoryName, configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, olderConfig, 0o600); err != nil {
		t.Fatalf("writing the older config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}

	if loaded.Startup.LaunchAtLogin || loaded.Startup.WatchOnLaunch {
		t.Errorf("a config.json with no startup section loaded as %+v, want both switched off",
			loaded.Startup)
	}
}

// TestAnUpgradeKeepsTheCameraItWasAlreadyUsing covers the migration signal.
// A file with no cameraChosen key was written before anybody was asked, and the
// index in it is the one that installation has been watching with, so sending it
// back to the Setup screen would undo a working setup.
func TestAnUpgradeKeepsTheCameraItWasAlreadyUsing(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)
	path := filepath.Join(temporaryHome, applicationDirectoryName, configFileName)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}

	olderConfig := []byte(`{"telegram":{"enabled":true},"detection":{"cameraIndex":1}}`)
	if err := os.WriteFile(path, olderConfig, 0o600); err != nil {
		t.Fatalf("writing the older config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}
	if !loaded.Detection.CameraChosen || loaded.Detection.CameraIndex != 1 {
		t.Errorf("an upgraded config loaded as %+v, want camera 1 already chosen", loaded.Detection)
	}
	if loaded.Detection.CameraURL != "" {
		t.Errorf("an upgraded config gained a stream address: %q", loaded.Detection.CameraURL)
	}
}

// TestAFreshInstallHasNotChosenACamera is the other side of that signal: a file
// this build wrote says so explicitly, and its answer has to survive a reload
// even when it is "not yet".
func TestAFreshInstallHasNotChosenACamera(t *testing.T) {
	isolateConfigDirectory(t)

	if Default().Detection.CameraChosen {
		t.Error("a fresh install claims a camera was chosen")
	}

	if err := Save(Default()); err != nil {
		t.Fatalf("Save() returned %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}
	if loaded.Detection.CameraChosen {
		t.Error("reloading a config that says no camera was chosen decided one had been")
	}
}

// TestSaveLeavesNoTemporaryFileBehind covers the atomic write. Save writes a
// .tmp and renames it so an interrupted save cannot leave a truncated
// config.json with no credentials in it; a surviving .tmp means the rename step
// was skipped or the wrong path was renamed.
func TestSaveLeavesNoTemporaryFileBehind(t *testing.T) {
	isolateConfigDirectory(t)

	if err := Save(Default()); err != nil {
		t.Fatalf("Save() returned %v", err)
	}

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("a temporary file survived a successful save: %s", entry.Name())
		}
	}

	if _, err := os.Stat(filepath.Join(directory, configFileName)); err != nil {
		t.Errorf("Save() did not produce %s: %v", configFileName, err)
	}
}

// TestSaveWritesOwnerOnlyPermissions matters because config.json holds a
// Telegram bot token: anyone who can read it can post as the user's bot.
func TestSaveWritesOwnerOnlyPermissions(t *testing.T) {
	// Windows has no Unix mode bits. Go's Chmod there only toggles the
	// read-only flag, so a file created with 0600 still reports 0666 and the
	// assertion below cannot mean anything. Confidentiality on Windows comes
	// from the ACL the file inherits from the user's roaming profile instead.
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not enforced on Windows")
	}

	isolateConfigDirectory(t)

	settings := Default()
	settings.Telegram.BotToken = "1234567890:EXAMPLE"

	if err := Save(settings); err != nil {
		t.Fatalf("Save() returned %v", err)
	}

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}

	info, err := os.Stat(filepath.Join(directory, configFileName))
	if err != nil {
		t.Fatalf("Stat returned %v", err)
	}

	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Errorf("config.json permissions = %#o, want 0600", permissions)
	}
}

func TestSaveOverwritesAnExistingConfig(t *testing.T) {
	// The UI saves on every settings change, so the second write has to replace
	// the first rather than fail on an existing file or an existing .tmp.
	isolateConfigDirectory(t)

	first := Default()
	first.Telegram.ChatID = "111"
	if err := Save(first); err != nil {
		t.Fatalf("first Save() returned %v", err)
	}

	second := Default()
	second.Telegram.ChatID = "222"
	if err := Save(second); err != nil {
		t.Fatalf("second Save() returned %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned %v", err)
	}
	if reloaded.Telegram.ChatID != "222" {
		t.Errorf("chat ID = %q, want the most recently saved %q", reloaded.Telegram.ChatID, "222")
	}
}

func TestMediaSubdirectoriesHangOffTheConfiguredMediaDirectory(t *testing.T) {
	settings := Default()
	settings.Storage.MediaDirectory = filepath.Join("some", "media")

	if expected := filepath.Join("some", "media", "snapshots"); settings.SnapshotDirectory() != expected {
		t.Errorf("SnapshotDirectory() = %q, want %q", settings.SnapshotDirectory(), expected)
	}
	if expected := filepath.Join("some", "media", "videos"); settings.VideoDirectory() != expected {
		t.Errorf("VideoDirectory() = %q, want %q", settings.VideoDirectory(), expected)
	}
}

// TestConfiguredForAlertsRequiresAllThreeSettings guards the check the UI uses
// before offering to arm the camera: a half-filled Telegram form must not count
// as configured, or the user arms a camera whose alerts silently fail.
func TestConfiguredForAlertsRequiresAllThreeSettings(t *testing.T) {
	cases := []struct {
		name     string
		telegram TelegramSettings
		expected bool
	}{
		{
			name:     "enabled with both credentials",
			telegram: TelegramSettings{Enabled: true, BotToken: "token", ChatID: "chat"},
			expected: true,
		},
		{
			name:     "disabled despite both credentials",
			telegram: TelegramSettings{Enabled: false, BotToken: "token", ChatID: "chat"},
			expected: false,
		},
		{
			name:     "missing bot token",
			telegram: TelegramSettings{Enabled: true, BotToken: "", ChatID: "chat"},
			expected: false,
		},
		{
			name:     "missing chat ID",
			telegram: TelegramSettings{Enabled: true, BotToken: "token", ChatID: ""},
			expected: false,
		},
		{
			name:     "enabled with nothing filled in, the state after a first run",
			telegram: TelegramSettings{Enabled: true},
			expected: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := Config{Telegram: testCase.telegram}

			if got := settings.ConfiguredForAlerts(); got != testCase.expected {
				t.Errorf("ConfiguredForAlerts() = %v, want %v", got, testCase.expected)
			}
		})
	}
}

// TestSavedConfigIsHandEditable checks the encoded shape, since the whole reason
// Duration marshals as a string is that a user may open this file in an editor.
func TestSavedConfigIsHandEditable(t *testing.T) {
	isolateConfigDirectory(t)

	if err := Save(Default()); err != nil {
		t.Fatalf("Save() returned %v", err)
	}

	directory, err := Directory()
	if err != nil {
		t.Fatalf("Directory() returned %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(directory, configFileName))
	if err != nil {
		t.Fatalf("ReadFile returned %v", err)
	}

	if !strings.Contains(string(contents), `"clipDuration": "15s"`) {
		t.Errorf("saved config does not spell durations readably:\n%s", contents)
	}
	if !strings.Contains(string(contents), "\n") {
		t.Error("saved config is not indented, which makes hand editing impractical")
	}
}
