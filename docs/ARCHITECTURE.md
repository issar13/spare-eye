# Architecture

For a developer adding a feature: what each package owns, how they communicate,
and why several of the choices are what they are. Many are direct fixes for
specific bugs, and undoing them reintroduces the bug.

For build instructions see [DEVELOPMENT.md](DEVELOPMENT.md). For user-facing
behaviour see the [README](../README.md).

## Package map

```
apps/desktop/           Wails entry point and webview UI
apps/cli/               headless CLI application
internal/app/           service layer bound to the UI
internal/autostart/     launch-at-login registration
internal/config/        settings persistence
internal/desktopnotify/ notifications on this computer
internal/detector/      camera, motion analysis, recording
internal/media/         listing and retention
internal/notify/        messaging providers
```

Dependencies run in one direction only:

```
apps/cli ──┐
           ├─> internal/detector ─> internal/config
internal/app ─┤  internal/media   ─> (stdlib only)
           ├─> internal/notify   ─> (stdlib only)
           └─> internal/desktopnotify ─> (D-Bus / Windows toast libraries)
```

`detector` imports `config` and nothing else of ours. `media` and `notify`
import nothing of ours at all. Neither the detector nor either notifier knows the
other exists; the process that wires them together is the only place they meet.
That is what makes the notifier swappable and the detector testable without a
network.

The CLI is not shipped in the release bundles, but it is a real part of the
codebase: CI builds and runs it on both platforms as a smoke test, since a
console binary that starts and lists cameras proves the OpenCV libraries link
and load without a window in the way.

## config

Owns the persisted settings for one installation and nothing else. Every tuning
value lives here rather than as a constant in code, because a value compiled
into the binary cannot be changed without a rebuild and drifts from whatever the
documentation claims about it.

Four things about the shape of this package are load-bearing:

**`Duration` is a named `time.Duration` with custom JSON marshalling.** It
serialises as `"15s"` rather than `15000000000`, and `config.json` is a file
people edit by hand.

**`Load` starts from `Default()` and unmarshals over it.** A field absent from
the file keeps its default rather than becoming zero, so an older `config.json`
stays loadable after new settings are added, with no migration step and no
version field. `Load` also clamps the live-view bounds, so a hand-edited file
heals itself instead of making the settings screen refuse every save.

**`Save` writes a temp file and renames.** A rename is atomic on both target
platforms, so an interrupted save cannot leave the user with a truncated config
and, in particular, no credentials. The file is `0600` because it holds a bot
token.

**`CameraChosen` is decided by key presence, not file existence.** No camera is
auto-adopted: a laptop whose index 0 is a virtual webcam would otherwise be
watched by a device that never sends a picture. But an installation predating
the question already has an index somebody is running with, so
`mentionsCameraChoice` unmarshals into a `*bool` and treats a file with no
`cameraChosen` key as a choice already made. Testing for the file instead would
re-prompt a fresh install that saved a bot token before picking a camera.
`CameraURL` and `CameraIndex` stay separate fields, since one overloaded string
leaves every reader guessing whether `"0"` names a device or a file.

Paths come from `os.UserConfigDir`, never the executable's directory. An
installed binary under `C:\Program Files` cannot write beside itself without
administrator rights, and a shared location would mean two users on one machine
sharing one person's credentials and footage.

The night window is stored as `NightStartMinutes` / `NightEndMinutes`, so a
schedule turning over at `21:30` stays expressible where an hour-valued setting
would round it away.

## detector

Owns the camera. Nothing else in the program opens a capture device, and only
one detector may exist per process because only one process can hold a camera at
all.

Its public surface is small on purpose:

| Method | Purpose |
|---|---|
| `Run(ctx)` | Opens the camera and blocks until ctx is cancelled |
| `Events()` | Receive-only channel of `Event`, closed when `Run` returns |
| `Status()` | Current state, safe from any goroutine |
| `PreviewJPEG()` | Most recent frame as JPEG, or nil before the first frame |
| `SetPreview(fps, quality)` | Retune the live view on a running detector |
| `StartManualRecording()` / `StopManualRecording()` | Request a manual clip |

Everything the detector tells the outside world goes out as an `Event` with a
typed `Kind`. Subscribers filter on the enum rather than parsing message text,
which lets the CLI print human lines and the UI render a timeline from the same
stream without either knowing about the other. The package has no logger for the
same reason: whatever wires it up decides what a reconnect attempt or a dropped
frame is worth saying.

`Status()` and `PreviewJPEG()` are polled rather than pushed, and both read an
`atomic.Pointer`. A UI redrawing at its own cadence should not influence the
capture loop, and a poll that always returns the latest value cannot build a
backlog.

### Files

| File | Responsibility |
|---|---|
| `detector.go` | Capture loop, motion analysis, event publication, recorder lifecycle |
| `recorder.go` | Writing one clip to disk on its own goroutine |
| `prebuffer.go` | Ring of recent frames so a clip can start before the trigger |
| `source.go` | The device-or-address decision, made once |
| `cameras.go` | Probing one source, backend selection |
| `camerascan.go` | Enumeration and stream testing in a child process |
| `lock.go`, `lock_unix.go`, `lock_windows.go` | The single-instance claim on the camera |
| `camerathread_*.go` | Pinning capture work to one OS thread |

### Source

`Source` is either a device index or the address of a network stream, and it
exists so the int-or-string decision gocv's API forces is made once.
`SourceFor` reads the settings: a non-empty `CameraURL` wins over
`CameraIndex`, because somebody who has typed an address is not also expressing
an opinion about which USB port to open.

`openCapture` asks for V4L2 explicitly for a local device on Linux. Left to
itself OpenCV tries GStreamer first and emits a paragraph of pipeline warnings
for every index that is not a capture device. A URL is the opposite case: V4L2
speaks to kernel video devices and cannot open one, so the default backend,
which resolves to FFMPEG, is the only one that works. Windows takes the default
either way, which is DirectShow for a device.

### The capture loop

One goroutine, in `processFrames`. It reads a frame, and that read blocks until
the camera has one, pacing the loop at the device's native rate with no sleep
and no clock arithmetic.

Each iteration does, in order: feed the active recorder, add to the prebuffer,
service manual start/stop requests, update the preview if its interval has
elapsed, and finally, if the profile's sample interval has elapsed, run motion
analysis.

**Analysis is throttled independently of capture.** If one interval governed
both, raising the sensitivity sampling to once a second would also thin the
recording to one frame per second. Every captured frame goes to the recorder and
the prebuffer; only the expensive grayscale/blur/diff work is rate-limited.

Analysis is the standard frame-differencing pipeline: convert to grayscale,
Gaussian blur (21×21) to suppress sensor noise, absolute difference against the
previous analysed frame, threshold at the profile's `ThresholdValue`, dilate with
a 3×3 kernel to close gaps, then find contours and ask whether any exceeds
`MinimumContourArea`.

Two independent timers govern what happens when motion is present:
`AlertCooldown` gates snapshot alerts, and `MotionPersist` gates starting a clip.
After a clip is triggered, `motionSince` is reset to now rather than cleared, so
one continuous event cannot immediately queue another clip the moment the first
finishes.

### A stream that stops

A failed read is fatal for a local device: nothing gets better by waiting for
hardware that has been unplugged or taken by another program. A network stream
is the other way round, so `Source.Network()` decides which rule applies.

Ten consecutive empty reads, each costing the loop its own half second, is taken
as gone rather than stuttering. `reconnect` then closes the handle and reopens
it, waiting two seconds and doubling to a ceiling of a minute. Only cancellation
makes it give up: a quiet camera usually means a router or a Wi-Fi link, and a
house left unwatched until somebody notices is the worse outcome. `close` nils
the handle as it releases it, because the run's deferred close must not hand the
same capture to OpenCV twice.

The first open is still fatal, for a stream as much as a device. Reconnecting is
for a watch that was working and stopped; an address that never answered is
something to say now, while the user is still looking at the button they pressed.

### Frame rate

`measureCaptureRate` reads 30 frames and times them, clamping the result to
between 1 and 60 FPS. It does not ask OpenCV for `VideoCaptureFPS`, because
webcams routinely report 0 or a nominal figure unrelated to what they deliver.

The measured rate sets the clip's declared frame rate, the prebuffer's frame
count, and the recorder channel's capacity. No hardcoded or configured rate is
used anywhere: if it disagrees with what the camera delivers, every clip plays
back too fast or too slow, and that is invisible until someone watches one.

### Prebuffer

A clip should include the moment before the trigger, so the prebuffer keeps
recent frames and `drain()` hands them to a new recorder, transferring ownership
of every `Mat` it returns.

Its bound is `PrebufferLead × measured FPS`, a wall-clock lead converted to a
frame count. A flat count fails in both directions: at VGA, 300 frames is
roughly 276 MB held permanently, and the span of history it represents swings
with the camera's rate, so a "15 second" clip can arrive with half a minute of
unrequested footage on the front.

### Recorder

One goroutine per clip, created by `startRecorder` and living exactly as long as
that clip. Encoding and disk I/O never run on the capture goroutine.

Frames arrive through a bounded channel sized at two seconds of footage
(`frameRate × 2`, floor 8). Bounded is the point: `submit` does a non-blocking
send and, when the channel is full, closes the clone and increments
`droppedFrames`. An unbounded queue never rejects a frame, so it grows instead
of shedding load and the counter stays at zero while memory climbs. The drop
count rides on `EventClipFinished`, which is how a user learns their machine
cannot encode at the camera's rate.

An automatic clip runs against a timer of `ClipDuration`; a manual one runs
until `stopRecording`. Both flush what is already queued before closing the
writer, so a clip does not lose its final moments, and
`finishActiveRecording` waits for the file to close on shutdown, so exiting
never leaves a truncated video.

### Mat ownership

Every `gocv.Mat` is a handle to memory the Go garbage collector does not manage.
Leaking one leaks native memory; closing one twice, or reading one after close,
is a crash rather than a panic. Hence the rule: **exactly one owner, responsible
for closing**, with every transfer explicit.

| Mat | Owner | Closed by |
|---|---|---|
| Capture `frame`, and the analysis scratch Mats | `Run` / `processFrames` | deferred `Close` in the function that created them |
| `prebuffer` entries | the prebuffer, which clones on `add` | evicted on overflow, or by the caller of `drain` |
| `clipRequest.lead` | the recorder, transferred by `drain` | `run`, after writing each one |
| Frames in `recorder.frames` | the recorder, cloned by `submit` | `writeFrame`, or `releasePending` on a failed or finished clip |
| Preview JPEG | copied out of the native buffer into a Go slice immediately | n/a, `IMEncode` result is closed at once |

`submit` clones because the capture loop reuses one `Mat` for every read; handing
the recorder the live frame would have it encode whatever the next read
overwrote it with. `releasePending` exists so a clip whose writer failed to open
still drains and closes everything queued behind it.

### Probing a source

`probeSource` requires a source to open **and** deliver a real frame. Requiring
a frame is not defensive padding: virtual camera drivers such as Iriun and OBS
register a device node that opens successfully and then yields nothing, and such
a device commonly holds index 0. A probe that trusted a successful open would
report a camera that fails at first read. `probeStream` applies the same proof
to an address and reports the result as a `Camera`, since the picture it
describes is the same one.

Indices 0 through 7 are the sweep (`cameraProbeLimit`). `FirstUsableCamera`
stops at the first success rather than probing all eight, because each failed
probe on Linux prints OpenCV warnings; the CLI uses it to pick a default.

### Enumeration in a child process

Opening a capture device can end the process from inside a vendor driver, below
any Go stack frame. There is no panic, nothing to recover, and no error to
return: on Windows this made the app appear and vanish during startup. The only
way to survive it is to do the opening somewhere else.

So `AvailableCamerasIsolated` re-executes the running binary with
`--scan-cameras=<startIndex>`. `main` answers that flag before anything else,
including opening the log, because the child reports on stdout and appending to
the log file its parent already holds open would interleave two processes'
records. `scanHelperPath` resolves symlinks, since a Linux user may launch
through one in `~/.local/bin` while the binary lives in an unpacked tarball.

The child writes one JSON object per line and **announces an index before it
opens it**:

```
{"probing":0}
{"camera":{"index":0,"width":640,"height":480,"label":"Camera 0 (640x480)"}}
{"probing":1}
{"done":true}
```

That ordering is what makes a truncated stream readable. The last `probing` line
names the device that killed the child, so `gatherCameras` restarts the helper
from the next index and merges the results, and one hostile driver cannot hide
every camera behind it. Each run starts strictly after the previous run's last
announcement, so the sweep always moves forward and the restart count cannot
exceed the number of indices. Only `{"done":true}` proves the stream was not cut
short.

A driver can also hang rather than crash, so each invocation has a 20-second
deadline, treated identically: a hung helper and a dead one leave the parent the
same question. For the same reason the child's exit status is ignored.

Failing to start the helper at all, because security software refused the exec
or the executable cannot be resolved, is a different case. On the **first** sweep
that falls back to probing in this process, since an app that reports no cameras
because it could not spawn a child is worse off than one that risks the open. A
later sweep does not fall back: by then a device is known to have killed a child,
and repeating that open in the parent would repeat the crash where it costs the
window.

A network address is tested through the same machinery, with
`--test-camera=<address>`. One source is the degenerate case of a sweep, so it
reuses the spawn, the deadline and the reader rather than adding a second
protocol. `errStreamSentNoPicture` separates a helper that ran and found nothing
from one that never ran, since only the second is worth retrying in the parent.

### Single-instance lock

`AcquireInstanceLock` takes an exclusive lock on `camera.lock` in the
configuration directory and returns `ErrAlreadyRunning` when another instance
holds it. Without it a second launch fails deep inside the OpenCV backend with a
message naming neither the camera nor the other process. The lock file lives
beside the config rather than the media, because a user may point the media
directory at a network share where locking is unreliable. The holder's PID is
written into it so a support question can be answered by reading one file.

Startup takes the claim before the window opens but does not exit on failure:
the error is handed to the service, and a window that explains why it cannot arm
beats a process that vanishes with a message nobody sees.

### Thread pinning

`pinCameraThread` keeps every call into the capture backend on one OS thread for
the length of a run or a probe. On Windows that backend is DirectShow, which is
COM: an object belongs to the apartment of the thread that created it, and
releasing it from another is undefined. Go reschedules goroutines across threads,
so without pinning the open and the close can land on different ones and the
process dies inside the driver, again with no panic to recover.

## media

Listing and retention over the media directory. Pure filesystem work; it never
touches the camera or the network, which is why a test can exercise it against a
temp directory.

`ListVideos` and `ListSnapshots` return `Recording` values sorted **newest
first**. The ordering is part of the contract, not an artefact of directory
order: the Telegram listing is numbered and the user replies with a number, so
"1" must reliably be the most recent clip. `Manual` is derived from the
`manual_` filename prefix, which is why the detector's clip naming is a contract
rather than cosmetic.

`Prune` deletes files older than `retentionDays`, treating zero or less as "keep
everything", and reports how many it removed. It scans one directory, so both
callers loop over the clip and snapshot directories in turn; pruning only clips
left snapshots accumulating forever, which on a busy camera is the larger share
of the files.

## desktopnotify

One function, `Show(Message)`, that asks the operating system to display a
notification: D-Bus `org.freedesktop.Notifications` on Linux, a toast on
Windows, and an "unsupported" error elsewhere, split into build-tagged files
like `autostart`. It exists so Telegram stays optional: it needs no account and
sends nothing off the machine. `internal/app` calls it when a snapshot is saved
and the `notifications.desktop` setting is on. It runs off the event reader's
goroutine and a failure is only logged, because the snapshot is already saved
and a missing notification daemon must never stop the camera. The detector does
not know it exists.

## notify

A messaging provider, plus the interface that keeps the detector ignorant of
which one is in use.

```go
type Notifier interface {
    Send(ctx context.Context, message string, attachment *Attachment) error
    Commands(ctx context.Context) (<-chan Incoming, error)
    Name() string
    Close() error
}
```

The interface is deliberately narrow, and its shape encodes a few decisions.

**`Send` takes an optional attachment rather than separate
`SendPhoto`/`SendVideo` methods.** A caller says what it wants delivered;
choosing an upload endpoint is the provider's problem. `AttachmentKind` is a
two-value enum rather than a MIME type because that is all the abstraction needs
to convey.

**`Commands` returns a channel, not a callback.** Providers differ in how they
learn about inbound messages (Telegram long-polls, another might hold a
websocket or receive webhook deliveries) and a channel hides all of it behind
one shape. The interface documents that messages from anyone other than the
authenticated user are discarded *before* reaching the channel, which makes
sender authentication a provider responsibility rather than something every
consumer has to remember.

**`Incoming` is a parsed command or a numeric selection, never message text.**
Command parsing belongs to the provider, so consumers never string-match.

### Telegram

`Commands` long-polls `getUpdates` with a 25-second timeout, backing off 5
seconds on a network error rather than treating a blip as fatal. Two details in
the poller are there for specific reasons:

- **Sender check.** An update whose `message.chat.id` does not equal the
  configured chat ID is dropped. A leaked bot token must not be enough to drive
  someone's camera.
- **Offset advance across discarded updates.** `lastUpdateID` advances past
  every update received, including rejected ones. Advancing only past accepted
  updates would let one message from a stranger wedge the offset and stall the
  poller permanently.

`flexibleID` accepts a chat ID whether Telegram encodes it as a JSON number or a
string, so the comparison cannot fail on representation.

Credentials are trimmed in `NewTelegram`. A `.env` written on Windows leaves a
carriage return on every value, invisible in an editor, which lands in the middle
of the request URL and makes every call fail with a confusing error.

`Verify` calls `getMe` and then sends a confirmation message, so the UI's *Test
connection* proves both that the token is valid and that the chat is reachable.
A token authenticating against a chat ID that does not exist would otherwise
look fine until the first real alert.

`parseCommand` trims whitespace *before* stripping the leading slash. The reverse
order leaves the slash intact on `" /record"` and nothing matches, a real bug
worth not reintroducing.

<a id="adding-a-notifier"></a>

### Adding a notifier

Implement the four methods and hand the value to whatever wires the process
together. Nothing in `detector` changes.

Concretely, a new provider must:

1. Deliver text and both attachment kinds through `Send`.
2. Authenticate inbound messages against the configured account and drop
   everything else *before* it reaches the `Incoming` channel.
3. Parse its own command text into `Command` / `Selection`, accepting the same
   vocabulary users already know.
4. Survive transient network failure inside `Commands` rather than returning.
5. Add its credentials to `config.Config` and its own `Verify`-equivalent so
   setup can be tested before the first alert.

**WhatsApp specifically** is deferred, and the interface is not the obstacle.
The official Cloud API cannot serve this use case from a desktop app: proactive
messages, which is what every motion alert is, require a Meta Business account
and pre-approved templates, so an alert cannot be composed freely at send time;
and inbound commands arrive at a public HTTPS webhook, which a program on a home
PC does not have and which anyone who finds it can reach, so point 2 has no
clean answer either.

The unofficial `whatsmeow` route sidesteps both by speaking WhatsApp's own
protocol as a linked device. It would satisfy the interface, but it carries a
terms-of-service and account-ban risk that has to be an explicit user choice
rather than a shipped default. That is the decision, not an oversight.

## apps/cli and internal/app

Both are wiring: construct a `Detector`, construct a `Notifier`, connect the
two. Neither owns behaviour the other should have to reimplement, and anything
that starts to look like shared logic between them belongs in `internal/app`.

The CLI's `deliver` sends each attachment on its own goroutine. An upload takes
seconds and the event channel is bounded at 64; blocking the event reader on a
network call would drop events, which `publish` is designed to do only under
genuine backpressure.

`applyEnvironmentCredentials` fills a token or chat ID from the environment only
when `config.json` supplies none. Saved settings win, so a UI-configured
installation is not silently overridden by a stale shell export.

On the desktop side, `resolveCamera` runs at startup and changes nothing. It
sweeps for devices and records what it found, then either confirms the chosen
camera is among them or reports that it did not answer: a camera that has
stopped working becomes a problem on screen, never a silent move to whatever
else is plugged in. A configured stream skips the sweep, which cannot find one
and would only duplicate the open that arming is about to do.

`RescanCameras` returns the cached list while armed, because the detector owns a
device and a fresh probe would fail on that index and risk disturbing capture.

An address is validated at the boundary and never opened in this process.
`validateCameraURL` requires a host and one of `rtsp`, `rtsps`, `http` or
`https`, turning away a local path or a scheme FFMPEG does not speak here rather
than where it would hang a driver. `TestCameraURL` then reports the picture size,
and `SelectCameraURL` opens the address before persisting it, since an address
that has never delivered a frame is not something to save as the source a house
will be watched by. Both go through `TestStreamIsolated`.

**The address stays out of the log.** It routinely carries `user:pass@` in front
of the host and the log is what gets pasted into a bug report, so the log says "a
network stream" and a failure is recorded without the backend's message, which
would quote the address back.

Settings that are only read when the camera opens (the source, and the prebuffer
size derived from the measured frame rate) are applied by `restartDetector`,
which stops and rearms only when a watch is running. Live-view speed and quality
are the exception: they are atomics on the detector, so `SetPreview` retunes the
picture without touching the open camera or a clip in progress.

## Concurrency summary

| Goroutine | Count | Lifetime |
|---|---|---|
| Capture and analysis (`processFrames`) | exactly 1 | `Run` |
| Recorder | 1 per clip | one clip |
| `awaitRecording` | 1 per clip | until the clip's file is closed |
| Telegram command poller | 1 | until ctx is cancelled |
| Attachment upload | 1 per send | one HTTP request |
| Camera scan helper | 1 process per sweep run | one helper invocation |

Shared state and how it is protected:

| State | Mechanism |
|---|---|
| `activeRecorder`, `recordingManual` | `sync.Mutex`, held only around the pointer read or write, never across a `Read` or an encode |
| `Status`, preview JPEG | `atomic.Pointer` to an immutable value |
| Preview FPS and quality | `atomic.Int64`, written by the UI, read on the capture path |
| `droppedFrames` | `atomic.Int64` |
| Manual start/stop requests | capacity-1 channels; a redundant request is discarded, not queued |
| Events out | buffered channel, capacity 64, non-blocking send |
| Frames to recorder | bounded channel, non-blocking send, drop on full |

The through-line: **the capture goroutine must never block.** Every channel it
writes to is bounded with a non-blocking send, every lock it takes is held for a
pointer assignment, and every slow operation (encoding, uploading, JPEG
conversion for a UI nobody may be watching) is rate-limited or moved off it. A
camera read that stalls is a gap in the footage, and there is no way to get that
time back.

## Known gaps

Tracked in [ROADMAP.md](../ROADMAP.md), noted here where they touch the design:

- **Clips are `mp4v`.** WebKit cannot decode it, so in-app playback does not
  work and the working path is opening the file externally. Fixing it properly
  means moving the recorder to H.264.
- **No config hot-reload for the capture path.** Everything except the live-view
  settings is fixed when the camera opens, so changing it restarts the detector.
- **The CLI prints with `fmt`.** Acceptable for a terminal program, but it means
  the two front ends report the same events in two unrelated formats; only the
  desktop app has a structured log.
