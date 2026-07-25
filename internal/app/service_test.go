package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kaa-rada/internal/autostart"
	"kaa-rada/internal/config"
	"kaa-rada/internal/detector"
)

// newTestService builds a service whose media directories are throwaway
// temporary ones, so the asset and deletion tests never touch real recordings.
func newTestService(t *testing.T) *Service {
	t.Helper()

	mediaDirectory := t.TempDir()
	for _, sub := range []string{"snapshots", "videos"} {
		if err := os.MkdirAll(filepath.Join(mediaDirectory, sub), 0o700); err != nil {
			t.Fatalf("creating %s: %v", sub, err)
		}
	}

	log, err := OpenLog(t.TempDir())
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	settings := config.Default()
	settings.Storage.MediaDirectory = mediaDirectory

	return NewService(log, settings, nil)
}

func TestResolveMediaRejectsPathsOutsideTheMediaDirectory(t *testing.T) {
	service := newTestService(t)
	settings := service.Settings()

	inside := filepath.Join(settings.MediaDirectory, "videos", "manual_2026-01-01_00-00-00.mp4")
	if _, kind, err := service.resolveMedia(inside); err != nil || kind == 0 {
		t.Fatalf("resolving a clip returned kind %v, error %v", kind, err)
	}

	outside := []string{
		"/etc/passwd",
		filepath.Join(settings.MediaDirectory, "videos", "..", "..", "config.json"),
		filepath.Join(settings.MediaDirectory, "videos", "nested", "clip.mp4"),
	}

	for _, path := range outside {
		if _, _, err := service.resolveMedia(path); err == nil {
			t.Errorf("resolveMedia(%q) was accepted, want rejection", path)
		}
	}
}

func TestAssetHandlerServesMediaAndRefusesTraversal(t *testing.T) {
	service := newTestService(t)
	settings := service.Settings()

	clipPath := filepath.Join(settings.MediaDirectory, "videos", "motion_2026-01-01_00-00-00.mp4")
	if err := os.WriteFile(clipPath, []byte("not really video"), 0o600); err != nil {
		t.Fatalf("writing clip: %v", err)
	}

	secret := filepath.Join(settings.MediaDirectory, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}

	handler := service.AssetHandler()

	served := httptest.NewRecorder()
	handler.ServeHTTP(served, httptest.NewRequest(http.MethodGet, videoRoute+"motion_2026-01-01_00-00-00.mp4", nil))
	if served.Code != http.StatusOK || served.Body.String() != "not really video" {
		t.Fatalf("serving a clip returned %d: %q", served.Code, served.Body.String())
	}

	// A traversal is collapsed by path.Base before it reaches the filesystem.
	escaped := httptest.NewRecorder()
	handler.ServeHTTP(escaped, httptest.NewRequest(http.MethodGet, videoRoute+"../secret.txt", nil))
	if escaped.Body.String() == "private" {
		t.Fatal("traversal escaped the video directory")
	}
}

func TestPreviewReportsNoContentWhileDisarmed(t *testing.T) {
	service := newTestService(t)

	response := httptest.NewRecorder()
	service.AssetHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, previewRoute, nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("preview returned %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestArmIsRefusedWhenTelegramIsHalfConfigured(t *testing.T) {
	service := newTestService(t)

	// config.Default enables Telegram without credentials, which is exactly the
	// state a first run is in.
	result := service.Arm()
	if result.OK {
		t.Fatal("arming succeeded without credentials, want a refusal")
	}
	if !strings.Contains(result.Headline, "Telegram") || result.NextStep == "" {
		t.Fatalf("refusal did not name Telegram or say what to do: %+v", result)
	}
	if result.DocPath == "" {
		t.Fatal("a credentials problem should point at the setup guide")
	}

	status := service.Status()
	if status.CanArm || status.Problem == nil {
		t.Fatalf("status did not report a problem: %+v", status)
	}
	if status.Headline != status.Problem.Headline {
		t.Fatalf("headline %q does not match the problem %q", status.Headline, status.Problem.Headline)
	}
}

func TestArmIsRefusedWhenAnotherInstanceHoldsTheCamera(t *testing.T) {
	log, err := OpenLog(t.TempDir())
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer log.Close()

	service := NewService(log, config.Default(), detector.ErrAlreadyRunning)

	if service.Arm().OK {
		t.Fatal("arming succeeded while the camera was claimed elsewhere")
	}

	status := service.Status()
	if status.Problem == nil || !strings.Contains(status.Problem.Headline, "already running") {
		t.Fatalf("status did not mention the other instance: %+v", status.Problem)
	}
	if status.Tone != "problem" {
		t.Fatalf("tone is %q, want problem", status.Tone)
	}
}

// TestAWrappedClaimStillNamesTheOtherInstance keeps the refusal working when the
// lock error arrives wrapped, which is how the detector reports the path it
// failed on.
func TestAWrappedClaimStillNamesTheOtherInstance(t *testing.T) {
	log, err := OpenLog(t.TempDir())
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer log.Close()

	wrapped := fmt.Errorf("claiming the camera: %w", detector.ErrAlreadyRunning)
	service := NewService(log, config.Default(), wrapped)

	status := service.Status()
	if status.Problem == nil || !strings.Contains(status.Problem.Headline, "already running") {
		t.Fatalf("status did not mention the other instance: %+v", status.Problem)
	}
}

// TestAClaimThatCouldNotBeTakenDoesNotBlockArming covers the failure that is not
// a second instance: the lock file itself was unusable. The camera is free, and
// refusing to arm would leave the user stuck behind a copy of the app that does
// not exist.
func TestAClaimThatCouldNotBeTakenDoesNotBlockArming(t *testing.T) {
	log, err := OpenLog(t.TempDir())
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer log.Close()

	settings := config.Default()
	settings.Telegram.Enabled = false

	service := NewService(log, settings, errors.New("opening lock file camera.lock: permission denied"))

	service.mutex.Lock()
	service.camerasScanned = true
	service.cameras = []detector.Camera{{Index: 0, Width: 640, Height: 480}}
	service.mutex.Unlock()

	status := service.Status()
	if !status.CanArm {
		t.Fatalf("a lock that could not be taken blocked arming: %+v", status.Problem)
	}
}

func TestValidateTuningRejectsUnusableValues(t *testing.T) {
	usable := TuningForm{
		Day:                  SensitivityForm{MinimumContourArea: 8000, ThresholdValue: 30, SampleIntervalSeconds: 1, AlertCooldownSeconds: 300},
		Night:                SensitivityForm{MinimumContourArea: 9000, ThresholdValue: 30, SampleIntervalSeconds: 1, AlertCooldownSeconds: 300},
		NightStartMinutes:    1140,
		NightEndMinutes:      360,
		MotionPersistSeconds: 5,
		PrebufferLeadSeconds: 3,
		ClipDurationSeconds:  15,
		RetentionDays:        30,
	}

	if problem := validateTuning(usable); problem != nil {
		t.Fatalf("a usable form was rejected: %+v", problem)
	}

	rejected := map[string]func(*TuningForm){
		"zero sample interval":                 func(form *TuningForm) { form.Day.SampleIntervalSeconds = 0 },
		"threshold above 255":                  func(form *TuningForm) { form.Night.ThresholdValue = 300 },
		"negative area":                        func(form *TuningForm) { form.Day.MinimumContourArea = -1 },
		"night start past midnight in minutes": func(form *TuningForm) { form.NightStartMinutes = 1440 },
		"zero clip duration":                   func(form *TuningForm) { form.ClipDurationSeconds = 0 },
		"negative retention":                   func(form *TuningForm) { form.RetentionDays = -1 },
	}

	for name, corrupt := range rejected {
		broken := usable
		corrupt(&broken)

		problem := validateTuning(broken)
		if problem == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		// Every rejection has to tell the user what to change.
		if problem.NextStep == "" {
			t.Errorf("%s was rejected without saying what to do", name)
		}
	}
}

func TestValidateLiveViewRejectsUnusableValues(t *testing.T) {
	usable := LiveViewForm{PreviewFPS: 12, PreviewQuality: 80}

	if problem := validateLiveView(usable); problem != nil {
		t.Fatalf("a usable form was rejected: %+v", problem)
	}

	rejected := map[string]LiveViewForm{
		"zero frame rate":     {PreviewFPS: 0, PreviewQuality: 80},
		"frame rate above 60": {PreviewFPS: 61, PreviewQuality: 80},
		"quality below 30":    {PreviewFPS: 12, PreviewQuality: 29},
		"quality above 95":    {PreviewFPS: 12, PreviewQuality: 96},
	}

	for name, broken := range rejected {
		problem := validateLiveView(broken)
		if problem == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		if problem.NextStep == "" {
			t.Errorf("%s was rejected without saying what to do", name)
		}
	}
}

func TestEventHistoryKeepsTheNewestEventsFirst(t *testing.T) {
	history := eventHistory{limit: 3}

	for index := 0; index < 5; index++ {
		history.add(detector.Event{
			Kind:      detector.EventSnapshotSaved,
			At:        time.Unix(int64(index), 0),
			MediaPath: filepath.Join("/media", "snapshots", "shot.jpg"),
		})
	}

	view := history.view()
	if len(view) != 3 {
		t.Fatalf("kept %d events, want 3", len(view))
	}
	if !view[0].At.Equal(time.Unix(4, 0)) {
		t.Fatalf("first event is %v, want the newest", view[0].At)
	}
	if view[0].MediaName != "shot.jpg" {
		t.Fatalf("media name is %q, want the base name", view[0].MediaName)
	}
	if view[0].Message != "Saved a photo" {
		t.Fatalf("event message is %q, want a plain sentence", view[0].Message)
	}
}

func TestLogRetainsRecentLinesAndWritesThemToDisk(t *testing.T) {
	directory := t.TempDir()

	log, err := OpenLog(directory)
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer log.Close()

	log.Info("camera opened", "index", 1)

	lines := log.Recent()
	if len(lines) != 1 || !strings.Contains(lines[0], "camera opened") {
		t.Fatalf("retained lines are %v", lines)
	}

	written, err := os.ReadFile(filepath.Join(directory, logFileName))
	if err != nil {
		t.Fatalf("reading the log file: %v", err)
	}
	if !strings.Contains(string(written), "index=1") {
		t.Fatalf("log file does not hold the record: %q", written)
	}
}

// TestPanickedWritesTheStackToTheLogFile guards the only diagnostic a Windows
// desktop build has: the runtime's crash report has no console to reach, so a
// panic that is not written to the file leaves nothing behind at all.
func TestPanickedWritesTheStackToTheLogFile(t *testing.T) {
	directory := t.TempDir()

	log, err := OpenLog(directory)
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer log.Close()

	func() {
		defer func() {
			if panicked := recover(); panicked != nil {
				log.Panicked("probing capture devices", panicked)
			}
		}()

		panic("camera 3 vanished mid-probe")
	}()

	written, err := os.ReadFile(filepath.Join(directory, logFileName))
	if err != nil {
		t.Fatalf("reading the log file: %v", err)
	}

	// The stage names where it happened, the value says what, and the stack has
	// to reach as far as the function that panicked to be worth reading.
	for _, expected := range []string{
		"probing capture devices",
		"camera 3 vanished mid-probe",
		"TestPanickedWritesTheStackToTheLogFile",
	} {
		if !strings.Contains(string(written), expected) {
			t.Errorf("the log file does not mention %q: %s", expected, written)
		}
	}
}

// TestEveryFailureTellsTheUserWhatToDo guards the rule that made this UI
// revision necessary: no action may report a bare engine error, and no failure
// may leave the user without a next step.
// isolateStartupRegistration redirects both the settings file and the login
// registration into a temporary directory.
//
// Only the Linux registration can be redirected this way. On Windows the Run
// key belongs to the logged-in user and nothing about it is per-test, so these
// tests would register the compiled test binary to run at somebody's next
// login; on macOS registering is not supported at all.
func isolateStartupRegistration(t *testing.T) string {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("the login registration can only be redirected into a temporary directory on Linux")
	}

	temporaryHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", temporaryHome)

	return temporaryHome
}

func TestSaveStartupKeepsBothChoicesAndMatchesTheOperatingSystemToThem(t *testing.T) {
	isolateStartupRegistration(t)

	service := newTestService(t)

	// The two switches are independent, so the interesting case is one on and
	// one off: a user who wants the app open at login has not thereby asked to
	// be recorded from the moment they log in.
	if result := service.SaveStartup(StartupForm{LaunchAtLogin: true}); !result.OK {
		t.Fatalf("SaveStartup() reported %+v", result)
	}

	saved := service.Settings().Startup
	if !saved.LaunchAtLogin || saved.WatchOnLaunch {
		t.Errorf("saved startup settings = %+v, want only launching at login", saved)
	}

	registered, err := autostart.Enabled()
	if err != nil {
		t.Fatalf("autostart.Enabled() returned %v", err)
	}
	if !registered {
		t.Error("saving with launch at login on left this computer unregistered")
	}

	if result := service.SaveStartup(StartupForm{WatchOnLaunch: true}); !result.OK {
		t.Fatalf("SaveStartup() reported %+v", result)
	}

	registered, err = autostart.Enabled()
	if err != nil {
		t.Fatalf("autostart.Enabled() returned %v", err)
	}
	if registered {
		t.Error("switching launch at login off left the registration in place")
	}
	if !service.Settings().Startup.WatchOnLaunch {
		t.Error("watching on launch was not saved")
	}
}

func TestSaveStartupKeepsTheChoicesWhenTheComputerRefusesToRegisterThem(t *testing.T) {
	temporaryHome := isolateStartupRegistration(t)

	// A file where the autostart directory belongs is a refusal no privilege
	// gets around, which is what makes it a usable stand-in for the security
	// software that blocks this on a real machine.
	if err := os.WriteFile(filepath.Join(temporaryHome, "autostart"), []byte("in the way"), 0o600); err != nil {
		t.Fatalf("blocking the autostart directory: %v", err)
	}

	service := newTestService(t)

	result := service.SaveStartup(StartupForm{LaunchAtLogin: true, WatchOnLaunch: true})
	if result.OK {
		t.Fatal("SaveStartup() reported success although the registration could not be written")
	}
	if result.Headline == "" || result.NextStep == "" {
		t.Errorf("SaveStartup() reported %+v, want a headline and a next step", result)
	}

	// The settings are written before the registration is attempted, so a
	// refusal costs the user only the switch that was refused.
	saved := service.Settings().Startup
	if !saved.WatchOnLaunch {
		t.Errorf("saved startup settings = %+v, want watching on launch kept", saved)
	}

	status := service.Status()
	if !status.LaunchAtLogin || status.LaunchAtLoginRegistered {
		t.Errorf("status reported asked-for %v and registered %v, want the disagreement shown",
			status.LaunchAtLogin, status.LaunchAtLoginRegistered)
	}
}

func TestEveryFailureTellsTheUserWhatToDo(t *testing.T) {
	service := newTestService(t)

	failures := map[string]ActionResult{
		"arm without credentials":               service.Arm(),
		"test telegram with blanks":             service.TestTelegram(TelegramForm{}),
		"send a file that is not a recording":   service.SendRecording("/etc/passwd"),
		"delete a file that is not a recording": service.DeleteRecording("/etc/passwd"),
		"record while stopped":                  service.StartManualRecording(),
	}

	for name, result := range failures {
		if result.OK {
			t.Errorf("%s unexpectedly succeeded", name)
			continue
		}
		if result.Headline == "" || result.NextStep == "" {
			t.Errorf("%s reported %+v, want a headline and a next step", name, result)
		}
		// Go error text reads like "opening camera 1: ..." or "status 401".
		// None of that belongs in front of somebody setting up a camera.
		for _, leak := range []string{"error", "failed:", "status 4", "invalid", "nil"} {
			if strings.Contains(strings.ToLower(result.Headline), leak) {
				t.Errorf("%s headline leaks engine wording %q: %q", name, leak, result.Headline)
			}
		}
	}
}

func TestTelegramFailuresAreTranslatedIntoAdvice(t *testing.T) {
	cases := map[string]string{
		"bot token rejected by Telegram":                   "BotFather",
		"reaching Telegram: dial tcp: no such host":        "internet connection",
		"Telegram status 400: Bad Request: chat not found": "chat ID",
		"Telegram status 403: Forbidden":                   "/start",
	}

	for failure, expected := range cases {
		problem := describeTelegramFailure(errors.New(failure))

		if !strings.Contains(problem.NextStep, expected) {
			t.Errorf("%q produced next step %q, want it to mention %q", failure, problem.NextStep, expected)
		}
		if strings.Contains(problem.Headline, "status") {
			t.Errorf("%q leaked an HTTP status into the headline: %q", failure, problem.Headline)
		}
	}
}

func TestStatusSpeaksPlainlyThroughEveryState(t *testing.T) {
	service := newTestService(t)

	// A half-configured install reports the obstacle, not "not running".
	blocked := service.Status()
	if blocked.Tone != "problem" || blocked.Headline == "" {
		t.Fatalf("blocked status is %+v", blocked)
	}

	// With alerts switched off there is nothing in the way, so the headline
	// becomes the ordinary idle one.
	if result := service.SaveTelegram(TelegramForm{Enabled: false}); !result.OK {
		t.Fatalf("turning alerts off: %+v", result)
	}
	service.mutex.Lock()
	service.camerasScanned = true
	service.cameras = []detector.Camera{{Index: 0, Width: 640, Height: 480}}
	service.mutex.Unlock()

	idle := service.Status()
	if idle.Headline != "Not running" || idle.Tone != "idle" {
		t.Fatalf("idle status is %+v", idle)
	}
	if idle.Explanation == "" {
		t.Fatal("idle status has no explanation for the user")
	}
	if !idle.CanArm {
		t.Fatal("a configured install with a camera should be able to start")
	}
}

func TestDeliveryOutcomesReachTheActivityList(t *testing.T) {
	history := eventHistory{limit: 10}

	history.record(EventView{Kind: eventAlertSent, Message: "Sent the photo to Telegram"})

	view := history.view()
	if len(view) != 1 || view[0].Message != "Sent the photo to Telegram" {
		t.Fatalf("history is %+v", view)
	}
	if view[0].At.IsZero() {
		t.Fatal("a recorded entry should be stamped with the time it happened")
	}
}
