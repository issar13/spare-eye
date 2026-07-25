package app

import (
	"context"
	"time"

	"motion-detector/internal/config"
	"motion-detector/internal/notify"
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

	CameraIndex int `json:"cameraIndex"`

	// AlertsEnabled is what the user asked for; AlertsConfigured is whether the
	// credentials to honour it are actually present. Setup needs both, so that
	// "I turned alerts off" is not mistaken for "I have not finished".
	AlertsEnabled     bool `json:"alertsEnabled"`
	AlertsConfigured  bool `json:"alertsConfigured"`
	TelegramConnected bool `json:"telegramConnected"`

	CanArm bool `json:"canArm"`

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

	CameraIndex    int    `json:"cameraIndex"`
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

	RetentionDays int `json:"retentionDays"`
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
		},
		CameraIndex:    settings.Detection.CameraIndex,
		MediaDirectory: settings.Storage.MediaDirectory,
	}
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
	s.mutex.Lock()
	updated := s.settings
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

// TestTelegram checks credentials the user has typed but may not have saved
// yet, so a typo is caught before it is written to disk.
func (s *Service) TestTelegram(form TelegramForm) ActionResult {
	if form.BotToken == "" || form.ChatID == "" {
		return failed(
			"Both boxes need filling in",
			"Enter the bot token from @BotFather and your chat ID from @userinfobot, then test again.",
		).withDoc(telegramSetupDoc)
	}

	testContext, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	telegram := notify.NewTelegram(form.BotToken, form.ChatID)
	defer telegram.Close()

	if err := telegram.Verify(testContext); err != nil {
		s.log.Warn("telegram verification failed", "error", err)
		return describeTelegramFailure(err).asResult()
	}

	s.log.Info("telegram verified")

	return succeeded("It works — check Telegram for a test message from your bot.")
}

// SelectCamera changes the capture device, restarting the detector when armed
// because the device is chosen once, at open time.
func (s *Service) SelectCamera(index int) ActionResult {
	s.mutex.Lock()
	updated := s.settings
	updated.Detection.CameraIndex = index
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
	s.mutex.Unlock()

	if s.persist(updated) != nil {
		return couldNotSave()
	}

	s.log.Info("tuning saved",
		"nightStart", form.NightStartMinutes,
		"nightEnd", form.NightEndMinutes,
		"clipSeconds", form.ClipDurationSeconds,
		"retentionDays", form.RetentionDays,
	)

	if problem := s.restartDetector(); problem != nil {
		return problem.asResult()
	}

	return succeeded("Settings saved")
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
