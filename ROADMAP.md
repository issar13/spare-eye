# Roadmap — from local script to distributable app

Working document. The goal is a Telegram-alerting motion detector that a
non-technical person can download, install, and configure on Windows or Linux
without touching a config file.

## Where things stand

**Done and verified**

- Detection engine in Go, replacing the Python original
- Clips contain real frames, written at the camera's *measured* frame rate
  (verified: 66 frames / 162 KB from a 5-second manual clip)
- Camera enumeration that requires a real frame, not just a successful open
- Telegram delivery with sender authentication
- Settings persisted per-user instead of compiled in as constants
- 16 unit tests plus 3 camera integration tests; `go vet` and `gofmt` clean

**Not yet verified**

- The motion trigger end-to-end. Needs a human in front of the camera.

**Not started**

- Desktop UI, Windows build, CI, installers, user-facing docs

Honest assessment: the engine is the finished part. The UI and the Windows
toolchain are each comparable in size to what has been built so far, and the
Windows gocv build is the single biggest unknown in the project.

## Phase 0 — arrange the project

Restructure before the frontend lands, while the tree is still small.

### Target layout

Wails v2 compiles the package in the project root, so the desktop entry point
must be `main.go` at the top level rather than under `cmd/`. The CLI keeps its
`cmd/` location.

```
motion/
├── main.go                      Wails application entry point
├── app.go                       methods bound to the frontend
├── cmd/
│   └── motiond/                 headless CLI (no display required)
├── internal/
│   ├── app/                     service layer bound to the UI
│   ├── config/                  settings persistence
│   ├── detector/                camera, motion analysis, recording
│   ├── media/                   listing and retention
│   └── notify/                  Telegram, and later WhatsApp
├── frontend/                    webview UI (Wails-managed)
├── build/
│   ├── appicon.png
│   ├── linux/                   .desktop entry, AppImage metadata
│   └── windows/                 icon, manifest, NSIS installer config
├── docs/
│   ├── ARCHITECTURE.md          how the pieces fit
│   ├── DEVELOPMENT.md           building from source per platform
│   └── SETUP-TELEGRAM.md        creating a bot, finding a chat ID
├── .github/workflows/
│   ├── ci.yml                   test and vet on every push
│   └── release.yml              tagged builds, artifacts attached
├── Makefile                     one command per task, tags baked in
├── README.md                    what it is, how to install, how to configure
├── LICENSE
├── go.mod / go.sum
└── wails.json
```

### Moves

- `cmd/motion-detector/` becomes `cmd/motiond/`. The desktop app takes the
  product name; the CLI gets the daemon-style short name.
- Snapshot and clip retention already lives in `internal/media`; the UI will
  call it on start rather than only the CLI.

### Deletions

Three documents describe a path no longer being taken. All predate the Go
decision and actively mislead.

- `GO_VERSION.md` — build instructions for a version that never compiled, plus
  memory and CPU benchmark figures nobody measured
- `DESKTOP_INSTALLER_GUIDE.md` — PyInstaller and Inno Setup for the deleted
  Python implementation
- `GIT_SETUP.md` — one-off instructions for a repo that now exists

Their useful content moves into `docs/DEVELOPMENT.md`.

- `.env.example` stays. The UI is the normal way to configure credentials, but
  the headless CLI still reads the environment, and that needs documenting.

### Also in this phase

- `Makefile` targets: `test`, `test-camera`, `vet`, `run`, `build`,
  `build-desktop`, `package-linux`, `package-windows`. The
  `-tags webkit2gtk_4_1` requirement lives here, not in anyone's memory.
- Pin the Go toolchain in `go.mod` with a `toolchain` directive. Two Go
  installs are currently visible on this machine (1.24.4 and 1.25.12) and CI
  must not be a third answer.
- Tests for `internal/config` and `internal/media`, which have none. Duration
  round-tripping, defaults surviving a partial config file, atomic save, and
  newest-first ordering.
- First commit.

## Phase 1 — desktop UI (Wails)

Four surfaces, as chosen. Each is bound to `internal/app`, which owns a
`Detector` and mediates between it and the frontend.

**Setup**
- Bot token and chat ID entry, saved to `config.json` with `0600` permissions
- A *Test connection* button calling `Telegram.Verify` so a bad token is caught
  during setup rather than at the first motion event
- Camera picker populated from `AvailableCameras()`, showing resolution.
  Necessary, not cosmetic: index 0 on the development machine is a virtual
  device that opens successfully and then yields nothing.

**Arm / disarm**
- Start and stop, with live status from `Detector.Status()`
- Replaces the `DISABLED` sentinel file the Python build used
- Refuse to arm when no camera is selectable, with the reason shown

**Live preview**
- MJPEG stream over localhost from `Detector.PreviewJPEG()`, already producing
  decodable frames at 200 ms intervals
- Needed to aim the camera and to judge sensitivity changes

**Sensitivity and schedule**
- Day and night profiles: contour area, threshold, sample interval, cooldown
- Night window as two time pickers, backed by minutes-since-midnight so a
  boundary like 18:45 is expressible
- Clip duration, prebuffer lead, retention days
- Changing settings must restart the detector cleanly, since the camera and
  prebuffer are sized at startup

**Recording browser**
- Snapshots and clips, newest first, with size and timestamp
- Local playback, delete, reveal-in-folder
- Explicit *Send to Telegram* action

### Known work inside this phase

- Single-instance guard. Only one process can hold the camera; a second launch
  currently fails with an opaque backend error. A lock file in the config
  directory, plus focusing the existing window.
- Config hot-reload path: apply settings without losing an in-progress clip.
- Replace `fmt.Println` with `log/slog`, writing to a file the UI can display.
  A windowed application has no terminal to print to.
- Decide the fate of the Python build's `wake_screen`, which nudged the mouse
  on Windows to light the room at night. Either implement it per-platform
  behind a setting or drop it deliberately — not silently, as it currently is.

## Phase 2 — Windows build

The highest-risk phase. gocv needs cgo linked against OpenCV, and on Windows
that means a MinGW toolchain and either a prebuilt OpenCV or one built locally.

- Establish a repeatable Windows setup: OpenCV **4.6.0** to match the pinned
  gocv v0.31.0, MinGW-w64, and the environment variables gocv's cgo directives
  expect
- Confirm the camera path works with the DirectShow backend. The Linux code
  requests V4L2 explicitly to avoid GStreamer noise; Windows takes the default
  and is untested
- Confirm `mp4v` clips are written and playable on Windows
- Document the whole thing in `docs/DEVELOPMENT.md` while it is fresh

Do this before CI. Automating a build nobody has performed by hand is how days
disappear.

## Phase 3 — CI and releases

- `ci.yml`: `ubuntu-latest` and `windows-latest`, native builds only — no
  cross-compilation, which cgo makes impractical. Run `go vet` and the
  non-camera tests. Camera tests stay behind `-tags camera` because runners
  have no webcam.
- Cache OpenCV and the Go build cache per platform; a cold Windows OpenCV
  setup is slow enough to discourage use otherwise.
- `release.yml`: on a version tag, build both platforms and attach artifacts.
- Verify no `.env` is ever bundled. Declare installer resources explicitly and
  add a CI step that fails if a credential-shaped file appears in a bundle.
- Windows binaries ship unsigned for now, so SmartScreen will warn. Say so in
  the README rather than letting users discover it.

## Phase 4 — documentation and release

- `README.md`: what it is, install, configure, a screenshot, and a plain
  statement that footage is uploaded to Telegram's servers — users may point
  this at rooms containing other people
- `docs/SETUP-TELEGRAM.md`: BotFather walkthrough, finding a chat ID,
  troubleshooting. The two failures already seen belong here: a `.env` saved
  with Windows line endings breaks the token, and only one process may hold the
  camera.
- `docs/ARCHITECTURE.md`: package boundaries and the notifier interface, so
  WhatsApp can be added without re-reading everything
- Tag `v0.1.0`

## Deferred, deliberately

- **WhatsApp.** The `Notifier` interface exists for it. `whatsmeow` is the only
  option that fits a local app, and it carries a ToS and account-ban risk that
  belongs in front of users, not buried in a release.
- **macOS.** Needs paid Apple enrollment to be openable at all, plus an
  `Info.plist` camera-usage declaration.
- **Code signing.** Roughly $99/yr Apple, $200–400/yr Windows.

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Windows gocv build | Blocks half the release | Phase 2 by hand before automating |
| gocv pinned to v0.31.0 (2022) | Stuck on old bindings | Only ~9 API calls used, none changed since |
| Camera contention | Confusing failures for users | Single-instance guard, clear error text |
| Wails webkit 4.0 vs 4.1 | Linux build fails on 24.04 | Build tag in the Makefile, not in memory |
| Disk filled by recordings | App degrades over weeks | Retention implemented; UI must expose it |
