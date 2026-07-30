# Kaa Rada

Turn a spare laptop into a home security camera.

Leave a laptop at home, arm it, and it watches the room. When something moves it
saves a snapshot, records a clip if the movement continues, and sends both to
your phone on Telegram. You can message it back to start a recording on demand
or pull up an earlier clip.

It works with any camera your machine can see: the built-in webcam, a USB camera
you plugged in, or a better camera you already own. You pick which one on the
Setup screen.

No cameras to buy, no subscription, no cloud account. It runs on hardware most
people already have in a drawer, which is the point.

Detection sensitivity switches between a day profile and a night profile on a
schedule you set, because a room at 19:00 and the same room at noon do not need
the same thresholds.

## Status

The detection engine is finished and tested. Clips, snapshots, Telegram alerts,
remote commands, and camera enumeration all work, and the desktop app runs on
both Windows and Linux.

CI builds and publishes the desktop app for Windows and Linux. Its screens cover
Telegram setup, arming and disarming, the live view and its speed and quality,
both sensitivity profiles, the day/night schedule, retention, and a browser for
saved clips and snapshots. Every one is a real control, so editing `config.json`
by hand is a fallback rather than the way in. `storage.mediaDirectory` is the
only setting the UI shows without letting you change it.

Some camera drivers crash when their device is opened, and on Windows that used
to take the whole app down during startup: it would appear and vanish with
nothing in the log after `probing capture devices`. Enumeration now runs in a
separate process, so a driver that crashes takes down only that process. The app
notes which device killed it, carries on with the next one, and still finds your
working camera. Virtual camera drivers such as Iriun and OBS are the usual
culprits, and they commonly sit at index 0.

Telegram is the only alerting channel. See [Why only Telegram](#why-only-telegram).

## Privacy, read this first

This app records video of whatever the camera can see, and it uploads clips and
snapshots to Telegram's servers to deliver them to you. Footage leaves your
machine.

If you point the camera at a space other people use, a shared flat or an office
or a room with guests in it, those people are being recorded and their footage
is being uploaded, and they have not agreed to either. Depending on where you
live, recording other people without telling them may be illegal regardless of
whose camera or whose home it is.

- Anyone who obtains your bot token can receive your footage and command your
  camera. Treat it like a password. See
  [docs/SETUP-TELEGRAM.md](docs/SETUP-TELEGRAM.md).
- Clips are also kept on local disk, for 30 days by default. Anyone with access
  to your user account can watch them.

## Requirements

| | |
|---|---|
| Platforms | Windows and Linux |
| Go | Whatever `go.mod` pins, which is the single source of truth |
| OpenCV | 4.6.0, to match gocv v0.31.0 |
| Camera | One that is not already in use by another program |
| Telegram | Your own bot, one per installation |

macOS is not supported. Shipping a macOS build that will open at all requires
paid Apple Developer enrollment, so it is deferred rather than half-done.

Binaries ship **unsigned**. On Windows, SmartScreen will warn that the publisher
is unknown and you will have to choose to run it anyway. Code-signing
certificates are an ongoing cost the project has not taken on.

## Getting started

### 1. Create your own Telegram bot

There is no shared bot and no token baked into the binary. Every installation
talks to a bot you create, so your footage only reaches your own chat.

Kaa Rada connects directly from this computer to Telegram's API. There is no
Kaa Rada server, VPS, relay, webhook, public port, or account shared with other
installations. The locally saved bot token is the credential that makes that
private, direct connection possible; Telegram does not provide an API for an
application to create a bot on somebody's behalf.

Follow [docs/SETUP-TELEGRAM.md](docs/SETUP-TELEGRAM.md), which covers creating
the bot with @BotFather, copying the token, and connecting your chat. The
desktop app discovers the chat ID from the message you send the bot. It takes
about five minutes and needs no technical background.

### 2. Pick your camera

In the desktop app this is the Setup screen: it lists the cameras it found and
you choose one. The app does not pick for you, because the right camera is not
something it can guess.

From a terminal:

```
go run ./cmd/kaa-rada-cli -list-cameras
```

This prints only devices that both open *and* hand over a real frame:

```
Usable cameras (1):
  index 1 = 640x480
```

Do not assume index 0. A virtual camera driver such as Iriun or OBS registers a
device that opens successfully and then delivers nothing, and it commonly takes
index 0, pushing the real webcam higher.

### 3. Run it

Launch the desktop app, paste the token into Setup, press Connect Telegram to
confirm it works, choose your camera, and arm it. Nothing else needs a
terminal.

With credentials in the environment instead:

```
export TELEGRAM_BOT_TOKEN=...
export TELEGRAM_CHAT_ID=...
go run ./cmd/kaa-rada-cli -camera 1
```

On start it prints the media directory and camera, prunes recordings past their
retention age, verifies the Telegram credentials, then watches. Ctrl+C stops it;
a clip in progress is finished and closed properly first.

## The headless command-line tool

`cmd/kaa-rada-cli` is the detector without a window, for a machine with no
display: a laptop with the lid shut, or a box reached over SSH.

| Flag | Meaning |
|---|---|
| `-list-cameras` | Print every usable camera with its resolution, then exit |
| `-camera N` | Use device index `N`. Default `-1` picks the first usable device |
| `-no-telegram` | Run detection and write files, but send nothing |

Credentials come from `config.json` first. When that file has no token or chat
ID, `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are read from the environment
instead, see [.env.example](.env.example). If neither supplies credentials the
detector still runs and still writes snapshots and clips; it just sends nothing.

Convenience targets in the `Makefile`:

```
make cameras      # go run ./cmd/kaa-rada-cli -list-cameras
make run          # go run ./cmd/kaa-rada-cli
make test         # tests that need no camera
make test-camera  # camera integration tests, needs a free webcam
make help         # every target
```

## Telegram commands

Message your bot to control the detector while it runs. Only messages from the
chat ID you configured are acted on; anything else is discarded.

| Command | Effect |
|---|---|
| `/record` | Start a manual recording. It runs until you stop it |
| `/stop` | End the manual recording |
| `/get` | List saved clips, newest first, numbered |

The leading slash is optional. `record`, `stop`, and `get` work identically,
`list` is a synonym for `get`, commands are case-insensitive, and surrounding
whitespace is ignored.

`/get` replies with a numbered list showing each clip's name, timestamp, and
size, newest first. **Reply with just the number** to have that clip sent:

```
📹 Recordings, newest first:

1. manual_2026-07-25_21-04-11.mp4
   2026-07-25 21:04:11 (1.8 MB)

2. motion_2026-07-25_19-52-03.mp4
   2026-07-25 19:52:03 (0.9 MB)

Reply with a number from 1 to 2 to receive it.
```

`/stop` only ends a *manual* recording. An automatic clip runs for its
configured duration and finishes on its own; a `/stop` arriving during one is
ignored rather than remembered, so it cannot silently cancel the next manual
recording you ask for.

## Where your data lives

Settings and media live in a per-user directory, not next to the binary:

| | Linux | Windows |
|---|---|---|
| Settings | `~/.config/kaa-rada/config.json` | `%AppData%\kaa-rada\config.json` |
| Snapshots | `~/.config/kaa-rada/media/snapshots/` | `%AppData%\kaa-rada\media\snapshots\` |
| Clips | `~/.config/kaa-rada/media/videos/` | `%AppData%\kaa-rada\media\videos\` |
| Log | `~/.config/kaa-rada/kaa-rada.log` | `%AppData%\kaa-rada\kaa-rada.log` |

The base directory is whatever the operating system reports as the user
configuration directory.

The reason for this rather than a folder beside the executable: an app installed
into `C:\Program Files` cannot write to its own directory without administrator
rights, so anything that tried would fail on a normal account. It would also
mean two users on one machine sharing one person's credentials and footage.

`config.json` is written with owner-only permissions, because it holds your bot
token. Saves go to a temporary file and are then renamed, so an interrupted
write cannot leave you with a truncated config and no credentials.

Snapshots are named `motion_YYYY-MM-DD_HH-MM-SS.jpg`. Clips are `motion_...mp4`
when triggered by movement and `manual_...mp4` when you asked for them, which is
how the listing tells them apart.

## Configuration

Settings live in `config.json` in the directory above, and almost all of them are
editable from the desktop app, so you should rarely need to open the file.

The ones people actually change:

| Field | Default | Meaning |
|---|---|---|
| `detection.cameraIndex` | `0` | Which camera to capture from. Confirm yours with `-list-cameras` before trusting the default |
| `storage.retentionDays` | `30` | Recordings older than this are deleted at startup. `0` keeps everything, which will eventually fill the disk |
| `startup.launchAtLogin` | `false` | Open the app when you log in |
| `startup.watchOnLaunch` | `false` | Begin watching as soon as the app opens |

`launchAtLogin` and `watchOnLaunch` are independent: the first opens the app and
leaves the camera off until you press Start watching, the second starts watching
whenever the app opens. Set the login switch on the **Setup** screen rather than
in the file, because the file only records the preference while the screen makes
the actual registration with your operating system.

Full reference for every field, including both sensitivity profiles and the
day/night schedule: [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Troubleshooting

**"no usable camera found", or a second launch fails**

Only one process can hold a camera. If the detector is already running, or a
previous run has not fully exited, or a video call has the webcam, a second
instance cannot open the device and fails with a backend error that does not say
so. Close the other user of the camera first.

**A camera is listed but the detector sees nothing, or index 0 does nothing**

Index 0 is frequently a virtual camera (Iriun, OBS) that opens successfully and
then delivers no frames. Run `go run ./cmd/kaa-rada-cli -list-cameras`, which
requires each device to hand over a real frame before reporting it, and pass the
index it gives you with `-camera N`.

**The bot token is rejected even though it looks correct**

A `.env` file written on Windows ends each line with a carriage return, invisible
in an editor but part of the value, which makes the request URL invalid.
Credentials are trimmed on the way in, so this case is handled, but watch for
trailing whitespace when pasting a token anywhere else.

**The window opens and closes again immediately, or never appears**

Open `%AppData%\kaa-rada\kaa-rada.log`, the same path the app shows on its
diagnostics screen. The log is opened before any other startup work and written
straight to disk, so its last record is the stage that failed:

| Last record | What it means |
|---|---|
| nothing at all, or a log only in `%Temp%` | `%AppData%` could not be located or written to |
| `startup: loading settings` | `config.json` is unreadable, or `storage.mediaDirectory` names a drive or share that no longer exists |
| `startup: opening the window` | the webview failed to start; install the Microsoft Edge WebView2 runtime |
| `startup: window opened` | the window appeared, then something failed while it was being prepared |
| `probing capture devices` | enumeration was interrupted. It runs in a separate process, so a crashing driver should no longer end the app. If it does, disable the camera named in Device Manager or set `detection.cameraIndex` by hand |
| `panic` | a bug. The record carries the stack; please attach it to an issue |

Any failure Go can report also raises a message box naming this file, so an empty
log means the process died before it could write one.

**A wall of OpenCV warnings on Linux**

Lines like `can't open camera by index` from `cap_v4l.cpp` are harmless. They are
OpenCV reporting each index that is not a capture device while enumerating. Linux
asks for the V4L2 backend directly, which already avoids a much longer page of
GStreamer pipeline warnings.

**A clip reports dropped frames**

A finished clip may note that frames were dropped because the encoder could not
keep up. Frames are dropped deliberately rather than allowed to accumulate, so a
slow disk cannot stall capture or grow memory without limit. Persistent dropping
means the machine cannot encode at the camera's rate.

## Building from source

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for toolchain setup, OpenCV
installation per platform, and the build and packaging targets.

For how the code is organised and why, see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Why only Telegram

WhatsApp's official Cloud API cannot do what this app needs without
infrastructure a local desktop app does not have. Sending an unprompted message,
which is exactly what a motion alert is, requires a Meta Business account and
message templates approved in advance, so an alert cannot simply be composed and
sent. Receiving commands requires a publicly reachable HTTPS webhook, which a
program on someone's home PC does not have and should not need.

The unofficial route works around both by driving WhatsApp's own protocol, but it
carries a terms-of-service and account-ban risk that belongs in front of a user
as an explicit choice, not shipped quietly in a release.

The `notify.Notifier` interface exists so another provider can be added without
touching the detector. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#adding-a-notifier).

## License

MIT. See [LICENSE](LICENSE).
