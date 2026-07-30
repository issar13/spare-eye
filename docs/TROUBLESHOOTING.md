# Troubleshooting

Start with the app's diagnostics screen. On Windows, logs are stored at
`%AppData%\spareeye\spareeye.log`; on Linux, at
`~/.config/spareeye/spareeye.log`.

For bot-specific problems, also see
[Telegram setup](SETUP-TELEGRAM.md#troubleshooting). For compiler and linker
problems, see [Development](DEVELOPMENT.md#troubleshooting).

## No usable camera is found

Only one process can hold a camera. Close SpareEye, video-call software, browser
tabs using the camera, and any other capture program before trying again.

Do not assume the physical webcam is index 0. Virtual camera drivers such as OBS
or Iriun often take the first index but deliver no frames. List only devices
that successfully hand over a real frame:

```bash
make cameras
```

Then select the reported camera in **Setup**, or pass it to the CLI:

```bash
go run ./cmd/spareeye-cli -camera 1
```

Camera discovery runs in a separate process so a faulty driver should stop only
the probe. If a driver repeatedly crashes discovery, disable that device in the
operating system or set `detection.cameraIndex` manually.

## The window closes immediately or never appears

The log is opened before the rest of startup, so its last record usually shows
the failing stage:

| Last record | Likely cause |
|---|---|
| No log, or only a log in `%Temp%` | The user configuration directory could not be located or written |
| `startup: loading settings` | `config.json` is unreadable, or the configured media directory is unavailable |
| `startup: opening the window` | The webview failed to start; on Windows, install Microsoft Edge WebView2 Runtime |
| `startup: window opened` | The window appeared, then failed while being prepared |
| `probing capture devices` | A camera driver interrupted enumeration |
| `panic` | An application bug; attach the stack trace to an issue |

Any error Go can report also raises a message box naming the log. An empty log
means the process stopped before it could write one.

## Telegram rejects a token that looks correct

Copy the token again from BotFather and make sure it has no surrounding
whitespace. SpareEye trims credentials on input, including the invisible
carriage return commonly left by a `.env` file created on Windows.

Never post a token in an issue or log excerpt. If it may have leaked, revoke it
with BotFather and create a replacement.

## Linux prints a wall of OpenCV warnings

Messages such as `can't open camera by index` from `cap_v4l.cpp` are OpenCV
reporting unused device indices during enumeration. They are harmless if
SpareEye still finds the intended camera.

## A completed clip reports dropped frames

Frames are dropped deliberately when the encoder cannot keep up, instead of
allowing capture to stall or memory use to grow without limit. Persistent
drops mean the machine cannot encode at the camera's current rate. Close other
CPU-heavy programs, try a lower-resolution camera mode, or use a faster disk or
computer.
