# Architecture

For a developer adding a feature. This describes what each package owns, how
they communicate, and why several of the choices are what they are — a number of
them are direct fixes for specific bugs in the previous implementation, and
undoing them reintroduces the bug.

For build instructions see [DEVELOPMENT.md](DEVELOPMENT.md). For user-facing
behaviour see the [README](../README.md).

## Package map

```
main.go            Wails desktop entry point          (in progress)
app.go             methods bound to the frontend      (in progress)
cmd/kaa-rada-cli/       headless CLI — no display needed
internal/app/      service layer bound to the UI      (in progress)
internal/config/   settings persistence
internal/detector/ camera, motion analysis, recording
internal/media/    listing and retention
internal/notify/   messaging providers
frontend/          webview UI, Wails-managed          (in progress)
```

Dependencies run in one direction only:

```
cmd/kaa-rada-cli ─┐
             ├─> detector ─> config
internal/app ┤   media    ─> (stdlib only)
             └─> notify   ─> (stdlib only)
```

`detector` imports `config` and nothing else of ours. `media` and `notify` import
nothing of ours at all. Neither the detector nor the notifier knows the other
exists — the process that wires them together (the CLI today, `internal/app`
under the UI) is the only place they meet. That is what makes the notifier
swappable and the detector testable without a network.

## config

Owns the persisted settings for one installation and nothing else.

Every tuning value lives here rather than as a constant in code. A value
compiled into the binary cannot be changed without a rebuild, and it drifts from
whatever the documentation claims about it, because nothing forces the two to
agree. A value the user can see and edit is one that can be documented
truthfully and tested against.

Three things about the shape of this package are load-bearing:

**`Duration` is a named `time.Duration` with custom JSON marshalling.** It
serialises as `"15s"` rather than `15000000000`. `config.json` is a file people
edit by hand, and nanosecond integers make that hostile.

**`Load` starts from `Default()` and unmarshals over it.** A field absent from
the file keeps its default rather than becoming zero. This means an older
`config.json` stays loadable after new settings are added, with no migration
step and no version field.

**`Save` writes a temp file and renames.** A rename is atomic on both target
platforms, so an interrupted save cannot leave the user with a truncated config
and, in particular, no credentials. The file is `0600` because it holds a bot
token.

Paths come from `os.UserConfigDir`, never from the executable's directory. An
installed binary under `C:\Program Files` cannot write beside itself without
administrator rights, and a shared location would mean two users on one machine
sharing one person's credentials and footage.

The night window is stored as `NightStartMinutes` / `NightEndMinutes`, minutes
since midnight, so that a schedule turning over at a time like `21:30` stays
expressible where an hour-valued setting would round it away.

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
| `SetPreview(fps, quality)` | Retune the live view on a detector that is already running |
| `StartManualRecording()` / `StopManualRecording()` | Request a manual clip |

Everything the detector wants to tell the outside world goes out as an `Event`
with a typed `Kind`. Subscribers filter on the enum rather than parsing message
text, which is what lets the CLI print human lines and a UI render a timeline
from the same stream without either knowing about the other.

`Status()` and `PreviewJPEG()` are polled rather than pushed, and both read an
`atomic.Pointer`. A UI redrawing at its own cadence should not be able to
influence the capture loop, and a poll that always returns the latest value
cannot build a backlog.

### Files

| File | Responsibility |
|---|---|
| `detector.go` | Capture loop, motion analysis, event publication, recorder lifecycle |
| `recorder.go` | Writing one clip to disk on its own goroutine |
| `prebuffer.go` | Ring of recent frames so a clip can start before the trigger |
| `cameras.go` | Device enumeration and backend selection |

### The capture loop

One goroutine, in `processFrames`. It reads a frame, and that read blocks until
the camera has one — which paces the loop at the device's native rate with no
sleep and no clock arithmetic.

Each iteration does, in order: feed the active recorder, add to the prebuffer,
service manual start/stop requests, and update the preview if the preview
interval has elapsed. Only then, if the profile's sample interval has elapsed,
does it run motion analysis.

**Analysis is throttled independently of capture, and that separation matters.**
If one interval governed both, then raising the sensitivity sampling to once a
second would also thin the recording to one frame per second. Every captured
frame goes to the recorder and the prebuffer; only the comparatively expensive
grayscale/blur/diff work is rate-limited.

Analysis itself is the standard frame-differencing pipeline: convert to
grayscale, Gaussian blur (21×21) to suppress sensor noise, absolute difference
against the previous analysed frame, threshold at the profile's
`ThresholdValue`, dilate with a 3×3 kernel to close gaps, then find contours and
ask whether any exceeds `MinimumContourArea`.

Two independent timers govern what happens when motion is present:
`AlertCooldown` gates snapshot alerts, and `MotionPersist` gates starting a clip.
After a clip is triggered, `motionSince` is reset to now rather than cleared, so
one continuous event cannot immediately queue another clip the moment the first
finishes.

### Frame rate

`measureCaptureRate` reads 30 frames and times them, clamping the result to
between 1 and 60 FPS. It does not ask OpenCV for `VideoCaptureFPS`, because
webcams routinely report 0 or a nominal figure unrelated to what they deliver.

The measured rate is then used for three things: the clip's declared frame rate,
the prebuffer's frame count, and the recorder channel's capacity. A hardcoded or
configured rate is not used anywhere: if it disagrees with what the camera
delivers, every clip plays back too fast or too slow, and the discrepancy is
invisible until someone watches one.

### Prebuffer

A clip should include the moment before the trigger, so the prebuffer keeps
recent frames and `drain()` hands them to a new recorder.

Its bound is `PrebufferLead × measured FPS` — a wall-clock lead converted to a
frame count — not a fixed frame count. A flat count fails in both directions: at
VGA, 300 frames is roughly 276 MB held permanently, and the span of history it
represents swings with the camera's rate, so a "15 second" clip can arrive with
half a minute of unrequested footage on the front. A time-based bound keeps the
memory cost proportional to the real frame rate and the prepended footage at the
length the user asked for.

`drain()` transfers ownership of every returned `Mat` to the caller.

### Recorder

One goroutine per clip, created by `startRecorder` and living exactly as long as
that clip. Encoding and disk I/O never run on the capture goroutine.

Frames arrive through a bounded channel sized at two seconds of footage
(`frameRate × 2`, floor 8). Bounded is the point: `submit` does a non-blocking
send and, when the channel is full, closes the clone and increments
`droppedFrames`. The bound is what makes the drop path reachable at all: an
unbounded queue never rejects a frame, so it silently grows instead of shedding
load and the counter stays at zero while memory climbs. The drop count is
reported on the `EventClipFinished` event, which is how a user learns their
machine cannot encode at the camera's rate.

An automatic clip runs against a timer of `ClipDuration`. A manual clip has no
duration and runs until `stopRecording`. Both flush whatever is already queued
before closing the writer, so a clip does not lose its final moments.

`finishActiveRecording` on shutdown stops the clip and waits for the file to be
closed, so Ctrl+C never leaves a truncated video.

### Mat ownership

Every `gocv.Mat` is a handle to memory the Go garbage collector does not manage.
Leaking one leaks native memory; closing one twice, or reading one after close,
is a crash rather than a panic. So the rule is: **exactly one owner, responsible
for closing.** Ownership transfers are explicit and documented at each boundary.

| Mat | Owner | Closed by |
|---|---|---|
| Capture `frame`, and the analysis scratch Mats | `Run` / `processFrames` | deferred `Close` in the function that created them |
| `prebuffer` entries | the prebuffer, which clones on `add` | evicted on overflow, or by the caller of `drain` |
| `clipRequest.lead` | the recorder, transferred by `drain` | `run`, after writing each one |
| Frames in `recorder.frames` | the recorder, cloned by `submit` | `writeFrame`, or `releasePending` on a failed or finished clip |
| Preview JPEG | copied out of the native buffer into a Go slice immediately | n/a — `IMEncode` result is closed at once |

`submit` clones because the capture loop reuses one `Mat` for every read; handing
the recorder the live frame would have it encode whatever the next read
overwrote it with. `releasePending` exists so that a clip whose writer failed to
open still drains and closes everything queued behind it.

### Camera enumeration

`AvailableCameras` probes indices 0 through 7 and reports only devices that open
**and** deliver a real frame. Requiring a frame is not defensive padding: virtual
camera drivers such as Iriun and OBS register a device node that opens
successfully and then yields nothing, and such a device commonly holds index 0. A probe that trusted a successful open would report a camera that
fails at first read, which is a much worse error to hand a user.

`FirstUsableCamera` stops at the first success rather than probing all eight,
because each failed probe on Linux prints OpenCV warnings.

`openCapture` requests V4L2 explicitly on Linux. Left to itself OpenCV tries
GStreamer first and emits a paragraph of pipeline warnings for every index that
is not a capture device. Windows takes the default, which is DirectShow, and is
untested.

## media

Listing and retention over the media directory. Pure filesystem work — it never
touches the camera or the network, which is why it can be exercised in a test
with a temp directory.

`ListVideos` and `ListSnapshots` return `Recording` values sorted **newest
first**. The ordering is part of the contract, not an incidental result of
directory order: the Telegram listing is numbered and the user replies with a
number, so "1" must reliably be the most recent clip. Directory order is
whatever the filesystem returns and is not sorted by time on any platform.

`Manual` is derived from the `manual_` filename prefix, which is why the
detector's clip naming is a contract rather than cosmetic.

`Prune` deletes files older than `retentionDays`, treating zero or less as
"keep everything". It reports how many it removed so a caller can log it. Note
that the CLI currently calls it on the video directory only; snapshots
accumulate.

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

**`Send` takes an optional attachment rather than having separate
`SendPhoto`/`SendVideo` methods.** A caller says what it wants delivered;
choosing an upload endpoint is the provider's problem. `AttachmentKind` is a
two-value enum rather than a MIME type because that is all the abstraction
needs to convey.

**`Commands` returns a channel, not a callback.** Providers differ in how they
learn about inbound messages — Telegram long-polls, another might hold a
websocket, a third might receive webhook deliveries — and a channel hides all of
it behind the same shape. The interface documents that messages from anyone other
than the authenticated user are discarded *before* reaching the channel, which
makes sender authentication a provider responsibility rather than something
every consumer has to remember.

**`Incoming` is a parsed command or a numeric selection, never message text.**
Command parsing belongs to the provider, so consumers never string-match.

### Telegram

`Commands` long-polls `getUpdates` with a 25-second timeout, backing off 5
seconds on a network error rather than treating a blip as fatal.

Two details in the poller are there for specific reasons:

- **Sender check.** An update whose `message.chat.id` does not equal the
  configured chat ID is dropped. A leaked bot token must not be enough to drive
  someone's camera.
- **Offset advance across discarded updates.** `lastUpdateID` advances past
  every update received, including rejected ones. Advancing only past accepted
  updates would let a single message from a stranger wedge the offset and stall
  the poller permanently.

`flexibleID` accepts a chat ID whether Telegram encodes it as a JSON number or a
string, so the comparison against the configured value cannot fail on
representation.

Credentials are trimmed in `NewTelegram`. A `.env` written on Windows leaves a
carriage return on every value, invisible in an editor, which lands in the middle
of the request URL and makes every call fail with a confusing error.

`Verify` calls `getMe` and then sends a confirmation message, so the UI's *Test
connection* proves both that the token is valid and that the chat is reachable.
A token that authenticates against a chat ID that does not exist would otherwise
look fine until the first real alert.

`parseCommand` trims whitespace *before* stripping the leading slash. The reverse
order leaves the slash intact on `" /record"` and nothing matches — a real bug
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
messages — which is what every motion alert is — require a Meta Business account
and pre-approved message templates, so an alert cannot be composed freely at
send time; and inbound commands are delivered to a public HTTPS webhook, which a
program on a home PC does not have. Point 2 above has no clean answer under that
model either, since the webhook is reachable by anyone who finds it.

The unofficial `whatsmeow` route sidesteps both by speaking WhatsApp's own
protocol as a linked device. It would satisfy the interface, but it carries a
terms-of-service and account-ban risk that has to be an explicit user choice
rather than a shipped default. That is the decision, not an oversight.

## cmd/kaa-rada-cli and internal/app

Both are wiring. They construct a `Detector`, construct a `Notifier`, and connect
the two. Neither owns any behaviour that the other should have to reimplement,
and anything that starts to look like shared logic between them belongs in
`internal/app`.

The CLI's `deliver` sends each attachment on its own goroutine. An upload takes
seconds and the event channel is bounded at 64; blocking the event reader on a
network call would let events be dropped, which is the one thing `publish` is
designed to do only under genuine backpressure.

`applyEnvironmentCredentials` fills a token or chat ID from the environment only
when `config.json` supplies none. Saved settings win, so a UI-configured
installation is not silently overridden by a stale shell export.

## Concurrency summary

| Goroutine | Count | Lifetime |
|---|---|---|
| Capture and analysis (`processFrames`) | exactly 1 | `Run` |
| Recorder | 1 per clip | one clip |
| `awaitRecording` | 1 per clip | until the clip's file is closed |
| Telegram command poller | 1 | until ctx is cancelled |
| Attachment upload | 1 per send | one HTTP request |

Shared state and how it is protected:

| State | Mechanism |
|---|---|
| `activeRecorder`, `recordingManual` | `sync.Mutex`, held only around the pointer read or write — never across a `Read` or an encode |
| `Status`, preview JPEG | `atomic.Pointer` to an immutable value |
| `droppedFrames` | `atomic.Int64` |
| Manual start/stop requests | capacity-1 channels; a redundant request is discarded, not queued |
| Events out | buffered channel, capacity 64, non-blocking send |
| Frames to recorder | bounded channel, non-blocking send, drop on full |

The through-line: **the capture goroutine must never block.** Every channel it
writes to has a bounded capacity and a non-blocking send, every lock it takes is
held for a pointer assignment, and every slow operation — encoding, uploading,
JPEG conversion for a UI nobody may be watching — is either rate-limited or moved
off it. A camera read that stalls is a gap in the footage, and there is no way to
get that time back.

## Known gaps

Tracked in [ROADMAP.md](../ROADMAP.md), noted here where they touch the design:

- **No single-instance guard.** Only one process can hold the camera; a second
  launch currently fails with an opaque backend error. Wants a lock file in the
  config directory.
- **No config hot-reload.** The camera, prebuffer size, and clip frame rate are
  all fixed at startup from the measured capture rate, so changing settings
  requires a clean detector restart.
- **`fmt.Println` rather than `log/slog`.** A windowed application has no
  terminal to print to.
- **Windows is untested.** The DirectShow capture path and `mp4v` clip writing
  have not been exercised there.
