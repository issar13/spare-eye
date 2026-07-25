package app

import (
	"context"
	"sync"
	"time"

	"kaa-rada/internal/config"
	"kaa-rada/internal/detector"
	"kaa-rada/internal/media"
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

	cameras []detector.Camera
	// camerasScanned distinguishes "no camera found" from "not looked yet",
	// which decide whether arming is refused or merely unproven.
	camerasScanned bool

	events    eventHistory
	messenger messenger
}

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
	}
}

// Startup runs the work that should happen once the app is up: reclaiming disk
// space and connecting the notifier.
func (s *Service) Startup(ctx context.Context) {
	if s.instanceLockError != nil {
		s.log.Error("camera claim refused", "error", s.instanceLockError)
	}

	s.pruneExpiredMedia()
	s.restartMessenger()

	// Probing opens every device in turn, which takes long enough to be worth
	// keeping off the path that paints the window.
	go s.resolveCamera()
}

// resolveCamera makes the configured index one that actually delivers frames.
//
// A fresh config points at index 0, and on a typical laptop index 0 is as
// likely to be a virtual webcam — Iriun, OBS — that opens cleanly and then
// hands over nothing. Left alone, a first run would arm, wait, and time out.
func (s *Service) resolveCamera() {
	found := detector.AvailableCameras()

	s.mutex.Lock()
	s.cameras = found
	s.camerasScanned = true
	configured := s.settings.Detection.CameraIndex
	updated := s.settings
	s.mutex.Unlock()

	if len(found) == 0 {
		s.log.Warn("no usable camera found", "configured", configured)
		return
	}

	for _, camera := range found {
		if camera.Index == configured {
			s.log.Info("camera confirmed usable", "index", configured, "size", camera.Label)
			return
		}
	}

	s.log.Info("configured camera is unusable; adopting one that works",
		"configured", configured, "adopted", found[0].Index)

	updated.Detection.CameraIndex = found[0].Index
	s.persist(updated)
}

// Shutdown stops the detector and the notifier. Wails calls it while the window
// is closing, so a clip in progress still gets flushed to disk.
func (s *Service) Shutdown(ctx context.Context) {
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
		removed, err := media.Prune(directory, settings.Storage.RetentionDays)
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

	found := detector.AvailableCameras()
	s.log.Info("probed cameras", "count", len(found))

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
	s.mutex.Unlock()

	view := StatusView{
		CameraIndex:       settings.Detection.CameraIndex,
		AlertsEnabled:     settings.Telegram.Enabled,
		AlertsConfigured:  settings.ConfiguredForAlerts(),
		TelegramConnected: s.messenger.connected(),
		LogPath:           s.log.Path(),
		MediaDirectory:    settings.Storage.MediaDirectory,
		LastFailure:       lastFail,
	}

	blocked := s.armBlockedProblem(settings)
	if blocked == nil {
		view.CanArm = true
	} else {
		view.Problem = blocked
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
	if s.instanceLockError != nil {
		return &Problem{
			Headline: "Kaa Rada is already running",
			NextStep: "Another copy of this app has the camera. Switch to that window, or close it and try again.",
		}
	}

	s.mutex.Lock()
	noneFound := s.camerasScanned && len(s.cameras) == 0
	s.mutex.Unlock()

	if noneFound || settings.Detection.CameraIndex < 0 {
		problem := cameraUnavailable()
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
	s.mutex.Unlock()

	select {
	case <-session.opened:
		s.log.Info("armed", "camera", settings.Detection.CameraIndex)
		return succeeded("Watching for motion")

	case <-session.finished:
		s.forgetSession(session)
		s.noteStartupFailure(session.runError, settings.Detection.CameraIndex)
		return cameraUnavailable().asResult()

	case <-time.After(cameraOpenTimeout):
		s.forgetSession(session)
		session.cancel()
		<-session.finished
		s.log.Error("camera produced no frames", "camera", settings.Detection.CameraIndex)

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
			s.deliverSnapshot(event.MediaPath, event.Night)

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
func (s *Service) noteStartupFailure(runError error, cameraIndex int) {
	s.log.Error("arming failed", "camera", cameraIndex, "error", runError)

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
