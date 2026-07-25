# Kaa Rada

Turn a spare laptop into a home security camera.

Leave a laptop at home, arm it, and it watches the room through the built-in
webcam. When something moves it saves a snapshot, records a clip if the movement
continues, and sends both to your phone on Telegram. You can message it back to
start a recording on demand or pull up an earlier clip.

No cameras to buy, no subscription, no cloud account. It runs on hardware most
people already have sitting in a drawer, which is the point: home monitoring
that costs nothing beyond a laptop you were not using.

Detection sensitivity switches between a day profile and a night profile on a
schedule you set, because a room at 19:00 and the same room at noon do not need
the same thresholds.

## Status

The detection engine is finished and tested — clips, snapshots, Telegram
alerts, remote commands, and camera enumeration all work and have been verified
on Linux.

The desktop user interface is **in progress**. Until it lands, the way to run
the detector is the headless command-line tool described below, which is real
and works today. Configuration values that the UI will eventually expose as
controls are editable now by hand in `config.json`.

Telegram is the only alerting channel. See
[Why only Telegram](#why-only-telegram).

## Privacy — read this first

This app records video of whatever the camera can see, and it uploads those
clips and snapshots to Telegram's servers as part of delivering them to you.
Footage leaves your machine.

If you point the camera at a space that other people use — a shared flat, an
office, a room with guests in it — those people are being recorded and their
footage is being uploaded, and they have not agreed to either. Depending on
where you live, recording other people without telling them may be illegal
regardless of whose camera it is or whose home it is in.

Two consequences worth stating plainly:

- Anyone who obtains your bot token can receive your footage and command your
  camera. Treat it like a password. See
  [docs/SETUP-TELEGRAM.md](docs/SETUP-TELEGRAM.md).
- Clips are kept on local disk as well, for 30 days by default. Anyone with
  access to your user account can watch them.

## Requirements

| | |
|---|---|
| Platforms | Windows and Linux |
| Go | 1.24.4 (pinned in `go.mod`) |
| OpenCV | 4.6.0, to match gocv v0.31.0 |
| Camera | A webcam that is not already in use by another program |
| Telegram | Your own bot — one per installation |

macOS is deliberately not supported. Distributing a macOS build that will open
at all requires paid Apple Developer enrollment, so it is deferred rather than
half-done.

Windows and Linux binaries ship **unsigned**. On Windows, SmartScreen will warn
you that the publisher is unknown and you will have to choose to run it anyway.
That is expected; code-signing certificates are an ongoing cost the project has
not taken on.

## Getting started

### 1. Create your own Telegram bot

There is no shared bot and no bot token baked into the binary. Every
installation talks to a bot you create, so your footage only ever reaches your
own chat.

Follow [docs/SETUP-TELEGRAM.md](docs/SETUP-TELEGRAM.md). It walks through
creating the bot with @BotFather, copying the token, and finding your chat ID.
It takes about five minutes and needs no technical background.

### 2. Find your camera

```
go run ./cmd/kaa-rada-cli -list-cameras
```

This prints only the devices that both open *and* hand over a real frame:

```
Usable cameras (1):
  index 1 — 640x480
```

Do not assume index 0. A virtual camera driver such as Iriun or OBS registers a
device that opens successfully and then delivers nothing, and it commonly takes
index 0, pushing the real webcam to a higher index.

### 3. Run it

With credentials in the environment:

```
export TELEGRAM_BOT_TOKEN=...
export TELEGRAM_CHAT_ID=...
go run ./cmd/kaa-rada-cli -camera 1
```

On start it prints the media directory and camera index, prunes recordings past
their retention age, verifies the Telegram credentials, and then watches. Press
Ctrl+C to stop; a clip in progress is finished and closed properly before exit.

## The headless command-line tool

`cmd/kaa-rada-cli` is the detector without a window. It exists so the engine can be
run on a machine with no display, and it is the supported way to use the app
until the desktop UI is finished.

| Flag | Meaning |
|---|---|
| `-list-cameras` | Print every usable camera with its resolution, then exit |
| `-camera N` | Use device index `N`. Default is `-1`, which picks the first usable device |
| `-no-telegram` | Run detection and write files, but send nothing |

Credentials come from `config.json` first. When that file has no token or chat
ID, `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are read from the environment
instead — see [.env.example](.env.example). If neither supplies credentials,
the detector still runs and still writes snapshots and clips to disk; it just
does not send them.

Convenience targets in the `Makefile`:

```
make cameras      # go run ./cmd/kaa-rada-cli -list-cameras
make run          # go run ./cmd/kaa-rada-cli
make test         # tests that need no camera
make test-camera  # camera integration tests, needs a free webcam
make help         # every target
```

## Telegram commands

Message your bot to control the detector while it is running. Only messages
from the chat ID you configured are acted on; anything from another chat is
discarded.

| Command | Effect |
|---|---|
| `/record` | Start a manual recording. It runs until you stop it |
| `/stop` | End the manual recording |
| `/get` | List saved clips, newest first, numbered |

The leading slash is optional — `record`, `stop`, and `get` work identically,
and `list` is accepted as a synonym for `get`. Commands are case-insensitive
and surrounding whitespace is ignored.

`/get` replies with a numbered list showing each clip's name, timestamp, and
size in megabytes, newest first. **Reply with just the number** to have that
clip sent to you:

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

The exact base directory is whatever the operating system reports as the user
configuration directory.

The reason for this, rather than a folder beside the executable: an app
installed into `C:\Program Files` cannot write to its own directory without
administrator rights. Anything that tried would fail on a normal user account,
and it would also mean two users on one machine sharing one person's
credentials and footage.

`config.json` is written with owner-only permissions, because it holds your bot
token. Saves go to a temporary file and are then renamed, so an interrupted
write cannot leave you with a truncated config and no credentials.

Snapshots are named `motion_YYYY-MM-DD_HH-MM-SS.jpg`. Clips are
`motion_...mp4` when triggered by movement and `manual_...mp4` when you asked
for them, which is how the listing tells them apart.

## Configuration

Settings live in `config.json` in the directory above, and will be editable
from the desktop UI. Durations are written as readable strings — `"15s"`,
`"5m0s"` — so the file stays sensible to edit by hand. Any field you leave out
keeps its default, so an older `config.json` remains loadable after an upgrade.

### Telegram

| Field | Default | Meaning |
|---|---|---|
| `telegram.botToken` | empty | Your bot's token from @BotFather |
| `telegram.chatID` | empty | The only chat that receives alerts and may send commands |
| `telegram.enabled` | `true` | Turn alerting off without deleting your credentials |

### Detection

| Field | Default | Meaning |
|---|---|---|
| `detection.cameraIndex` | `0` | Device index to capture from. Confirm yours with `-list-cameras` before trusting the default |
| `detection.nightStartMinutes` | `1140` (19:00) | Start of the night profile, minutes since midnight |
| `detection.nightEndMinutes` | `360` (06:00) | End of the night profile, minutes since midnight |
| `detection.motionPersist` | `"5s"` | How long movement must continue before a clip is started |
| `detection.prebufferLead` | `"3s"` | How much footage from *before* the trigger is prepended to a clip |
| `detection.clipDuration` | `"15s"` | Length of an automatic clip. Manual clips ignore this and run until `/stop` |

### Sensitivity profiles

`detection.day` and `detection.night` hold the same four fields, applied
depending on the time of day.

| Field | Day default | Night default | Meaning |
|---|---|---|---|
| `minimumContourArea` | `8000` | `9000` | Pixel area a moving region must exceed to count. Raise it to ignore smaller movement |
| `thresholdValue` | `30` | `30` | Per-pixel brightness change treated as movement. Raise it to ignore gradual light changes |
| `sampleInterval` | `"1s"` | `"1s"` | Gap between frames examined for motion while idle |
| `alertCooldown` | `"5m0s"` | `"5m0s"` | Minimum gap between snapshot alerts, so one long event does not produce a stream of messages |

Night is slightly less sensitive by default: a dim, noisy image produces more
spurious pixel change, so a larger region is required before it counts.

### Why the night window is in minutes

The boundaries are minutes since midnight rather than whole hours, so a schedule
that turns over at `21:30` or `06:30` is expressible instead of being rounded to
something you did not mean. Multiply hours by 60 and add the minutes: `1140` is
`19 × 60`, `360` is `6 × 60`. A window that wraps past midnight is handled
correctly, and setting start equal to end disables the night profile entirely.

### Storage

| Field | Default | Meaning |
|---|---|---|
| `storage.mediaDirectory` | per-user media directory | Where snapshots and clips are written |
| `storage.retentionDays` | `30` | Recordings older than this are deleted at startup. `0` keeps everything, which will eventually fill the disk |

Clips are written at the frame rate measured from the camera on startup, not at
a configured one. A guessed rate is what makes recordings play back too fast or
too slow, so the rate is not exposed as a setting.

## Troubleshooting

**"no usable camera found", or a second launch fails**

Only one process can hold a camera. If the detector is already running — or a
previous run has not fully exited, or a video-conferencing app has the webcam —
a second instance cannot open the device and fails with a backend error that
does not say so. Close the other user of the camera first.

**A camera is listed but the detector sees nothing, or index 0 does nothing**

Index 0 is frequently a virtual camera (Iriun, OBS) that opens successfully and
then delivers no frames. Run `go run ./cmd/kaa-rada-cli -list-cameras`, which
requires each device to hand over a real frame before reporting it, and pass
the index it gives you with `-camera N`.

**The bot token is rejected even though it looks correct**

A `.env` file written on Windows ends each line with a carriage return, which is
invisible in an editor but becomes part of the value and makes the request URL
invalid. Credentials are trimmed on the way in, so this case is handled — but if
you are pasting a token anywhere else, watch for trailing whitespace.

**A wall of OpenCV warnings on Linux**

Lines like `can't open camera by index` from `cap_v4l.cpp` are harmless. They
are OpenCV reporting each device index that is not a capture device while
enumerating. Linux asks for the V4L2 backend directly, which already avoids a
much longer page of GStreamer pipeline warnings.

**A clip reports dropped frames**

A finished clip may note that frames were dropped because the encoder could not
keep up. Frames are dropped deliberately rather than allowed to accumulate, so
that a slow disk cannot stall capture or grow memory without limit. Persistent
dropping means the machine cannot encode at the camera's rate.

## Building from source

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for toolchain setup, OpenCV
installation per platform, and the build and packaging targets.

For how the code is organised and why, see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Why only Telegram

Telegram is the only supported channel, and WhatsApp is deferred on purpose.

WhatsApp's official Cloud API cannot do what this app needs without
infrastructure a local desktop app does not have. Sending an unprompted message
— which is exactly what a motion alert is — requires a Meta Business account
and message templates approved in advance, so an alert cannot simply be composed
and sent. Receiving commands requires a publicly reachable HTTPS webhook, which
a program running on someone's home PC does not have and should not need.

The unofficial route works around both by driving WhatsApp's own protocol, but
it carries a terms-of-service and account-ban risk that belongs in front of a
user as an explicit choice, not shipped quietly in a release.

The `notify.Notifier` interface exists so another provider can be added without
touching the detector. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#adding-a-notifier) for what that
involves.

## License

MIT. See [LICENSE](LICENSE).
