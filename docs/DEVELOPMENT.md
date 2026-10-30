# Development

Building SpareEye from source, on Linux and on Windows.

> **Status.** Linux instructions are verified against Ubuntu 24.04. Windows
> instructions follow gocv's own upstream Windows CI configuration.

---

## Repository layout

```
apps/
  cli/        headless CLI (spareeye-cli)
  desktop/    Wails desktop app: Go bindings plus frontend/
  site/       landing page, deployed by Netlify (see netlify.toml)
internal/     shared engine: detector, media, notify, onvif, config, autostart, app
build/        packaging files Wails requires; see build/README.md
docs/         user and developer guides
scripts/      repository checks run in CI
```

---

## The one thing to understand first: gocv is pinned to OpenCV 4.6.0

This project depends on `gocv.io/x/gocv v0.31.0`, which is welded to
**OpenCV 4.6.0**. Not "4.6 or later". Exactly 4.6.

Two independent reasons, both load-bearing:

1. **gocv links OpenCV by version-suffixed library name.** Its cgo directives
   contain `-lopencv_core460`, `-lopencv_videoio460` and eighteen more in the
   same shape. Those filenames only exist in an OpenCV 4.6.0 install.
2. **Newer gocv does not compile against OpenCV 4.6.** gocv v0.43.0 fails to
   build, because the aruco C++ API changed between OpenCV releases and the
   newer bindings assume the newer API:

   ```
   error: expected primary-expression before 'bool'
   ```

Ubuntu 24.04's `libopencv-dev` is exactly 4.6.0, which is why Linux setup is a
single `apt install` and Windows setup is an hour of compilation.

**Do not bump either version independently.** Upgrade both at once and expect
to fix call sites. CI enforces the Linux half with a
`pkg-config --modversion opencv4` check that fails on a mismatch, so a
distribution upgrade cannot silently break the link step months later.

---

## Linux: verified on Ubuntu 24.04

### Prerequisites

```sh
sudo apt update
sudo apt install --no-install-recommends -y \
  build-essential \
  pkg-config \
  libopencv-dev \
  libgtk-3-dev \
  libwebkit2gtk-4.1-dev
```

Confirm the OpenCV version:

```sh
pkg-config --modversion opencv4   # must print 4.6.0
```

You also need Go (see *Pinning Go*), Node 20+ for the frontend, and Wails:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

### Building

The Makefile holds the platform flags so nobody has to remember them:

```sh
make check          # gofmt, go vet, go test
make build-cli      # headless CLI       -> build/bin/spareeye-cli
make build-desktop  # desktop app        -> build/bin/spareeye
make dev            # desktop app with hot reload
make frontend       # web assets         -> apps/desktop/frontend/dist
make package-linux  # release build for linux/amd64
```

`apps/desktop/frontend/dist` is uncommitted build output, but `main.go` embeds it with
`//go:embed all:frontend/dist`. Anything compiling the desktop app package, including
a bare `go vet ./...` or `go test ./...`, therefore fails on a fresh clone with
`pattern all:frontend/dist: no matching files found` until the assets have been
built once. `make vet` and `make test` depend on `make frontend`, so only a
direct `go` invocation hits this, and `make frontend` is the fix. CI builds the
frontend before its first Go step for the same reason.

### Why `-tags webkit2_41`

Ubuntu 24.04 dropped `webkit2gtk-4.0`. Wails v2.13 still looks for the 4.0
series by default, so an untagged build fails at the pkg-config stage. The tag
switches it to the 4.1 packages 24.04 ships.

`make build-desktop` and `make dev` apply the tag automatically on Linux. If
you invoke Wails yourself, you must pass it:

```sh
(cd apps/desktop && wails build -tags webkit2_41 -clean)
```

### `wails doctor` says libwebkit is "Not Found": this is expected

`wails doctor` on Ubuntu 24.04 reports:

```
libwebkit: Not Found
```

**Harmless. Ignore it.** `wails doctor` probes for the 4.0 package
unconditionally; it cannot know you intend to build with the 4.1 tag. The build
works. Do not "fix" it by installing `libwebkit2gtk-4.0-dev` from a third-party
PPA.

---

## Windows: UNVERIFIED, best-effort

> **Read this first.** No one has completed this build. The steps below derive
> from gocv's own Windows CI workflow, its `win_build_opencv.cmd` script, and
> the cgo directives in gocv v0.31.0's `cgo.go`. They are internally consistent
> and based on primary sources, but never executed. Expect to iterate, and when
> you get it working please replace this warning with what actually happened.
> The CI Windows job carries the same caveat; its first real run is expected to
> need fixes.

### Why this is the hard part

There is no usable prebuilt OpenCV for this configuration:

- **The Chocolatey `opencv` package will not work.** It unpacks the official
  `opencv-4.6.0-vc14_vc15.exe`, whose MSVC import libraries live under
  `build\x64\vc14` and `vc15`. gocv links with MinGW and expects
  `x64\mingw\lib`. MinGW's linker cannot consume MSVC `.lib` files, and the
  C++ ABIs are incompatible anyway. The package also ships no `opencv_contrib`
  modules, and gocv links nine of them.
- **`huihut/OpenCV-MinGW-Build`**, the best-known prebuilt MinGW OpenCV, stops
  at 4.5.5.
- No public prebuilt OpenCV 4.6.0 + contrib + MinGW x64 artifact appears to
  exist.

So OpenCV must be built from source with MinGW: a **60–80 minute** compile.

### Prerequisites

- **MinGW-w64 (x86_64)** with `gcc`, `g++` and `mingw32-make` on `PATH`.
  GitHub's `windows-2022` runner already ships this at `C:\mingw64\bin`.
  Otherwise install [mingw-w64 from
  niXman/mingw-builds-binaries](https://github.com/niXman/mingw-builds-binaries/releases)
  and add its `bin` to `PATH`.
- **CMake**
- **Go** (see *Pinning Go*), **Node 20+**, **NSIS** (for the installer),
  and **Wails v2.13.0**.

### Build OpenCV 4.6.0

Download both archives and extract them side by side under a working
directory, here `C:\opencv`.

- `https://github.com/opencv/opencv/archive/4.6.0.zip`
- `https://github.com/opencv/opencv_contrib/archive/4.6.0.zip`

`opencv_contrib` is **not optional**: gocv links `xfeatures2d`, `tracking`,
`face`, `aruco`, `bgsegm`, `plot`, `img_hash`, `wechat_qrcode` and `ximgproc`,
all of which live there.

```bat
cmake C:\opencv\opencv-4.6.0 ^
  -G "MinGW Makefiles" ^
  -BC:\opencv\build ^
  -DCMAKE_INSTALL_PREFIX=C:\opencv\build\install ^
  -DOPENCV_EXTRA_MODULES_PATH=C:\opencv\opencv_contrib-4.6.0\modules ^
  -DENABLE_CXX11=ON -DBUILD_SHARED_LIBS=ON -DOPENCV_ENABLE_NONFREE=ON ^
  -DBUILD_opencv_gapi=OFF -DWITH_IPP=OFF -DWITH_MSMF=OFF ^
  -DWITH_OPENCL_D3D11_NV=OFF -DBUILD_EXAMPLES=OFF -DBUILD_TESTS=OFF ^
  -DBUILD_PERF_TESTS=OFF -DBUILD_DOCS=OFF -DBUILD_opencv_java=OFF ^
  -DBUILD_opencv_python=OFF -DBUILD_opencv_python2=OFF ^
  -DBUILD_opencv_python3=OFF -DBUILD_opencv_saliency=OFF ^
  -DBUILD_opencv_wechat_qrcode=ON -DENABLE_PRECOMPILED_HEADERS=OFF ^
  -DCPU_DISPATCH= -DOPENCV_GENERATE_PKGCONFIG=ON ^
  -DOPENCV_ALLOCATOR_STATS_COUNTER_TYPE=int64_t -Wno-dev

cd C:\opencv\build
mingw32-make -j%NUMBER_OF_PROCESSORS%
mingw32-make install
```

The build list is deliberately narrow. Java, Python, docs, examples, tests,
perf tests and saliency are off to keep an already long compile short.
`-DOPENCV_ENABLE_NONFREE=ON` is required because gocv links `xfeatures2d`.
`-DBUILD_opencv_gapi=OFF` is there because G-API is a well-known MinGW build
failure and gocv does not use it.

gocv's own `win_build_opencv.cmd` is **not** invoked directly: it hardcodes
`C:\opencv` and ends with a `chdir` into a `GOPATH`-style source layout this
project does not use.

### Point cgo at the install

Installed to exactly `C:\opencv\build\install`, gocv's built-in cgo directives
already point there and no environment variables are needed.

For any other location, build with the **`customenv`** tag, which tells gocv to
ignore its baked-in directives, and set:

```
CGO_CXXFLAGS=--std=c++11 -DNDEBUG
CGO_CPPFLAGS=-I<prefix>\build\install\include
CGO_LDFLAGS=-L<prefix>\build\install\x64\mingw\lib -lopencv_core460 -lopencv_face460 -lopencv_videoio460 -lopencv_imgproc460 -lopencv_highgui460 -lopencv_imgcodecs460 -lopencv_objdetect460 -lopencv_features2d460 -lopencv_video460 -lopencv_dnn460 -lopencv_xfeatures2d460 -lopencv_plot460 -lopencv_tracking460 -lopencv_img_hash460 -lopencv_calib3d460 -lopencv_bgsegm460 -lopencv_photo460 -lopencv_aruco460 -lopencv_wechat_qrcode460 -lopencv_ximgproc460
```

Also add `<prefix>\build\install\x64\mingw\bin` to `PATH`. It holds the DLLs,
needed at **run** time as well as link time. Without it the binary links fine
and then dies immediately with exit status `3221225781`
(`STATUS_DLL_NOT_FOUND`).

### Build

```bat
go build -tags customenv -o build\bin\spareeye-cli.exe .\apps\cli
cd apps\desktop && wails build -tags customenv -clean -platform windows/amd64 -nsis
```

No webkit tag on Windows. The UI uses the system WebView2 runtime, which the
installer provisions if it is missing.

### Known risks not yet hit

- **GCC 14 versus OpenCV 4.6.0.** Current MinGW builds ship GCC 14. GCC 13
  stopped including `<cstdint>` transitively from other libstdc++ headers, and
  OpenCV 4.6.0 predates that change. If the compile fails with
  `'uint32_t' was not declared in this scope`, either patch the offending
  headers or pin GCC 11 or 12 via `egor-tensin/setup-mingw@v2`.
- **Camera backend.** Linux requests V4L2 explicitly to suppress GStreamer
  noise; Windows takes OpenCV's default, DirectShow. Untested.
- **Clip encoding on Windows** is likewise unconfirmed. The recorder tries H.264
  and falls back to `mp4v`; which one a Windows build ends up with is unknown.

---

## Pinning Go

**Two Go installations are often visible at once**, `go version` reporting one
release and `wails doctor` another. A build can succeed under one and fail
under the other, and which tool found which is rarely obvious.

`go.mod` is the single source of truth. CI uses:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
```

so CI cannot become a third answer. Locally, check with `go version` and
`which -a go`, and make sure the one you want comes first on `PATH`.

---

## Tests

```sh
make test          # everything that does not need a camera
make test-camera   # camera integration tests
```

Camera integration tests sit behind a `//go:build camera` constraint, so plain
`go test ./...` excludes them. That is deliberate: CI runners have no webcam.
There are currently three: `TestManualClipContainsFrames`,
`TestPreviewProducesDecodableJPEG` and `TestStatusReportsCameraGeometry`.

Running them needs a real, free webcam:

```sh
go test -tags camera ./internal/detector/ -v
```

**Close the app first.** Only one process can hold the camera; if the desktop
app or another test run has it, these fail with an opaque backend error.

---

## Troubleshooting

### "Only one process can hold the camera"

Opening a camera already in use fails with a confusing low-level backend error
rather than a clean "device busy". If the detector will not start, or camera
tests fail immediately, check for another instance:

```sh
pgrep -a 'spareeye'
fuser -v /dev/video*
```

This is also why the `.desktop` entry sets `SingleMainWindow=true`.

### Camera index 0 opens but produces no frames

Index 0 is frequently a **virtual** device, not the real webcam: it opens
successfully and yields nothing, so a naive "can we open it?" check reports it
as working. Hence enumeration requires a real decoded frame before calling a
device usable, and the UI shows a picker rather than assuming index 0.

For a black or frozen preview, try another index:

```sh
make cameras   # lists only devices that actually delivered a frame
```

### `invalid control character in URL` from Telegram

The bot token was read with a trailing carriage return, which happens when
`.env` is saved with Windows (CRLF) line endings: the `\r` joins the token
value and then the request URL.

Fix the line endings:

```sh
dos2unix .env      # or: sed -i 's/\r$//' .env
```

Configuring the token through the application UI avoids this; the `.env` path
exists for the headless CLI.

### `wails doctor` reports libwebkit "Not Found"

Expected on Ubuntu 24.04. See the Linux section above: build with
`-tags webkit2_41` and ignore the report.

### Link errors mentioning `opencv_core460`

Your OpenCV is not 4.6.0. Check with `pkg-config --modversion opencv4`, then
see the pinning section at the top of this document.

---

## Credentials and the leak guard

A live Telegram bot token was once hardcoded in this project's source. That is
why the current structure exists, and `scripts/check-no-credentials.sh` stops a
recurrence. It runs in CI on every push and before every release, failing the
build if:

- any tracked file contains a Telegram-bot-token-shaped string
  (`[0-9]{8,10}:[A-Za-z0-9_-]{35}`), or
- an `.env` file is tracked by git, or
- an `.env` file appears in a build output directory or release bundle.

Run it yourself any time:

```sh
./scripts/check-no-credentials.sh            # tracked source only
./scripts/check-no-credentials.sh build/bin  # plus a build output
```

It scans `git ls-files`, not the working tree, so your local `.env` is never
read. On a hit it prints the file and line number but **not** the matched text,
so a secret is never copied into a public CI log.

`.env` is gitignored and **must stay that way**. If the guard ever fires on a
real token, revoke it with `@BotFather` `/revoke` first. A pushed secret is
compromised whether or not you amend the commit afterwards.

---

## CI and releases

`.github/workflows/ci.yml` runs on every push and pull request:

| Job | Runner | What it does |
|---|---|---|
| `credential-scan` | `ubuntu-latest` | Leak guard, fast, first |
| `linux` | `ubuntu-latest` | apt deps, gofmt, vet, test, CLI + desktop build |
| `windows` | `windows-2022` | OpenCV build, vet, test, build; **unvalidated** |

`.github/workflows/release.yml` runs on a `v*` tag and publishes a GitHub
Release with a Linux `.tar.gz`, a Linux `.deb`, and a Windows NSIS installer,
each with a `.sha256`, plus generated release notes.

The `.deb` is assembled by hand with `dpkg-deb` in the `linux` job. Its
`Depends` are not written out anywhere: `dpkg-shlibdeps` reads them off the
built binary, so the package requires the OpenCV and WebKit *runtime* packages
rather than the `-dev` packages CI compiles against. `lintian --fail-on error`
runs on the result, so a packaging mistake fails the release.

Everything is built on **native runners**. There are no `GOOS`/`GOARCH`
cross-builds anywhere, because cgo makes cross-compiling gocv impractical.
macOS is out of scope for this release.

### Expected CI times

| | Cold cache | Warm cache |
|---|---|---|
| Linux | ~3–5 min | ~2–3 min |
| Windows | **~60–80 min** (OpenCV compile) | ~5 min |

The Windows figures are extrapolated from gocv's own CI, where a cold OpenCV
4.13.0 build measured 1 h 18 m and warm runs finished under 5 minutes. OpenCV
4.6.0 is a smaller tree and this configuration disables perf tests and G-API,
so the lower end is plausible. Treat it as an estimate, not a measurement.

OpenCV source and build tree are cached **separately**, so a failed compile
still leaves a reusable download for the retry. Bump the `v1` suffix in the
cache keys whenever you change the cmake flags, or a stale build tree will be
restored and your change silently ignored.

The Linux and Windows jobs are independent, so the unvalidated Windows job can
never block or mask a good Linux result.

### Signing

Binaries ship **unsigned**. Windows SmartScreen will warn that the publisher is
unknown; the release notes say so explicitly. Signing hooks are present but
commented out in `build/windows/installer/project.nsi`.
