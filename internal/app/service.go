package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"spareeye/internal/autostart"
	"spareeye/internal/config"
	"spareeye/internal/detector"
	"spareeye/internal/host"
	"spareeye/internal/media"
)

// cameraOpenTimeout bounds how long Arm waits for the first frame before giving
// up. Opening a device and measuring its frame rate takes a second or two; a
// device that has not produced a frame well past that is not going to.
const cameraOpenTimeout = 25 * time.Second

// Service owns the detector for the desktop UI: one camera, one notifier, one
// set of settings, and the history the UI reads back.
//
// Every exported method is safe to call from the webview's goroutine, because
// Wails dispatches bound calls concurrently.
type Service struct {
	log *Log

	// instanceLockError records why the camera claim failed, if it did. The UI
	// refuses to arm rather than letting the second instance die inside OpenCV.
	instanceLockError error

	mutex    sync.Mutex
	settings config.Config
	session  *detectorSession
	lastFail string

	// launchProblem is why watching did not begin by itself, kept so that a
	// user who asked to be watching on launch, and is not, is told why instead
	// of finding a screen that simply reads "Not running".
	launchProblem *Problem

	cameras []detector.Camera
	// camerasScanned distinguishes "no camera found" from "not looked yet",
	// which decide whether arming is refused or merely unproven.
	camerasScanned bool

	events    eventHistory
	messenger messenger

	// hostReading is the last look at the battery, disk and temperature. The UI
	// asks for status every second, but these come from sysfs or Win32 and move
	// slowly, so they are refreshed lazily at most once per hostReadingMaxAge.
	hostReading    host.Reading
	hostReadAt     time.Time
	readHost       func(directory string) host.Reading
	currentTimeFor func() time.Time

	// stopCleanup and cleanupFinished belong to the periodic cleanup loop,
	// which runs from Startup until Shutdown.
	stopCleanup     context.CancelFunc
	cleanupFinished chan struct{}
}

// hostReadingMaxAge is how stale the battery, disk and temperature figures may
// get before the next status call reads them again.
const hostReadingMaxAge = 30 * time.Second

// detectorSession is one run of the detector, from opening the camera to the
// event channel closing.
type detectorSession struct {
	engine *detector.Detector
	cancel context.CancelFunc

	// opened closes once the camera has delivered a frame, which is the only
	// proof the device is really usable.
	opened chan struct{}

	// finished closes after Run returned *and* every event was consumed, so
	// runError is safe to read once it is closed.
	finished chan struct{}
	runError error
}

// NewService prepares the UI's service layer. It does not touch the camera;
// call Startup once the window exists.
func NewService(log *Log, settings config.Config, instanceLockError error) *Service {
	return &Service{
		log:               log,
		instanceLockError: instanceLockError,
		settings:          settings,
		events:            eventHistory{limit: retainedEvents},
		readHost:          host.Read,
		currentTimeFor:    time.Now,
	}
}

// currentHostReading returns the cached reading, taking a fresh one when it is
// too old. The read happens outside the mutex because it touches the file
// system, and Status must not make every other call wait behind that. Two
// callers racing past the age check both read, which is harmless.
func (s *Service) currentHostReading(mediaDirectory string) host.Reading {
	s.mutex.Lock()
	cached, readAt := s.hostReading, s.hostReadAt
	s.mutex.Unlock()

	now := s.currentTimeFor()
	if !readAt.IsZero() && now.Sub(readAt) < hostReadingMaxAge {
		return cached
	}

	fresh := s.readHost(mediaDirectory)

	s.mutex.Lock()
	s.hostReading = fresh
	s.hostReadAt = now
	s.mutex.Unlock()

	return fresh
}

// Startup runs the work that should happen once the app is up: reclaiming disk
// space and connecting the notifier.
func (s *Service) Startup(ctx context.Context) {
	// Wails calls this on its own goroutine, so a panic here would close the
	// window that has just been painted and end the process before main could
	// report it. An app that came up without pruning is worth more.
	defer func() {
		if panicked := recover(); panicked != nil {
			s.log.Panicked("preparing the window", panicked)
		}
	}()

	s.log.Info("startup: window opened")

	if s.instanceLockError != nil {
		s.log.Error("camera claim refused", "error", s.instanceLockError)
	}

	s.pruneExpiredMedia()
	s.startCleanupLoop()
	s.restartMessenger()

	// Probing opens every device in turn, which takes long enough to be worth
	// keeping off the path that paints the window. Arming waits for it to
	// finish, because the probe is what settles which device is captured from.
	go func() {
		s.resolveCamera()
		s.armAtLaunch()
	}()
}

// armAtLaunch starts watching when the user has asked for that, whether the app
// was opened by them or by the operating system at login.
func (s *Service) armAtLaunch() {
	// Like the probe before it, this runs where a panic would take the window
	// down with the process and leave the user with nothing to read.
	defer func() {
		if panicked := recover(); panicked != nil {
			s.log.Panicked("starting the watch at launch", panicked)
		}
	}()

	s.mutex.Lock()
	wanted := s.settings.Startup.WatchOnLaunch
	s.mutex.Unlock()

	if !wanted {
		return
	}

	s.log.Info("starting the watch at launch")

	result := s.Arm()
	if result.OK {
		return
	}

	s.mutex.Lock()
	s.launchProblem = &Problem{
		Headline: result.Headline,
		NextStep: result.NextStep,
		DocPath:  result.DocPath,
	}
	s.mutex.Unlock()
}

// resolveCamera finds out what is available and whether the chosen camera is
// among it. It changes nothing: a camera is adopted only when somebody picks
// one, and a chosen camera that has stopped answering becomes a problem on
// screen rather than a silent move to whatever else is plugged in.
func (s *Service) resolveCamera() {
	// This runs on its own goroutine, where a panic would take the window down
	// with the process and leave the user with nothing to read.
	defer func() {
		if panicked := recover(); panicked != nil {
			s.log.Panicked("probing capture devices", panicked)
		}
	}()

	s.mutex.Lock()
	detection := s.settings.Detection
	s.mutex.Unlock()

	// A network stream is not something a device sweep can find, and opening it
	// here would only duplicate what arming is about to do anyway.
	if detection.CameraChosen && detection.CameraURL != "" {
		s.log.Info("watching a network stream; no device sweep needed")
		return
	}

	// The whole sweep runs, including the configured device: the scan happens in
	// a child process, so opening a device the user did not ask for costs a
	// second rather than risking the app, and the Setup picker needs the full
	// list anyway.
	s.log.Info("probing capture devices")
	found := detector.AvailableCamerasIsolated(s.log.Logger)

	s.mutex.Lock()
	s.cameras = found
	s.camerasScanned = true
	s.mutex.Unlock()

	if !detection.CameraChosen {
		s.log.Info("waiting for a camera to be chosen", "found", len(found))
		return
	}

	if containsCamera(found, detection.CameraIndex) {
		s.log.Info("camera confirmed usable", "index", detection.CameraIndex)
		return
	}

	s.log.Warn("the chosen camera did not answer", "index", detection.CameraIndex, "found", len(found))
}

func containsCamera(cameras []detector.Camera, index int) bool {
	return slices.ContainsFunc(cameras, func(camera detector.Camera) bool {
		return camera.Index == index
	})
}

// Shutdown stops the cleanup loop, the detector and the notifier. Wails calls it while the window
// is closing, so a clip in progress still gets flushed to disk.
func (s *Service) Shutdown(ctx context.Context) {
	s.stopCleanupLoop()
	s.stopSession()
	s.messenger.stop()
	s.log.Info("shutdown complete")
}

func (s *Service) pruneExpiredMedia() {
	s.mutex.Lock()
	settings := s.settings
	s.mutex.Unlock()

	// Prune scans one directory at a time, and snapshots and clips live in
	// separate ones.
	for _, directory := range []string{settings.VideoDirectory(), settings.SnapshotDirectory()} {
		removed, err := media.Prune(directory, settings.Storage.RetentionDays, settings.Storage.MediaDirectory)
		if err != nil {
			s.log.Error("pruning failed", "directory", directory, "error", err)
			continue
		}
		if removed > 0 {
			s.log.Info("pruned expired media", "directory", directory, "removed", removed)
		}
	}
}

// Settings returns the persisted configuration in the shape the UI edits.
func (s *Service) Settings() SettingsView {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return settingsView(s.settings)
}

// Cameras returns what the last scan found, scanning once if nothing has looked
// yet. Scanning opens every device index in turn, so repeating it on each visit
// to the Setup screen would keep grabbing at hardware another program may want.
func (s *Service) Cameras() []detector.Camera {
	s.mutex.Lock()
	scanned := s.camerasScanned
	cached := s.cameras
	s.mutex.Unlock()

	if scanned {
		return cached
	}

	return s.RescanCameras()
}

// RescanCameras looks again, for when the user has just plugged a camera in or
// closed whatever was holding it.
func (s *Service) RescanCameras() []detector.Camera {
	s.mutex.Lock()
	armed := s.session != nil
	cached := s.cameras
	s.mutex.Unlock()

	// While watching, the detector owns a device: a fresh probe would fail on
	// that index and risk disturbing capture.
	if armed {
		return cached
	}

	found := detector.AvailableCamerasIsolated(s.log.Logger)

	s.mutex.Lock()
	s.cameras = found
	s.camerasScanned = true
	s.mutex.Unlock()

	return found
}

// Status is what the UI polls a few times a second.
func (s *Service) Status() StatusView {
	s.mutex.Lock()
	session := s.session
	settings := s.settings
	lastFail := s.lastFail
	launchProblem := s.launchProblem
	s.mutex.Unlock()

	// A failure to read the registration means the app cannot show that it is
	// there, which for the user is the same news as its not being there: either
	// way nothing can be promised about the next login.
	registered, _ := autostart.Enabled()

	hostReading := s.currentHostReading(settings.Storage.MediaDirectory)

	view := StatusView{
		CameraSource:            describeCameraSource(settings.Detection),
		AlertsEnabled:           settings.Telegram.Enabled,
		AlertsConfigured:        settings.ConfiguredForAlerts(),
		TelegramConnected:       s.messenger.connected(),
		LaunchAtLogin:           settings.Startup.LaunchAtLogin,
		LaunchAtLoginRegistered: registered,
		LogPath:                 s.log.Path(),
		MediaDirectory:          settings.Storage.MediaDirectory,
		LastFailure:             lastFail,
		OnMains:                 hostReading.OnMains,
	}

	if hostReading.BatteryPercent >= 0 {
		view.BatteryPercent = &hostReading.BatteryPercent
	}
	if hostReading.FreeBytes > 0 {
		view.DiskFreeBytes = &hostReading.FreeBytes
	}
	if hostReading.CelsiusOK {
		view.TemperatureCelsius = &hostReading.Celsius
	}

	blocked := s.armBlockedProblem(settings)
	if blocked == nil {
		view.CanArm = true
	} else {
		view.Problem = blocked
	}

	// Nothing stops the user arming by hand, so a watch that failed to start on
	// its own is described rather than blocked.
	if blocked == nil && launchProblem != nil {
		view.Problem = launchProblem
	}

	if session != nil {
		engineStatus := session.engine.Status()
		view.Armed = true
		view.Recording = engineStatus.Recording
		view.RecordingManual = engineStatus.RecordingManual
		view.Night = engineStatus.Night
		view.CaptureFPS = engineStatus.CaptureFPS
		view.FrameWidth = engineStatus.FrameWidth
		view.FrameHeight = engineStatus.FrameHeight

		if !engineStatus.MotionSince.IsZero() {
			view.MotionSeconds = time.Since(engineStatus.MotionSince).Seconds()
		}
	}

	view.Headline, view.Tone = headline(view)
	view.Explanation = explanation(view)

	return view
}

// headline is the one line the user reads to know what the app is doing. It is
// decided here rather than in the webview so the sidebar, the main screen and
// the window all say the same thing.
func headline(view StatusView) (string, string) {
	if view.Problem != nil && !view.Armed {
		return view.Problem.Headline, "problem"
	}
	if !view.Armed {
		return "Not running", "idle"
	}
	if view.Recording {
		return "Recording now", "recording"
	}
	if view.MotionSeconds > 0 {
		return "Motion detected", "motion"
	}

	return "Watching for motion", "watching"
}

// explanation is the smaller line under the headline. It says what the state
// means for the user, not what the detector is doing internally.
func explanation(view StatusView) string {
	// A problem states itself once, in the headline, and the explanation says
	// what to do about it. Repeating the headline in a banner underneath just
	// makes the screen look broken.
	if view.Problem != nil && !view.Armed {
		return view.Problem.NextStep
	}
	if !view.Armed {
		return "Your camera is off and nothing is being recorded."
	}
	if view.RecordingManual {
		return "Recording because you asked. It will keep going until you stop it."
	}
	if view.Recording {
		return "Saving a video clip of what is happening."
	}
	if view.MotionSeconds > 0 {
		return "Something is moving in view."
	}
	if view.AlertsConfigured && view.TelegramConnected {
		return "All quiet. You will get a Telegram message if anything moves."
	}
	if view.AlertsConfigured {
		return "All quiet. Still connecting to Telegram."
	}

	return "All quiet. Recordings are being saved to this computer only."
}

// armBlockedProblem explains why starting would fail, or returns nil when it
// would work. The UI shows it instead of letting the user press a button that
// cannot succeed.
func (s *Service) armBlockedProblem(settings config.Config) *Problem {
	// Only a claim another instance holds is a reason to refuse. Any other
	// failure means the claim could not be made at all, because of an unwritable
	// configuration directory or a lock the filesystem does not support, and the
	// camera is almost certainly free. Blocking on that would leave the user
	// permanently unable to arm, told about a second copy that does not exist.
	if errors.Is(s.instanceLockError, detector.ErrAlreadyRunning) {
		return &Problem{
			Headline: "SpareEye is already running",
			NextStep: "Another copy of this app has the camera. Switch to that window, or close it and try again.",
		}
	}

	if !settings.Detection.CameraChosen {
		problem := noCameraChosen()
		return &problem
	}

	s.mutex.Lock()
	scanned := s.camerasScanned
	cameras := s.cameras
	s.mutex.Unlock()

	// A stream is not in the device list and cannot be proven from here; whether
	// it answers is settled when the watch starts. A device that the sweep did
	// not see is a different matter: the user chose it, it is gone, and they are
	// the only one who can decide what to use instead.
	usingStream := settings.Detection.CameraURL != ""

	if !usingStream && settings.Detection.CameraIndex < 0 {
		problem := cameraUnavailable()
		return &problem
	}

	if !usingStream && scanned && !containsCamera(cameras, settings.Detection.CameraIndex) {
		problem := chosenCameraMissing()
		return &problem
	}

	// Alerts are opt-out: a user who turns Telegram off is choosing to record
	// locally, which is a legitimate way to run. Half-configured credentials
	// are the actual mistake worth blocking on.
	if settings.Telegram.Enabled && !settings.ConfiguredForAlerts() {
		return &Problem{
			Headline: "Telegram is not finished being set up",
			NextStep: "Go to Setup and enter both the bot token and the chat ID, or turn off Telegram alerts " +
				"to record to this computer only.",
			DocPath: telegramSetupDoc,
		}
	}

	return nil
}

// Arm opens the camera and starts watching. It waits for the first frame so a
// device that is present but unusable is reported now rather than as silence.
func (s *Service) Arm() ActionResult {
	s.mutex.Lock()
	if s.session != nil {
		s.mutex.Unlock()
		return succeeded("Watching for motion")
	}
	settings := s.settings
	s.mutex.Unlock()

	if blocked := s.armBlockedProblem(settings); blocked != nil {
		return blocked.asResult()
	}

	session := s.beginSession(settings)

	s.mutex.Lock()
	s.session = session
	s.lastFail = ""
	s.launchProblem = nil
	s.mutex.Unlock()

	source := loggableCameraSource(settings.Detection)

	select {
	case <-session.opened:
		s.log.Info("armed", "source", source)
		return succeeded("Watching for motion")

	case <-session.finished:
		s.forgetSession(session)
		s.noteStartupFailure(session.runError, source)

		if settings.Detection.CameraURL != "" {
			return streamUnavailable().asResult()
		}
		return cameraUnavailable().asResult()

	case <-time.After(cameraOpenTimeout):
		s.forgetSession(session)
		session.cancel()
		<-session.finished
		s.log.Error("the camera produced no frames", "source", source)

		return failed(
			"Your camera did not send any pictures",
			"The camera opened but stayed blank. This usually means it is a virtual webcam that is not "+
				"streaming yet. Start it first, or choose a different camera in Setup.",
		)
	}
}

// Disarm releases the camera, flushing any clip still being written.
func (s *Service) Disarm() ActionResult {
	s.stopSession()
	s.log.Info("disarmed")

	return succeeded("Stopped")
}

func (s *Service) beginSession(settings config.Config) *detectorSession {
	engine := detector.New(settings)
	sessionContext, cancel := context.WithCancel(context.Background())

	session := &detectorSession{
		engine:   engine,
		cancel:   cancel,
		opened:   make(chan struct{}),
		finished: make(chan struct{}),
	}

	drained := make(chan struct{})
	go s.consumeEvents(session, drained)

	go func() {
		session.runError = engine.Run(sessionContext)

		// Waiting for the reader before publishing runError is what makes it
		// safe to read after finished closes.
		<-drained
		close(session.finished)
	}()

	return session
}

// consumeEvents records every event and forwards the ones worth a message.
func (s *Service) consumeEvents(session *detectorSession, drained chan struct{}) {
	defer close(drained)

	cameraReported := false

	for event := range session.engine.Events() {
		s.events.add(event)
		s.logEvent(event)

		switch event.Kind {
		case detector.EventCameraOpened:
			if !cameraReported {
				cameraReported = true
				close(session.opened)
			}

		case detector.EventSnapshotSaved:
			// A photo the user asked for is for them to look at here, not
			// an alert, so it is not sent to their phone as motion.
			if !event.Manual {
				s.deliverSnapshot(event.MediaPath, event.Night)
			}

		case detector.EventClipFinished:
			if event.Err == nil {
				s.deliverClip(event.MediaPath, event.Manual)
			}

		case detector.EventFailure:
			s.mutex.Lock()
			s.lastFail = event.Err.Error()
			s.mutex.Unlock()
		}
	}
}

func (s *Service) logEvent(event detector.Event) {
	switch event.Kind {
	case detector.EventFailure:
		s.log.Error("detector failure", "error", event.Err)
	case detector.EventClipFinished:
		s.log.Info("clip finished", "path", event.MediaPath, "manual", event.Manual, "detail", event.Detail, "error", event.Err)
	default:
		kind, _ := describeDetectorEvent(event)
		s.log.Info(kind, "night", event.Night, "path", event.MediaPath, "detail", event.Detail)
	}
}

// stopSession cancels the current run and waits for the camera to be released,
// which must complete before another detector can open the same device.
func (s *Service) stopSession() {
	s.mutex.Lock()
	session := s.session
	s.session = nil
	s.mutex.Unlock()

	if session == nil {
		return
	}

	session.cancel()
	<-session.finished
}

func (s *Service) forgetSession(session *detectorSession) {
	s.mutex.Lock()
	if s.session == session {
		s.session = nil
	}
	s.mutex.Unlock()
}

// noteStartupFailure records the backend's error for the log and the
// diagnostics panel. The user is shown cameraUnavailable's guidance instead:
// OpenCV reports a busy device and an absent one almost identically, so the
// distinction its message draws is not one anybody can act on.
func (s *Service) noteStartupFailure(runError error, source string) {
	s.log.Error("arming failed", "source", source, "error", runError)

	if runError == nil {
		return
	}

	s.mutex.Lock()
	s.lastFail = runError.Error()
	s.mutex.Unlock()
}

// restartDetector reapplies settings that are only read when the camera opens:
// the prebuffer is sized from the measured frame rate, and the device index is
// fixed for the life of a run. It reports a problem only when the camera failed
// to come back, since the settings themselves were already saved.
func (s *Service) restartDetector() *Problem {
	s.mutex.Lock()
	armed := s.session != nil
	s.mutex.Unlock()

	if !armed {
		return nil
	}

	s.log.Info("restarting detector to apply settings")
	s.stopSession()

	if result := s.Arm(); !result.OK {
		return &Problem{Headline: result.Headline, NextStep: result.NextStep, DocPath: result.DocPath}
	}

	return nil
}

// StartManualRecording begins a clip that runs until stopped.
func (s *Service) StartManualRecording() ActionResult {
	s.mutex.Lock()
	session := s.session
	s.mutex.Unlock()

	if session == nil {
		return failed("Nothing to record yet", "Press Start watching first, then record.")
	}

	session.engine.StartManualRecording()

	return succeeded("Recording now")
}

// TakeSnapshot saves the current picture as a photo.
func (s *Service) TakeSnapshot() ActionResult {
	s.mutex.Lock()
	session := s.session
	s.mutex.Unlock()

	if session == nil {
		return failed("Nothing to photograph yet", "Press Start watching first, then take a snapshot.")
	}

	session.engine.TakeSnapshot()

	return succeeded("Taking a snapshot…")
}

// StopManualRecording ends the manual clip in progress.
func (s *Service) StopManualRecording() ActionResult {
	s.mutex.Lock()
	session := s.session
	s.mutex.Unlock()

	if session == nil {
		return failed("Nothing is recording", "Press Start watching first.")
	}

	session.engine.StopManualRecording()

	return succeeded("Saving your recording")
}

// PreviewJPEG returns the latest frame, or nil when the camera is not running.
func (s *Service) PreviewJPEG() []byte {
	s.mutex.Lock()
	session := s.session
	s.mutex.Unlock()

	if session == nil {
		return nil
	}

	return session.engine.PreviewJPEG()
}

// Events returns the retained event history, newest first.
func (s *Service) Events() []EventView { return s.events.view() }

// LogLines returns the retained log records, oldest first.
func (s *Service) LogLines() []string { return s.log.Recent() }
