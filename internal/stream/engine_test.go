//go:build parked

package stream

import (
	"os/exec"
	"strings"
	"testing"
)

// fakeSpawn replaces process creation so tests never run a real ffmpeg.
func fakeSpawn(captured *[][]string) func(args []string) (*exec.Cmd, error) {
	return func(args []string) (*exec.Cmd, error) {
		*captured = append(*captured, append([]string{}, args...))
		return &exec.Cmd{}, nil
	}
}

func TestStartEmptyRTMP(t *testing.T) {
	for _, tc := range []struct {
		name, url, key string
	}{
		{"empty url", "", "key123"},
		{"empty key", "rtmp://live.example/app", ""},
		{"both empty", "", ""},
	} {
		e := New()
		var captured [][]string
		e.spawnFn = fakeSpawn(&captured)
		if err := e.Start(tc.url, tc.key, ""); err == nil {
			t.Fatalf("%s: expected error for empty rtmp, got nil", tc.name)
		} else if !strings.Contains(err.Error(), "missing rtmp url/key") {
			t.Fatalf("%s: unexpected error %q", tc.name, err)
		}
		if len(captured) != 0 {
			t.Fatalf("%s: spawn must not be called on validation error", tc.name)
		}
		if e.Running() {
			t.Fatalf("%s: engine must not be running after failed start", tc.name)
		}
	}
}

func TestStartTwiceAlreadyRunning(t *testing.T) {
	e := New()
	var captured [][]string
	e.spawnFn = fakeSpawn(&captured)

	if err := e.Start("rtmp://live.example/app", "key123", ""); err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer e.Stop()
	if !e.Running() {
		t.Fatal("expected running after start")
	}

	// dest must be rtmpURL + "/" + rtmpKey, last arg of ffmpeg command.
	if len(captured) != 1 {
		t.Fatalf("expected one spawn, got %d", len(captured))
	}
	got := captured[0][len(captured[0])-1]
	want := "rtmp://live.example/app/key123"
	if got != want {
		t.Fatalf("rtmp dest = %q, want %q", got, want)
	}

	err := e.Start("rtmp://live.example/app", "key123", "")
	if err == nil {
		t.Fatal("second start: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second start: error %q does not contain %q", err, "already running")
	}
	if len(captured) != 1 {
		t.Fatalf("second start must not spawn again (spawns=%d)", len(captured))
	}
}

func TestBuildArgsV1Pipeline(t *testing.T) {
	args := buildArgs("rtmp://live.example/app/key123", "")
	want := []string{
		"-re", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency",
		"-c:a", "aac", "-f", "flv", "rtmp://live.example/app/key123",
	}
	if len(args) != len(want) {
		t.Fatalf("args len %d, want %d", len(args), len(want))
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
	// disclosure accepted but does not change v1 args (bookkeeping only).
	args2 := buildArgs("rtmp://x/k", "AI-generated content")
	if len(args2) != len(want) {
		t.Fatalf("disclosure changed arg count: %d vs %d", len(args2), len(want))
	}
}

func TestPushSegmentNotRunning(t *testing.T) {
	e := New()
	if err := e.PushSegment([]byte{0x52, 0x49, 0x46, 0x46}, "hi"); err == nil {
		t.Fatal("expected error pushing before start, got nil")
	} else if !strings.Contains(err.Error(), "stream not running") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestPushSegmentFlow(t *testing.T) {
	e := New()
	var captured [][]string
	e.spawnFn = fakeSpawn(&captured)
	if err := e.Start("rtmp://live.example/app", "key123", ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer e.Stop()

	if err := e.PushSegment(nil, "empty"); err == nil {
		t.Fatal("expected error for empty audio, got nil")
	}

	audio := []byte{0x52, 0x49, 0x46, 0x46, 0x57, 0x41, 0x56, 0x45}
	if err := e.PushSegment(audio, "hello"); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := e.PushSegment(audio, "world"); err != nil {
		t.Fatalf("push 2: %v", err)
	}
	if got := e.QueueLen(); got != 2 {
		t.Fatalf("QueueLen = %d, want 2", got)
	}
	if got := e.SegmentsPushed(); got != 2 {
		t.Fatalf("SegmentsPushed = %d, want 2", got)
	}

	// after stop, pushes are rejected again.
	if err := e.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if e.Running() {
		t.Fatal("expected not running after stop")
	}
	if err := e.PushSegment(audio, "late"); err == nil {
		t.Fatal("expected error pushing after stop, got nil")
	}
}

func TestStopNotStartedNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop panicked on fresh engine: %v", r)
		}
	}()
	e := New()
	if err := e.Stop(); err != nil {
		t.Fatalf("Stop on fresh engine returned error: %v", err)
	}
	if e.Running() {
		t.Fatal("fresh engine must not be running")
	}
}

func TestQueueBounded(t *testing.T) {
	e := New()
	var captured [][]string
	e.spawnFn = fakeSpawn(&captured)
	if err := e.Start("rtmp://live.example/app", "key123", ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer e.Stop()

	for i := 0; i < maxQueueLen+10; i++ {
		if err := e.PushSegment([]byte{byte(i)}, "seg"); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	if got := e.QueueLen(); got != maxQueueLen {
		t.Fatalf("QueueLen = %d, want bounded %d", got, maxQueueLen)
	}
	if got := e.SegmentsPushed(); got != maxQueueLen+10 {
		t.Fatalf("SegmentsPushed = %d, want %d", got, maxQueueLen+10)
	}
}
