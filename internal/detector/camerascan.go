package detector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CameraScanFlag puts this executable into scan mode. It carries the index to
// start from, as in --scan-cameras=3, so a parent whose helper died on a device
// can ask for the rest of the sweep.
//
// The flag is not documented to users: it exists for one process to talk to
// another copy of itself.
const CameraScanFlag = "--scan-cameras"

// CameraTestFlag puts this executable into single-source mode, carrying the
// network address to open, as in --test-camera=rtsp://camera.lan/stream.
//
// A stream can wedge a capture backend exactly as a local driver can, so it is
// opened where that costs a child process rather than the window. One source is
// the degenerate case of a sweep, so it reports on the same protocol and is read
// back by the same parser.
const CameraTestFlag = "--test-camera"

// cameraScanTimeout bounds one helper invocation. A driver that hangs never
// returns from the open call, so without a deadline the app would wait forever
// with a window that cannot arm.
//
// Eight devices at the couple of seconds a healthy open costs is comfortably
// under this, and the worst case — a hang on every index in turn — is bounded
// by the restart limit rather than by this value alone.
const cameraScanTimeout = 20 * time.Second

// cameraScanLine is one record of the helper's protocol, which is a stream of
// JSON objects, one per line:
//
//	{"probing":0}
//	{"camera":{"index":0,"width":640,"height":480,"label":"Camera 0 (640x480)"}}
//	{"probing":1}
//	{"done":true}
//
// An index is announced before it is opened, which is what makes a truncated
// stream readable: the last announcement names the device that took the helper
// down, and everything reported before it still counts.
type cameraScanLine struct {
	Probing *int    `json:"probing,omitempty"`
	Camera  *Camera `json:"camera,omitempty"`
	Done    bool    `json:"done,omitempty"`
}

// cameraScanOutcome is what one helper invocation managed to report.
type cameraScanOutcome struct {
	found []Camera

	// lastProbed is the highest index the helper announced, or -1 when it ended
	// before announcing anything.
	lastProbed int

	// complete records that the helper reached the end of its sweep, which is
	// the only way to know the stream was not cut short.
	complete bool
}

// CameraScanArgument is the argument that asks this executable to scan from
// startIndex.
func CameraScanArgument(startIndex int) string {
	return fmt.Sprintf("%s=%d", CameraScanFlag, startIndex)
}

// CameraScanStart reports whether the given arguments ask for a scan, and where
// the sweep should begin.
//
// The arguments are scanned rather than parsed with the flag package, because
// flag.Parse exits the process on anything it does not recognise and a webview
// host is entitled to pass arguments of its own.
func CameraScanStart(arguments []string) (int, bool) {
	for _, argument := range arguments {
		if !strings.HasPrefix(argument, CameraScanFlag+"=") {
			continue
		}

		startIndex, err := strconv.Atoi(strings.TrimPrefix(argument, CameraScanFlag+"="))
		if err != nil || startIndex < 0 || startIndex >= cameraProbeLimit {
			return 0, false
		}

		return startIndex, true
	}

	return 0, false
}

// CameraTestArgument is the argument that asks this executable to open one
// network address.
func CameraTestArgument(address string) string {
	return fmt.Sprintf("%s=%s", CameraTestFlag, address)
}

// CameraTestSource reports whether the given arguments ask for a single stream
// to be opened, and which one.
//
// Like CameraScanStart it walks the arguments rather than using the flag
// package, which exits the process on anything it does not recognise.
func CameraTestSource(arguments []string) (string, bool) {
	for _, argument := range arguments {
		if !strings.HasPrefix(argument, CameraTestFlag+"=") {
			continue
		}

		address := strings.TrimPrefix(argument, CameraTestFlag+"=")
		if address == "" {
			return "", false
		}

		return address, true
	}

	return "", false
}

// WriteCameraTest opens one network stream and reports what it saw on the same
// line protocol the sweep uses. This is what the executable does in test mode.
func WriteCameraTest(output io.Writer, address string) {
	defer pinCameraThread()()

	encoder := json.NewEncoder(output)

	announced := 0
	encoder.Encode(cameraScanLine{Probing: &announced})

	if stream, err := probeStream(address); err == nil {
		encoder.Encode(cameraScanLine{Camera: &stream})
	}

	encoder.Encode(cameraScanLine{Done: true})
}

// WriteCameraScan probes devices from startIndex onwards and reports each step
// on the writer. This is what the executable does in scan mode.
//
// It reports as it goes rather than at the end, because the reason it runs in a
// child process at all is that any one of these opens can end the process from
// inside the driver, and results already sent have already survived.
func WriteCameraScan(output io.Writer, startIndex int) {
	defer pinCameraThread()()

	encoder := json.NewEncoder(output)

	for index := startIndex; index < cameraProbeLimit; index++ {
		announced := index
		encoder.Encode(cameraScanLine{Probing: &announced})

		camera, err := probeCamera(index)
		if err != nil {
			continue
		}
		encoder.Encode(cameraScanLine{Camera: &camera})
	}

	encoder.Encode(cameraScanLine{Done: true})
}

// AvailableCamerasIsolated enumerates capture devices in a child process, so a
// driver that ends the process it runs in ends that one instead of the app.
//
// A helper that cannot be started at all — the executable is not resolvable,
// security software refuses the exec — leaves probing in this process as the
// only way to find anything, and an app that reports no cameras because it
// could not spawn a helper is worse off than one that risks the open.
func AvailableCamerasIsolated(log *slog.Logger) []Camera {
	found, err := scanCamerasOutOfProcess(log)
	if err == nil {
		log.Info("probed cameras out of process", "count", len(found))
		return found
	}

	log.Warn("the camera scan helper could not run; probing in this process instead", "error", err)

	return AvailableCameras(func(index int) {
		log.Info("opening capture device", "index", index)
	})
}

// TestStreamIsolated opens one network stream in a child process and reports
// the picture it delivered, so an address that hangs or crashes the capture
// backend does that somewhere disposable.
//
// The fallback matches AvailableCamerasIsolated's: a helper that cannot be
// started at all leaves opening it here as the only way to answer at all, and a
// Setup screen that cannot test anything is worse off than one that risks it.
func TestStreamIsolated(log *slog.Logger, address string) (Camera, error) {
	stream, err := testStreamOutOfProcess(address)
	if err == nil {
		log.Info("tested a network stream out of process", "size", stream.Label)
		return stream, nil
	}

	if errors.Is(err, errStreamSentNoPicture) {
		return Camera{}, err
	}

	log.Warn("the camera scan helper could not run; testing this stream in this process instead", "error", err)

	return probeStream(address)
}

// errStreamSentNoPicture separates a helper that ran and found nothing from one
// that never ran, since only the second is worth retrying in this process.
var errStreamSentNoPicture = errors.New("the stream did not send a picture")

func testStreamOutOfProcess(address string) (Camera, error) {
	executable, err := scanHelperPath()
	if err != nil {
		return Camera{}, err
	}

	// The start index is meaningless for a single source and the helper ignores
	// it; what is being reused is the spawn, the deadline and the reader.
	outcome, err := runCameraScan(0, func(ctx context.Context, _ int) *exec.Cmd {
		return exec.CommandContext(ctx, executable, CameraTestArgument(address))
	})
	if err != nil {
		return Camera{}, err
	}

	if len(outcome.found) == 0 {
		return Camera{}, errStreamSentNoPicture
	}

	return outcome.found[0], nil
}

func scanCamerasOutOfProcess(log *slog.Logger) ([]Camera, error) {
	executable, err := scanHelperPath()
	if err != nil {
		return nil, err
	}

	return gatherCameras(log, func(startIndex int) (cameraScanOutcome, error) {
		return runCameraScan(startIndex, func(ctx context.Context, from int) *exec.Cmd {
			return exec.CommandContext(ctx, executable, CameraScanArgument(from))
		})
	})
}

// scanHelperPath is the real location of the running binary, which is also the
// helper: the app re-invokes itself rather than looking for a second program
// beside it, because nothing guarantees one is shipped there.
//
// Symlinks are resolved because a Linux user may launch the app through one in
// ~/.local/bin while the binary itself lives in an unpacked tarball anywhere.
func scanHelperPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating this program: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", executable, err)
	}

	return resolved, nil
}

// cameraScanCommand builds one helper invocation. Taking the command as an
// argument is what lets the parent's half of the protocol be exercised against
// a stand-in program.
type cameraScanCommand func(ctx context.Context, startIndex int) *exec.Cmd

// runCameraScan runs the helper once and reads what it says before it ends.
//
// Only a failure to start the process is an error: a helper that died partway
// through is the case this whole mechanism exists for, and it is described by
// the outcome rather than reported as a problem. The deadline is treated the
// same way, since a hung driver and a crashed one leave the parent with the
// same question — which index was being opened.
func runCameraScan(startIndex int, newCommand cameraScanCommand) (cameraScanOutcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cameraScanTimeout)
	defer cancel()

	command := newCommand(ctx, startIndex)

	stream, err := command.StdoutPipe()
	if err != nil {
		return cameraScanOutcome{}, fmt.Errorf("reading from the camera scan helper: %w", err)
	}

	if err := command.Start(); err != nil {
		return cameraScanOutcome{}, fmt.Errorf("starting the camera scan helper: %w", err)
	}

	outcome := readCameraScan(stream)

	// The exit status says nothing the stream has not already said: a helper
	// killed by its driver and one killed by the deadline both leave an
	// incomplete outcome, and the parent resumes from the same place either way.
	command.Wait()

	return outcome, nil
}

// readCameraScan reads the helper's stream to the end.
//
// Everything here is a boundary: the stream comes from another process that is
// expected to die halfway through a sentence. A line that does not parse, or
// that describes an index outside the sweep, is dropped rather than trusted.
func readCameraScan(stream io.Reader) cameraScanOutcome {
	outcome := cameraScanOutcome{lastProbed: -1}
	lines := bufio.NewScanner(stream)

	for lines.Scan() {
		var line cameraScanLine
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			continue
		}

		if line.Probing != nil && *line.Probing >= 0 && *line.Probing < cameraProbeLimit {
			outcome.lastProbed = *line.Probing
		}

		if line.Camera != nil && usableScanResult(*line.Camera) {
			outcome.found = append(outcome.found, *line.Camera)
		}

		if line.Done {
			outcome.complete = true
		}
	}

	return outcome
}

func usableScanResult(camera Camera) bool {
	return camera.Index >= 0 &&
		camera.Index < cameraProbeLimit &&
		camera.Width > 0 &&
		camera.Height > 0
}

// gatherCameras sweeps the devices across as many helper runs as it takes,
// resuming after each device that ended one.
//
// Without the resume a single hostile driver would hide every camera above it,
// which on a machine whose index 0 is a virtual webcam means all of them. Each
// run starts strictly after the index the previous run announced, so the sweep
// always moves forward and the restart count cannot exceed the number of
// devices there are to probe.
func gatherCameras(log *slog.Logger, runScan func(startIndex int) (cameraScanOutcome, error)) ([]Camera, error) {
	found := make([]Camera, 0, 1)
	nextIndex := 0

	for restarts := 0; restarts < cameraProbeLimit && nextIndex < cameraProbeLimit; restarts++ {
		outcome, err := runScan(nextIndex)
		if err != nil {
			if restarts == 0 {
				return nil, err
			}

			log.Warn("the camera scan helper could not be restarted", "from", nextIndex, "error", err)
			return found, nil
		}

		found = append(found, outcome.found...)

		if outcome.complete {
			return found, nil
		}

		// A run that ended without announcing anything never reached a device, so
		// the helper itself is what failed. On the first run that is grounds for
		// falling back to probing here; later it means the results already in hand
		// are all there are going to be.
		if outcome.lastProbed < nextIndex {
			if restarts == 0 {
				return nil, errors.New("the camera scan helper ended without probing anything")
			}

			log.Warn("the camera scan helper ended without probing anything", "from", nextIndex)
			return found, nil
		}

		log.Warn("a capture device ended the camera scan helper; resuming after it", "index", outcome.lastProbed)
		nextIndex = outcome.lastProbed + 1
	}

	return found, nil
}
