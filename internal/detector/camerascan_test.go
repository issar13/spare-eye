package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// quietLogger keeps the scan's own reporting out of the test output while still
// giving it somewhere to write.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestReadCameraScanAcceptsACompleteStream(t *testing.T) {
	stream := strings.NewReader(
		`{"probing":0}` + "\n" +
			`{"probing":1}` + "\n" +
			`{"camera":{"index":1,"width":640,"height":480,"label":"Camera 1 (640x480)"}}` + "\n" +
			`{"probing":2}` + "\n" +
			`{"done":true}` + "\n")

	outcome := readCameraScan(stream)

	if !outcome.complete {
		t.Fatal("a stream ending in done was not read as complete")
	}
	if outcome.lastProbed != 2 {
		t.Fatalf("last probed index is %d, want 2", outcome.lastProbed)
	}
	if len(outcome.found) != 1 || outcome.found[0].Index != 1 {
		t.Fatalf("found %+v, want camera 1 alone", outcome.found)
	}
}

func TestReadCameraScanNamesTheIndexATruncatedStreamDiedOn(t *testing.T) {
	stream := strings.NewReader(
		`{"probing":0}` + "\n" +
			`{"camera":{"index":0,"width":320,"height":240,"label":"Camera 0 (320x240)"}}` + "\n" +
			`{"probing":1}` + "\n" +
			`{"probi`)

	outcome := readCameraScan(stream)

	if outcome.complete {
		t.Fatal("a truncated stream was read as complete")
	}
	if outcome.lastProbed != 1 {
		t.Fatalf("last probed index is %d, want 1", outcome.lastProbed)
	}
	if len(outcome.found) != 1 {
		t.Fatalf("found %+v, want the camera reported before the stream ended", outcome.found)
	}
}

func TestReadCameraScanDropsLinesItCannotTrust(t *testing.T) {
	stream := strings.NewReader(
		`{"probing":0}` + "\n" +
			`not json at all` + "\n" +
			`{"camera":{"index":0,"width":0,"height":0,"label":""}}` + "\n" +
			`{"probing":99}` + "\n" +
			`{"camera":{"index":1,"width":640,"height":480,"label":"Camera 1 (640x480)"}}` + "\n" +
			`{"done":true}` + "\n")

	outcome := readCameraScan(stream)

	if outcome.lastProbed != 0 {
		t.Fatalf("last probed index is %d, want 0: an index outside the sweep is not a probe", outcome.lastProbed)
	}
	if len(outcome.found) != 1 || outcome.found[0].Index != 1 {
		t.Fatalf("found %+v, want only the camera with real dimensions", outcome.found)
	}
}

func TestReadCameraScanOnAnEmptyStream(t *testing.T) {
	outcome := readCameraScan(strings.NewReader(""))

	if outcome.complete || outcome.lastProbed != -1 || len(outcome.found) != 0 {
		t.Fatalf("an empty stream produced %+v", outcome)
	}
}

func TestGatherCamerasResumesAfterTheDeviceThatKilledTheHelper(t *testing.T) {
	var starts []int

	found, err := gatherCameras(quietLogger(), func(startIndex int) (cameraScanOutcome, error) {
		starts = append(starts, startIndex)

		if startIndex == 0 {
			// Index 0 is the hostile device: announced, then nothing.
			return cameraScanOutcome{lastProbed: 0}, nil
		}

		return cameraScanOutcome{
			found:      []Camera{{Index: 1, Width: 640, Height: 480}},
			lastProbed: cameraProbeLimit - 1,
			complete:   true,
		}, nil
	})

	if err != nil {
		t.Fatalf("gathering: %v", err)
	}
	if len(starts) != 2 || starts[0] != 0 || starts[1] != 1 {
		t.Fatalf("helper was started from %v, want a restart at index 1", starts)
	}
	if len(found) != 1 || found[0].Index != 1 {
		t.Fatalf("found %+v, want the camera the second run reported", found)
	}
}

func TestGatherCamerasKeepsWhatEachRunReportedBeforeDying(t *testing.T) {
	found, err := gatherCameras(quietLogger(), func(startIndex int) (cameraScanOutcome, error) {
		if startIndex >= cameraProbeLimit-1 {
			return cameraScanOutcome{lastProbed: cameraProbeLimit - 1, complete: true}, nil
		}

		return cameraScanOutcome{
			found:      []Camera{{Index: startIndex, Width: 640, Height: 480}},
			lastProbed: startIndex,
		}, nil
	})

	if err != nil {
		t.Fatalf("gathering: %v", err)
	}
	if len(found) != cameraProbeLimit-1 {
		t.Fatalf("found %d cameras, want one from each run that died", len(found))
	}
}

// TestGatherCamerasStopsWhenEveryRunDies covers the machine where nothing works
// at all: the sweep has to end rather than restart forever.
func TestGatherCamerasStopsWhenEveryRunDies(t *testing.T) {
	runs := 0

	found, err := gatherCameras(quietLogger(), func(startIndex int) (cameraScanOutcome, error) {
		runs++
		return cameraScanOutcome{lastProbed: startIndex}, nil
	})

	if err != nil {
		t.Fatalf("gathering: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found %+v, want nothing", found)
	}
	if runs > cameraProbeLimit {
		t.Fatalf("the helper was run %d times, want at most %d", runs, cameraProbeLimit)
	}
}

// TestGatherCamerasReportsAHelperThatNeverStarts is what sends the app back to
// probing in its own process.
func TestGatherCamerasReportsAHelperThatNeverStarts(t *testing.T) {
	_, err := gatherCameras(quietLogger(), func(startIndex int) (cameraScanOutcome, error) {
		return cameraScanOutcome{}, exec.ErrNotFound
	})

	if err == nil {
		t.Fatal("a helper that could not be started was reported as a successful scan")
	}
}

func TestGatherCamerasReportsAHelperThatProbesNothing(t *testing.T) {
	_, err := gatherCameras(quietLogger(), func(startIndex int) (cameraScanOutcome, error) {
		return cameraScanOutcome{lastProbed: -1}, nil
	})

	if err == nil {
		t.Fatal("a helper that announced nothing on its first run was reported as a successful scan")
	}
}

// TestRunCameraScanReadsARealChildProcess exercises the pipe, the wait and the
// parsing together.
func TestRunCameraScanReadsARealChildProcess(t *testing.T) {
	outcome, err := runCameraScan(0, helperCommand(sweepToTheEnd))

	if err != nil {
		t.Fatalf("running the helper: %v", err)
	}
	if !outcome.complete {
		t.Fatalf("outcome is %+v, want a complete sweep", outcome)
	}
	if len(outcome.found) != 1 || outcome.found[0].Index != 0 {
		t.Fatalf("found %+v, want the one camera the child reported", outcome.found)
	}
}

// TestRunCameraScanSurvivesAChildThatDies is the case the whole mechanism is
// for: the child ends abruptly, and the parent is left with a usable answer
// instead of an error.
func TestRunCameraScanSurvivesAChildThatDies(t *testing.T) {
	outcome, err := runCameraScan(0, helperCommand(dieOnTheFirstDevice))

	if err != nil {
		t.Fatalf("running the helper: %v", err)
	}
	if outcome.complete {
		t.Fatal("a child that died reported a complete sweep")
	}
	if outcome.lastProbed != 0 {
		t.Fatalf("last probed index is %d, want the 0 the child announced before dying", outcome.lastProbed)
	}
}

func TestRunCameraScanReportsAnExecutableThatIsNotThere(t *testing.T) {
	_, err := runCameraScan(0, func(ctx context.Context, startIndex int) *exec.Cmd {
		return exec.CommandContext(ctx, filepath.Join(t.TempDir(), "no-such-helper"))
	})

	if err == nil {
		t.Fatal("a helper that does not exist was run successfully")
	}
}

func TestCameraScanStartReadsItsOwnArgument(t *testing.T) {
	startIndex, scanning := CameraScanStart([]string{"--started-at-login", CameraScanArgument(3)})

	if !scanning || startIndex != 3 {
		t.Fatalf("parsed %d, %v; want 3, true", startIndex, scanning)
	}
}

func TestCameraScanStartRejectsArgumentsItCannotUse(t *testing.T) {
	for _, argument := range []string{"--started-at-login", CameraScanFlag, CameraScanFlag + "=", CameraScanFlag + "=-1", CameraScanFlag + "=99"} {
		if _, scanning := CameraScanStart([]string{argument}); scanning {
			t.Fatalf("%q was accepted as a scan request", argument)
		}
	}
}

func TestCameraTestSourceReadsItsOwnArgument(t *testing.T) {
	address, testing := CameraTestSource([]string{"--started-at-login", CameraTestArgument("rtsp://camera.lan/live")})

	if !testing || address != "rtsp://camera.lan/live" {
		t.Fatalf("parsed %q, %v; want the address, true", address, testing)
	}
}

func TestCameraTestSourceRejectsArgumentsItCannotUse(t *testing.T) {
	for _, argument := range []string{"--started-at-login", CameraTestFlag, CameraTestFlag + "=", CameraScanArgument(0)} {
		if _, testing := CameraTestSource([]string{argument}); testing {
			t.Fatalf("%q was accepted as a stream test request", argument)
		}
	}
}

// TestAStreamReplyIsReadBackByTheSweepsReader is what lets a single-source test
// reuse the sweep's machinery instead of growing a protocol of its own.
func TestAStreamReplyIsReadBackByTheSweepsReader(t *testing.T) {
	var reply strings.Builder
	stream := Camera{Width: 1920, Height: 1080, Label: "Network stream (1920x1080)"}

	announced := 0
	encoder := json.NewEncoder(&reply)
	encoder.Encode(cameraScanLine{Probing: &announced})
	encoder.Encode(cameraScanLine{Camera: &stream})
	encoder.Encode(cameraScanLine{Done: true})

	outcome := readCameraScan(strings.NewReader(reply.String()))

	if !outcome.complete {
		t.Fatal("a finished stream test was not read as complete")
	}
	if len(outcome.found) != 1 || outcome.found[0].Width != 1920 {
		t.Fatalf("found %+v, want the stream the child reported", outcome.found)
	}
}

// The spawning tests re-invoke this test binary rather than a shell, so they
// run the same way on every platform the app is built for.
const (
	childBehaviourVariable = "SPAREEYE_CAMERA_SCAN_CHILD"

	sweepToTheEnd       = "sweep-to-the-end"
	dieOnTheFirstDevice = "die-on-the-first-device"
)

func helperCommand(behaviour string) cameraScanCommand {
	return func(ctx context.Context, startIndex int) *exec.Cmd {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestCameraScanChildBehaviour")
		command.Env = append(os.Environ(), childBehaviourVariable+"="+behaviour)

		return command
	}
}

// TestCameraScanChildBehaviour is the child the spawning tests run. It stands
// in for a helper whose devices are hostile, which is not something a real
// camera can be asked to be.
//
// The framework's own "PASS" and timing lines go to the same stdout, and the
// parent dropping them is part of what is under test.
func TestCameraScanChildBehaviour(t *testing.T) {
	behaviour := os.Getenv(childBehaviourVariable)
	if behaviour == "" {
		t.Skip("this test is only meaningful as the child of another test")
	}

	fmt.Println(`{"probing":0}`)

	if behaviour == dieOnTheFirstDevice {
		os.Exit(1)
	}

	fmt.Println(`{"camera":{"index":0,"width":640,"height":480,"label":"Camera 0 (640x480)"}}`)
	fmt.Println(`{"done":true}`)
}
