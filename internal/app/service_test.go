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

	"spareeye/internal/autostart"
	"spareeye/internal/config"
	"spareeye/internal/detector"
	"spareeye/internal/host"
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

	service := NewService(log, settings, nil)

	// Most of these tests are about something other than the camera, and an app
	// that has not been told which one to use refuses to arm before it gets as
	// far as whatever they are checking. They start from an installation where
	// somebody already chose camera 0 and a sweep confirmed it.
	chooseWorkingCamera(service, 0)

	return service
}

func chooseWorkingCamera(service *Service, index int) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.settings.Detection.CameraIndex = index
	service.settings.Detection.CameraChosen = true
	service.camerasScanned = true
	service.cameras = []detector.Camera{{Index: index, Width: 640, Height: 480}}
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

func TestOpenTargetOnlyAcceptsFilesDirectlyInTheMediaDirectories(t *testing.T) {
	service := newTestService(t)
	mediaDirectory := service.Settings().MediaDirectory

	siblingDirectory := filepath.Join(mediaDirectory, "other")
	if err := os.MkdirAll(siblingDirectory, 0o700); err != nil {
		t.Fatalf("creating sibling directory: %v", err)
	}

	clipPath := filepath.Join(mediaDirectory, "videos", "motion_2026-01-01_00-00-00.mp4")
	if err := os.WriteFile(clipPath, []byte("not really video"), 0o600); err != nil {
		t.Fatalf("writing clip: %v", err)
	}

	resolved, err := service.OpenTarget(clipPath)
	if err != nil || resolved != clipPath {
		t.Fatalf("OpenTarget(%q) returned %q, error %v", clipPath, resolved, err)
	}

	refused := []string{
		"/etc/passwd",
		filepath.Join(mediaDirectory, "videos", "..", "..", "config.json"),
		filepath.Join(siblingDirectory, "clip.mp4"),
	}

	for _, path := range refused {
		if _, err := service.OpenTarget(path); err == nil {
			t.Errorf("OpenTarget(%q) was accepted, want rejection", path)
		}
	}
}

func TestRecordingsFolderCreatesTheMediaDirectory(t *testing.T) {
	service := newTestService(t)

	missing := filepath.Join(t.TempDir(), "not-yet-created")
	service.mutex.Lock()
	service.settings.Storage.MediaDirectory = missing
	service.mutex.Unlock()

	folder, err := service.RecordingsFolder()
	if err != nil || folder != missing {
		t.Fatalf("RecordingsFolder returned %q, error %v", folder, err)
	}

	if _, err := os.Stat(missing); err != nil {
		t.Fatalf("media directory was not created: %v", err)
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

func TestPosterRouteServesPostersAndRefusesTraversal(t *testing.T) {
	service := newTestService(t)
	settings := service.Settings()

	posterDirectory := filepath.Join(settings.MediaDirectory, "videos", ".posters")
	if err := os.MkdirAll(posterDirectory, 0o700); err != nil {
		t.Fatalf("creating poster directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(posterDirectory, "motion_a.mp4.jpg"), []byte("poster"), 0o600); err != nil {
		t.Fatalf("writing poster: %v", err)
	}

	secret := filepath.Join(settings.MediaDirectory, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}

	handler := service.AssetHandler()

	served := httptest.NewRecorder()
	handler.ServeHTTP(served, httptest.NewRequest(http.MethodGet, posterRoute+"motion_a.mp4.jpg", nil))
	if served.Code != http.StatusOK || served.Body.String() != "poster" {
		t.Fatalf("serving a poster returned %d: %q", served.Code, served.Body.String())
	}

	escaped := httptest.NewRecorder()
	handler.ServeHTTP(escaped, httptest.NewRequest(http.MethodGet, posterRoute+"../../secret.txt", nil))
	if escaped.Body.String() == "private" {
		t.Fatal("traversal escaped the poster directory")
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
	chooseWorkingCamera(service, 0)

	status := service.Status()
	if !status.CanArm {
		t.Fatalf("a lock that could not be taken blocked arming: %+v", status.Problem)
	}
}

// TestAFirstRunWaitsToBeToldWhichCamera is the whole point of the chosen flag:
// a working camera being present is not permission to point it at a room.
func TestAFirstRunWaitsToBeToldWhichCamera(t *testing.T) {
	service := newTestService(t)

	service.mutex.Lock()
	service.settings.Detection.CameraChosen = false
	service.settings.Telegram.Enabled = false
	service.cameras = []detector.Camera{{Index: 1, Width: 640, Height: 480}}
	service.mutex.Unlock()

	status := service.Status()
	if status.CanArm {
		t.Fatal("a first run armed itself before anybody chose a camera")
	}
	if status.Problem == nil || !strings.Contains(status.Problem.Headline, "not chosen") {
		t.Fatalf("status did not ask for a camera to be chosen: %+v", status.Problem)
	}
	if service.Settings().CameraIndex != 0 {
		t.Fatalf("the camera index moved to %d without anybody choosing it", service.Settings().CameraIndex)
	}
}

// TestAChosenCameraThatStopsRespondingIsSaidSo covers the upgrade from silently
// adopting a replacement: the user has to know their camera stopped, because
// only they can decide what the app should watch instead.
func TestAChosenCameraThatStopsRespondingIsSaidSo(t *testing.T) {
	service := newTestService(t)

	service.mutex.Lock()
	service.settings.Telegram.Enabled = false
	service.cameras = []detector.Camera{{Index: 3, Width: 640, Height: 480}}
	service.mutex.Unlock()

	status := service.Status()
	if status.CanArm {
		t.Fatal("arming was offered with the chosen camera missing")
	}
	if status.Problem == nil || !strings.Contains(status.Problem.Headline, "not responding") {
		t.Fatalf("status did not report the missing camera: %+v", status.Problem)
	}
	if service.Settings().CameraIndex != 0 {
		t.Fatalf("the camera moved to %d on its own", service.Settings().CameraIndex)
	}
}

// TestAStreamIsNotBlockedByAnEmptyDeviceSweep guards the case a device sweep
// says nothing about: a network camera is not on the list and never will be.
func TestAStreamIsNotBlockedByAnEmptyDeviceSweep(t *testing.T) {
	service := newTestService(t)

	service.mutex.Lock()
	service.settings.Telegram.Enabled = false
	service.settings.Detection.CameraURL = "rtsp://192.168.1.50:554/stream"
	service.cameras = nil
	service.mutex.Unlock()

	if status := service.Status(); !status.CanArm {
		t.Fatalf("a chosen stream was refused because no local device was found: %+v", status.Problem)
	}
}

func TestValidateCameraURLAcceptsOnlyStreamsThatCanBeOpened(t *testing.T) {
	for _, address := range []string{
		"rtsp://192.168.1.50:554/stream",
		"rtsps://camera.lan/live",
		"http://camera.lan/video.mjpg",
		"HTTPS://camera.lan/video",
	} {
		if problem := validateCameraURL(address); problem != nil {
			t.Errorf("%q was rejected: %+v", address, problem)
		}
	}

	for _, address := range []string{
		"",
		"   ",
		"/dev/video0",
		"file:///etc/passwd",
		"ftp://camera.lan/live",
		"rtsp://",
		"camera.lan/live",
	} {
		problem := validateCameraURL(address)
		if problem == nil {
			t.Errorf("%q was accepted as a camera address", address)
			continue
		}
		if problem.NextStep == "" {
			t.Errorf("%q was rejected without saying what to do instead", address)
		}
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
		MinimumFreeGigabytes: 5,
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
		"negative free space floor":            func(form *TuningForm) { form.MinimumFreeGigabytes = -1 },
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

func TestChangingTelegramTokenForcesChatRediscovery(t *testing.T) {
	saved := config.TelegramSettings{
		BotToken: "old-token",
		ChatID:   "old-chat",
		Enabled:  true,
	}

	changed := telegramFormForConnection(TelegramForm{
		BotToken: "new-token",
		ChatID:   saved.ChatID,
		Enabled:  true,
	}, saved)
	if changed.ChatID != "" {
		t.Errorf("chat ID after token change = %q, want rediscovery", changed.ChatID)
	}

	unchanged := telegramFormForConnection(TelegramForm{
		BotToken: " old-token ",
		ChatID:   saved.ChatID,
		Enabled:  true,
	}, saved)
	if unchanged.ChatID != saved.ChatID {
		t.Errorf("chat ID with unchanged token = %q, want %q", unchanged.ChatID, saved.ChatID)
	}
}

func TestTelegramConnectionFormNormalizesCredentials(t *testing.T) {
	saved := config.TelegramSettings{
		BotToken: "old-token",
		ChatID:   "old-chat",
		Enabled:  true,
	}

	form := telegramFormForConnection(TelegramForm{
		BotToken: " old-token ",
		ChatID:   " old-chat ",
		Enabled:  true,
	}, saved)

	if form.BotToken != saved.BotToken {
		t.Errorf("normalized bot token = %q, want %q", form.BotToken, saved.BotToken)
	}
	if form.ChatID != saved.ChatID {
		t.Errorf("normalized chat ID = %q, want %q", form.ChatID, saved.ChatID)
	}
}

func TestTelegramDiscoveryCredentialFailuresKeepTokenAdvice(t *testing.T) {
	cases := []string{
		"bot token rejected by Telegram",
		"Telegram returned status 404",
		"Telegram reported not-ok",
	}

	for _, failure := range cases {
		err := errors.New(failure)
		if !telegramDiscoveryCredentialsFailed(err) {
			t.Errorf("telegramDiscoveryCredentialsFailed(%q) = false, want true", failure)
			continue
		}
		problem := describeTelegramFailure(err)
		if !strings.Contains(problem.NextStep, "BotFather") {
			t.Errorf("%q produced next step %q, want token advice", failure, problem.NextStep)
		}
	}

	if telegramDiscoveryCredentialsFailed(errors.New("no Telegram conversation found")) {
		t.Error("an empty update stream was mistaken for a credential failure")
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

func TestStatusReadsTheHostOncePerHalfMinute(t *testing.T) {
	service := newTestService(t)

	reads := 0
	now := time.Date(2026, time.October, 30, 12, 0, 0, 0, time.UTC)
	service.currentTimeFor = func() time.Time { return now }
	service.readHost = func(string) host.Reading {
		reads++
		return host.Reading{BatteryPercent: 64, FreeBytes: 1 << 30, CelsiusOK: true, Celsius: 41}
	}

	first := service.Status()
	now = now.Add(10 * time.Second)
	service.Status()

	if reads != 1 {
		t.Fatalf("host read %d times within 30s, want 1", reads)
	}
	if first.BatteryPercent == nil || *first.BatteryPercent != 64 || first.OnMains {
		t.Fatalf("battery = %v, onMains = %v, want 64 on battery", first.BatteryPercent, first.OnMains)
	}
	if first.DiskFreeBytes == nil || first.TemperatureCelsius == nil {
		t.Fatalf("disk and temperature should be known: %v %v", first.DiskFreeBytes, first.TemperatureCelsius)
	}

	now = now.Add(30 * time.Second)
	service.Status()

	if reads != 2 {
		t.Fatalf("host read %d times after 30s, want 2", reads)
	}
}

func TestStatusLeavesUnknownHostReadingsNull(t *testing.T) {
	service := newTestService(t)
	service.readHost = func(string) host.Reading { return host.Reading{BatteryPercent: -1, OnMains: true} }

	status := service.Status()

	if status.BatteryPercent != nil || status.DiskFreeBytes != nil || status.TemperatureCelsius != nil {
		t.Fatalf("unknown readings should be nil, got %v %v %v", status.BatteryPercent, status.DiskFreeBytes, status.TemperatureCelsius)
	}
}
