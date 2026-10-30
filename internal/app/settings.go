package app

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"spareeye/internal/autostart"
	"spareeye/internal/config"
	"spareeye/internal/detector"
	"spareeye/internal/notify"
)

// verifyTimeout bounds the Telegram round trip so a wrong host cannot leave the
// Setup screen spinning.
const verifyTimeout = 30 * time.Second

// StatusView is everything the arm/disarm screen shows.
//
// Headline, Tone and Explanation are the primary display; every other field is
// detail for the diagnostics panel, kept out of the way of somebody who only
// wants to know whether their camera is watching.
type StatusView struct {
	Headline    string `json:"headline"`
	Tone        string `json:"tone"`
	Explanation string `json:"explanation"`

	// Problem is set whenever the app cannot start watching, and always names
	// something the user can do about it.
	Problem *Problem `json:"problem"`

	Armed           bool `json:"armed"`
	Recording       bool `json:"recording"`
	RecordingManual bool `json:"recordingManual"`
	Night           bool `json:"night"`

	// MotionSeconds is how long the current motion has lasted, zero when still.
	// The detector reports an instant, but a timestamp crossing into JavaScript
	// would be compared against the browser's clock rather than Go's.
	MotionSeconds float64 `json:"motionSeconds"`

	CaptureFPS  float64 `json:"captureFPS"`
	FrameWidth  int     `json:"frameWidth"`
	FrameHeight int     `json:"frameHeight"`

	// CameraSource names the camera in words rather than as a number, because a
	// bare index says nothing about an installation watching a stream and an
	// address says nothing about one watching a webcam.
	CameraSource string `json:"cameraSource"`

	// AlertsEnabled is what the user asked for; AlertsConfigured is whether the
	// credentials to honour it are actually present. Setup needs both, so that
	// "I turned alerts off" is not mistaken for "I have not finished".
	AlertsEnabled     bool `json:"alertsEnabled"`
	AlertsConfigured  bool `json:"alertsConfigured"`
	TelegramConnected bool `json:"telegramConnected"`

	// LaunchAtLogin is what the user asked for; LaunchAtLoginRegistered is what
	// the operating system will actually do. They disagree when security
	// software has removed the registration, or when the binary has moved since
	// it was made, and the screen has to show the second one.
	LaunchAtLogin           bool `json:"launchAtLogin"`
	LaunchAtLoginRegistered bool `json:"launchAtLoginRegistered"`

	CanArm bool `json:"canArm"`

	// The laptop's own condition. Pointers, so that JSON says null for what
	// the machine could not report and the app bar hides that chip instead of
	// showing a made-up zero.
	BatteryPercent     *int     `json:"batteryPercent"`
	OnMains            bool     `json:"onMains"`
	DiskFreeBytes      *uint64  `json:"diskFreeBytes"`
	TemperatureCelsius *float64 `json:"temperatureCelsius"`

	// LastFailure is the engine's own error text. It is diagnostics only and
	// must never be the message a user is asked to act on.
	LastFailure string `json:"lastFailure"`

	LogPath        string `json:"logPath"`
	MediaDirectory string `json:"mediaDirectory"`
}

// SettingsView mirrors the persisted config, with every duration expressed in
// seconds. config.Duration serialises as "15s", which is friendly in a
// hand-edited file and hostile in a number input.
type SettingsView struct {
	Telegram TelegramForm `json:"telegram"`
	Tuning   TuningForm   `json:"tuning"`
	LiveView LiveViewForm `json:"liveView"`
	Startup  StartupForm  `json:"startup"`

	CameraIndex int    `json:"cameraIndex"`
	CameraURL   string `json:"cameraURL"`

	// CameraChosen is what tells Setup to wait for an answer instead of showing
	// the shipped default as though somebody had picked it.
	CameraChosen bool `json:"cameraChosen"`

	MediaDirectory string `json:"mediaDirectory"`
}

// TelegramForm is the Setup screen's credentials.
type TelegramForm struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatID"`
	Enabled  bool   `json:"enabled"`
}

// TuningForm is the sensitivity and schedule screen.
type TuningForm struct {
	Day   SensitivityForm `json:"day"`
	Night SensitivityForm `json:"night"`

	NightStartMinutes int `json:"nightStartMinutes"`
	NightEndMinutes   int `json:"nightEndMinutes"`

	MotionPersistSeconds float64 `json:"motionPersistSeconds"`
	PrebufferLeadSeconds float64 `json:"prebufferLeadSeconds"`
	ClipDurationSeconds  float64 `json:"clipDurationSeconds"`

	RetentionDays        int `json:"retentionDays"`
	MinimumFreeGigabytes int `json:"minimumFreeGigabytes"`
}

// LiveViewForm is the Live view screen's speed and sharpness. Both govern the
// picture on screen only; neither touches what is written to disk, which is why
// they belong to their own form and their own save.
type LiveViewForm struct {
	PreviewFPS     int `json:"previewFPS"`
	PreviewQuality int `json:"previewQuality"`
}

// StartupForm is the Setup screen's two startup switches. They are saved
// together because they are read together, but neither implies the other.
type StartupForm struct {
	LaunchAtLogin bool `json:"launchAtLogin"`
	WatchOnLaunch bool `json:"watchOnLaunch"`
}

// SensitivityForm is one lighting profile.
type SensitivityForm struct {
	MinimumContourArea    float64 `json:"minimumContourArea"`
	ThresholdValue        float64 `json:"thresholdValue"`
	SampleIntervalSeconds float64 `json:"sampleIntervalSeconds"`
	AlertCooldownSeconds  float64 `json:"alertCooldownSeconds"`
}

func settingsView(settings config.Config) SettingsView {
	return SettingsView{
		Telegram: TelegramForm{
			BotToken: settings.Telegram.BotToken,
			ChatID:   settings.Telegram.ChatID,
			Enabled:  settings.Telegram.Enabled,
		},
		Tuning: TuningForm{
			Day:                  sensitivityForm(settings.Detection.Day),
			Night:                sensitivityForm(settings.Detection.Night),
			NightStartMinutes:    settings.Detection.NightStartMinutes,
			NightEndMinutes:      settings.Detection.NightEndMinutes,
			MotionPersistSeconds: settings.Detection.MotionPersist.AsDuration().Seconds(),
			PrebufferLeadSeconds: settings.Detection.PrebufferLead.AsDuration().Seconds(),
			ClipDurationSeconds:  settings.Detection.ClipDuration.AsDuration().Seconds(),
			RetentionDays:        settings.Storage.RetentionDays,
			MinimumFreeGigabytes: settings.Storage.MinimumFreeGigabytes,
		},
		LiveView: LiveViewForm{
			PreviewFPS:     settings.Detection.PreviewFPS,
			PreviewQuality: settings.Detection.PreviewQuality,
		},
		Startup: StartupForm{
			LaunchAtLogin: settings.Startup.LaunchAtLogin,
			WatchOnLaunch: settings.Startup.WatchOnLaunch,
		},
		CameraIndex:    settings.Detection.CameraIndex,
		CameraURL:      settings.Detection.CameraURL,
		CameraChosen:   settings.Detection.CameraChosen,
		MediaDirectory: settings.Storage.MediaDirectory,
	}
}

// describeCameraSource is the camera as the diagnostics panel shows it.
func describeCameraSource(settings config.DetectionSettings) string {
	if !settings.CameraChosen {
		return "Not chosen yet"
	}
	if settings.CameraURL != "" {
		return settings.CameraURL
	}

	return fmt.Sprintf("Camera %d", settings.CameraIndex)
}

// loggableCameraSource names the camera for the log file, which is the one
// place the address must not appear: a stream address commonly carries the
// camera's user name and password in front of the host, and the log is what
// gets pasted into a bug report.
func loggableCameraSource(settings config.DetectionSettings) string {
	if settings.CameraURL != "" {
		return "a network stream"
	}

	return fmt.Sprintf("camera %d", settings.CameraIndex)
}

func sensitivityForm(profile config.SensitivityProfile) SensitivityForm {
	return SensitivityForm{
		MinimumContourArea:    profile.MinimumContourArea,
		ThresholdValue:        profile.ThresholdValue,
		SampleIntervalSeconds: profile.SampleInterval.AsDuration().Seconds(),
		AlertCooldownSeconds:  profile.AlertCooldown.AsDuration().Seconds(),
	}
}

// SaveTelegram stores credentials and reconnects the notifier. It does not
// contact Telegram; TestTelegram does that.
func (s *Service) SaveTelegram(form TelegramForm) ActionResult {
	return s.saveTelegram(form, true)
}

// saveTelegram can trust a chat ID only when TestTelegram has just discovered
// and verified it. A plain form save must discard an ID tied to an old token.
func (s *Service) saveTelegram(form TelegramForm, clearChatOnTokenChange bool) ActionResult {
	form = normalizedTelegramForm(form)

	s.mutex.Lock()
	updated := s.settings
	// A chat ID belongs to one particular bot's update stream. Keeping the old
	// hidden ID after the token changes could silently send footage to a chat
	// belonging to the previous bot instead of discovering the new one.
	if clearChatOnTokenChange && form.BotToken != strings.TrimSpace(updated.Telegram.BotToken) {
		form.ChatID = ""
	}
	updated.Telegram = config.TelegramSettings{
		BotToken: form.BotToken,
		ChatID:   form.ChatID,
		Enabled:  form.Enabled,
	}
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	s.log.Info("telegram settings saved", "enabled", form.Enabled, "configured", updated.ConfiguredForAlerts())
	s.restartMessenger()

	if !form.Enabled {
		return succeeded("Saved. Telegram alerts are turned off.")
	}

	return succeeded("Saved")
}

// TestTelegram connects the bot and checks it can deliver a message. On a new
// setup it discovers the chat from the message the user sent to their bot, so
// nobody has to find or copy Telegram's internal chat ID.
func (s *Service) TestTelegram(form TelegramForm) ActionResult {
	form = normalizedTelegramForm(form)
	if form.BotToken == "" {
		return failed(
			"The bot token is missing",
			"Paste the token from @BotFather, then open your new bot in Telegram and press Start.",
		).withDoc(telegramSetupDoc)
	}

	// The setup screen no longer exposes the saved chat ID. When the token is
	// replaced, that ID belongs to the previous bot and must not be reused for
	// verification (or persisted with the new token). Discover the conversation
	// that contacted the replacement bot instead.
	s.mutex.Lock()
	savedTelegram := s.settings.Telegram
	s.mutex.Unlock()
	form = telegramFormForConnection(form, savedTelegram)

	testContext, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	telegram := notify.NewTelegram(form.BotToken, form.ChatID)
	defer telegram.Close()
	if form.ChatID == "" {
		chatID, err := telegram.DiscoverChatID(testContext)
		if err != nil {
			s.log.Warn("telegram chat discovery failed", "error", err)
			if strings.Contains(err.Error(), "multiple Telegram conversations") {
				return failed(
					"More than one chat messaged this bot",
					"Use a fresh bot for SpareEye, press Start in that bot's private chat, then connect again.",
				).withDoc(telegramSetupDoc)
			}
			if telegramDiscoveryCredentialsFailed(err) {
				return describeTelegramFailure(err).asResult()
			}
			return failed(
				"SpareEye cannot see your Telegram chat yet",
				"Open your new bot in Telegram, press Start or send it a message, then come back and connect again.",
			).withDoc(telegramSetupDoc)
		}
		form.ChatID = chatID
	}

	if err := telegram.Verify(testContext); err != nil {
		s.log.Warn("telegram verification failed", "error", err)
		return describeTelegramFailure(err).asResult()
	}

	s.log.Info("telegram verified")
	if result := s.saveTelegram(form, false); !result.OK {
		return result
	}

	return succeeded("Telegram is connected. Check your phone for the test message.")
}

func telegramFormForConnection(form TelegramForm, saved config.TelegramSettings) TelegramForm {
	form = normalizedTelegramForm(form)
	if form.BotToken != strings.TrimSpace(saved.BotToken) {
		form.ChatID = ""
	}
	return form
}

func normalizedTelegramForm(form TelegramForm) TelegramForm {
	form.BotToken = strings.TrimSpace(form.BotToken)
	form.ChatID = strings.TrimSpace(form.ChatID)
	return form
}

// telegramDiscoveryCredentialsFailed distinguishes an API refusal from the
// ordinary "no messages yet" result. Discovery happens before Verify when a
// token changes, but an invalid token still needs Verify's BotFather advice
// rather than instructions to message a bot that Telegram has rejected.
func telegramDiscoveryCredentialsFailed(failure error) bool {
	message := failure.Error()
	return strings.Contains(message, "bot token rejected") ||
		strings.Contains(message, "Telegram returned status") ||
		strings.Contains(message, "Telegram reported not-ok")
}

// SelectCamera changes the capture device, restarting the detector when armed
// because the device is chosen once, at open time.
//
// Choosing a device puts aside any network stream: the two are alternatives, and
// leaving an address behind would mean the picker on screen no longer named what
// was being watched.
func (s *Service) SelectCamera(index int) ActionResult {
	s.mutex.Lock()
	updated := s.settings
	updated.Detection.CameraIndex = index
	updated.Detection.CameraURL = ""
	updated.Detection.CameraChosen = true
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	s.log.Info("camera selected", "index", index)

	if problem := s.restartDetector(); problem != nil {
		return problem.asResult()
	}

	return succeeded("Camera selected")
}

// TestCameraURL opens an address the user has typed but not saved, so a wrong
// one is caught before it becomes the camera a house is watched by.
func (s *Service) TestCameraURL(address string) ActionResult {
	stream, problem := s.openStreamForTesting(address)
	if problem != nil {
		return problem.asResult()
	}

	return succeeded(fmt.Sprintf("It works. That camera is sending a %d × %d picture.", stream.Width, stream.Height))
}

// SelectCameraURL adopts a network stream as the camera. It is opened first: an
// address that has never delivered a frame is not something to persist as the
// source a watch will be started from.
func (s *Service) SelectCameraURL(address string) ActionResult {
	if _, problem := s.openStreamForTesting(address); problem != nil {
		return problem.asResult()
	}

	s.mutex.Lock()
	updated := s.settings
	updated.Detection.CameraURL = strings.TrimSpace(address)
	updated.Detection.CameraChosen = true
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	// The address itself is kept out of the log: a stream address commonly
	// carries the camera's user name and password in front of the host.
	s.log.Info("network stream selected")

	if problem := s.restartDetector(); problem != nil {
		return problem.asResult()
	}

	return succeeded("Network stream selected")
}

// openStreamForTesting checks the address and then opens it in a child process,
// which is the same isolation device enumeration gets and for the same reason: a
// stream can hang or crash the capture backend, and the app has a window open.
func (s *Service) openStreamForTesting(address string) (detector.Camera, *Problem) {
	if problem := validateCameraURL(address); problem != nil {
		return detector.Camera{}, problem
	}

	stream, err := detector.TestStreamIsolated(s.log.Logger, strings.TrimSpace(address))
	if err != nil {
		// Only that it failed, never why: the backend's message quotes the
		// address back, and an address commonly has a password in it.
		s.log.Warn("a network stream did not send a picture")

		problem := streamUnavailable()
		return detector.Camera{}, &problem
	}

	return stream, nil
}

// SaveTuning stores the sensitivity and schedule settings. The prebuffer is
// sized from the frame rate measured at startup, so applying a new lead time
// means recreating the detector.
func (s *Service) SaveTuning(form TuningForm) ActionResult {
	if problem := validateTuning(form); problem != nil {
		return problem.asResult()
	}

	s.mutex.Lock()
	updated := s.settings
	updated.Detection.Day = sensitivityProfile(form.Day)
	updated.Detection.Night = sensitivityProfile(form.Night)
	updated.Detection.NightStartMinutes = form.NightStartMinutes
	updated.Detection.NightEndMinutes = form.NightEndMinutes
	updated.Detection.MotionPersist = seconds(form.MotionPersistSeconds)
	updated.Detection.PrebufferLead = seconds(form.PrebufferLeadSeconds)
	updated.Detection.ClipDuration = seconds(form.ClipDurationSeconds)
	updated.Storage.RetentionDays = form.RetentionDays
	updated.Storage.MinimumFreeGigabytes = form.MinimumFreeGigabytes
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	s.log.Info("tuning saved",
		"nightStart", form.NightStartMinutes,
		"nightEnd", form.NightEndMinutes,
		"clipSeconds", form.ClipDurationSeconds,
		"retentionDays", form.RetentionDays,
		"minimumFreeGigabytes", form.MinimumFreeGigabytes,
	)

	if problem := s.restartDetector(); problem != nil {
		return problem.asResult()
	}

	return succeeded("Settings saved")
}

// SaveLiveView stores the live-view speed and sharpness and hands them straight
// to a running detector. Nothing about the picture on screen depends on how the
// camera was opened, so this applies without interrupting the watch.
func (s *Service) SaveLiveView(form LiveViewForm) ActionResult {
	if problem := validateLiveView(form); problem != nil {
		return problem.asResult()
	}

	s.mutex.Lock()
	updated := s.settings
	updated.Detection.PreviewFPS = form.PreviewFPS
	updated.Detection.PreviewQuality = form.PreviewQuality
	session := s.session
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	if session != nil {
		session.engine.SetPreview(form.PreviewFPS, form.PreviewQuality)
	}

	s.log.Info("live view saved", "previewFPS", form.PreviewFPS, "previewQuality", form.PreviewQuality)

	return succeeded("Live view updated")
}

// SaveStartup stores the two startup switches and makes the operating system
// agree with the first of them.
//
// The settings are written before the registration is attempted, so that a
// refusal from the operating system does not also lose the choice about
// watching on launch. What the system will really do is read back by Status,
// which is what the screen shows.
func (s *Service) SaveStartup(form StartupForm) ActionResult {
	s.mutex.Lock()
	updated := s.settings
	updated.Startup = config.StartupSettings{
		LaunchAtLogin: form.LaunchAtLogin,
		WatchOnLaunch: form.WatchOnLaunch,
	}
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	s.log.Info("startup settings saved",
		"launchAtLogin", form.LaunchAtLogin, "watchOnLaunch", form.WatchOnLaunch)

	register := autostart.Disable
	if form.LaunchAtLogin {
		register = autostart.Enable
	}

	if err := register(); err != nil {
		s.log.Error("registering for login failed", "wanted", form.LaunchAtLogin, "error", err)
		return loginRegistrationRefused(form.LaunchAtLogin).asResult()
	}

	return succeeded("Saved")
}

// validateTuning rejects values that would make the detector behave absurdly.
// This is the boundary where numbers a user typed become engine settings, and
// each rejection names the control on screen rather than the field underneath.
func validateTuning(form TuningForm) *Problem {
	if problem := validateProfile("Daytime", form.Day); problem != nil {
		return problem
	}
	if problem := validateProfile("Night-time", form.Night); problem != nil {
		return problem
	}

	if form.NightStartMinutes < 0 || form.NightStartMinutes > 1439 {
		return &Problem{Headline: "Night mode needs a start time", NextStep: "Pick a time for “Night mode from”."}
	}
	if form.NightEndMinutes < 0 || form.NightEndMinutes > 1439 {
		return &Problem{Headline: "Night mode needs an end time", NextStep: "Pick a time for “Night mode until”."}
	}
	if form.MotionPersistSeconds <= 0 {
		return &Problem{
			Headline: "Movement has to last at least a moment",
			NextStep: "Set “Only record if movement lasts longer than” to 1 second or more.",
		}
	}
	if form.PrebufferLeadSeconds < 0 {
		return &Problem{
			Headline: "That head start cannot be negative",
			NextStep: "Set the seconds to include before motion starts to 0 or more.",
		}
	}
	if form.ClipDurationSeconds <= 0 {
		return &Problem{
			Headline: "Recordings need a length",
			NextStep: "Set “Record for” to at least 1 second.",
		}
	}
	if form.RetentionDays < 0 {
		return &Problem{
			Headline: "That is not a number of days",
			NextStep: "Set how long to keep recordings to 0 or more days. 0 keeps them forever.",
		}
	}
	if form.MinimumFreeGigabytes < 0 {
		return &Problem{
			Headline: "That is not an amount of free space",
			NextStep: "Set the free space to keep to 0 or more gigabytes. 0 turns it off.",
		}
	}

	return nil
}

// cameraURLSchemes are the ones a capture backend can actually open. Anything
// else (a local file path, a device node, a scheme FFMPEG does not speak) is
// turned away here rather than at the point where it would hang a driver.
var cameraURLSchemes = []string{"rtsp", "rtsps", "http", "https"}

// validateCameraURL is the boundary where an address somebody typed becomes
// something a capture backend is asked to open.
func validateCameraURL(address string) *Problem {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return &Problem{
			Headline: "You have not entered an address yet",
			NextStep: "Type the address of your network camera, then try again. It usually looks like " +
				"rtsp://192.168.1.50:554/stream.",
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || !slices.Contains(cameraURLSchemes, strings.ToLower(parsed.Scheme)) {
		return &Problem{
			Headline: "That is not an address SpareEye can use",
			NextStep: "A network camera address has to begin with rtsp://, rtsps://, http:// or https:// and " +
				"name the camera after it, as in rtsp://192.168.1.50:554/stream. Your camera's manual or " +
				"its own settings page will tell you the exact address.",
		}
	}

	return nil
}

func validateLiveView(form LiveViewForm) *Problem {
	if form.PreviewFPS < config.MinPreviewFPS || form.PreviewFPS > config.MaxPreviewFPS {
		return &Problem{
			Headline: "That live-view speed is out of range",
			NextStep: "Choose a smoothness, or set the frames per second between 1 and 60 under Advanced.",
		}
	}
	if form.PreviewQuality < config.MinPreviewQuality || form.PreviewQuality > config.MaxPreviewQuality {
		return &Problem{
			Headline: "That live-view sharpness is out of range",
			NextStep: "Choose a smoothness, or set the picture quality between 30 and 95 under Advanced.",
		}
	}

	return nil
}

func validateProfile(name string, profile SensitivityForm) *Problem {
	if profile.MinimumContourArea <= 0 || profile.ThresholdValue <= 0 || profile.ThresholdValue > 255 {
		return &Problem{
			Headline: name + " sensitivity is out of range",
			NextStep: "Choose Low, Medium or High. If you changed the advanced numbers, reset them there.",
		}
	}
	if profile.SampleIntervalSeconds <= 0 {
		return &Problem{
			Headline: name + " checking speed is out of range",
			NextStep: "Under Advanced, set how often to check the picture to more than 0 seconds.",
		}
	}
	if profile.AlertCooldownSeconds < 0 {
		return &Problem{
			Headline: "That alert gap cannot be negative",
			NextStep: "Set “Don’t alert me more than once every” to 0 minutes or more.",
		}
	}

	return nil
}

// couldNotSave covers a failed write to the settings file, which in practice
// means a full disk or a permissions problem on the configuration directory.
func couldNotSave() ActionResult {
	return failed(
		"Your settings could not be saved",
		"This computer would not let the settings file be written. Check that your disk is not full, "+
			"then try again.",
	)
}

func sensitivityProfile(form SensitivityForm) config.SensitivityProfile {
	return config.SensitivityProfile{
		MinimumContourArea: form.MinimumContourArea,
		ThresholdValue:     form.ThresholdValue,
		SampleInterval:     seconds(form.SampleIntervalSeconds),
		AlertCooldown:      seconds(form.AlertCooldownSeconds),
	}
}

func seconds(count float64) config.Duration {
	return config.Duration(time.Duration(count * float64(time.Second)))
}

// persist writes settings to disk and only then adopts them, so a failed write
// never leaves the running app disagreeing with the file it will reload from.
func (s *Service) persist(updated config.Config) error {
	if err := config.Save(updated); err != nil {
		s.log.Error("saving settings failed", "error", err)
		return err
	}

	s.mutex.Lock()
	s.settings = updated
	s.mutex.Unlock()

	return nil
}
