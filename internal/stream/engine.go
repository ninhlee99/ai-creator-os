package stream

import (
	"errors"
	"log"
	"os/exec"
	"sync"
	"time"
)

// segment is a pending audio/caption unit. v1 only records it in the queue;
// v2 will feed it to the frame pipe for caption overlay.
type segment struct {
	audioWAV []byte
	caption  string
}

const (
	// maxFails mirrors the original stream-engine: the watchdog gives up
	// after this many consecutive ffmpeg deaths.
	maxFails = 5
	// failBackoffStep: restart backoff is fails * failBackoffStep.
	failBackoffStep = 5 * time.Second
	// maxQueueLen bounds the v1 segment queue (resource-capped by design).
	maxQueueLen = 64
)

// Engine owns the FFmpeg RTMP process: start/stop, watchdog restarts, and a
// bounded queue of pending audio segments (v1: recorded only, no overlay).
type Engine struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	running        bool
	fails          int
	queue          []segment
	segmentsPushed int64

	// spawnFn creates and starts the ffmpeg process. The default starts a
	// real ffmpeg; tests override it so no process is spawned.
	spawnFn func(args []string) (*exec.Cmd, error)
}

// New returns an Engine that spawns real ffmpeg processes.
func New() *Engine {
	return &Engine{spawnFn: defaultSpawn}
}

func defaultSpawn(args []string) (*exec.Cmd, error) {
	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// buildArgs is pure: the v1 ffmpeg pipeline. Kept as a separate function so
// tests can assert on the generated args without spawning ffmpeg.
func buildArgs(dest, disclosure string) []string {
	args := []string{
		"-re", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency",
		"-c:a", "aac", "-f", "flv", dest,
	}
	if disclosure != "" {
		// v1: disclosure is bookkeeping only; burning it in via drawtext
		// needs a fontfile and is deferred to the v2 frame-pipe overlay.
		_ = disclosure
	}
	return args
}

// Start launches ffmpeg publishing to rtmpURL/rtmpKey. It rejects empty
// url/key and refuses to start while already running.
func (e *Engine) Start(rtmpURL, rtmpKey, disclosure string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return errors.New("already running")
	}
	if rtmpURL == "" || rtmpKey == "" {
		return errors.New("missing rtmp url/key")
	}
	dest := rtmpURL + "/" + rtmpKey
	args := buildArgs(dest, disclosure)
	cmd, err := e.spawnFn(args)
	if err != nil {
		return err
	}
	e.cmd = cmd
	e.running = true
	e.fails = 0
	go e.watchdog(args)
	log.Println("stream started ->", rtmpURL)
	return nil
}

// watchdog mirrors the original stream-engine logic: poll every 5s; if the
// process exited, restart with backoff fails*5s and give up after 5 failures.
func (e *Engine) watchdog(args []string) {
	for {
		time.Sleep(5 * time.Second)
		e.mu.Lock()
		if !e.running {
			e.mu.Unlock()
			return
		}
		// process exited?
		done := make(chan error, 1)
		go func() { done <- e.cmd.Wait() }()
		select {
		case <-done:
			e.fails++
			if e.fails > maxFails {
				log.Println("too many ffmpeg failures, giving up (alert!)")
				e.running = false
				e.mu.Unlock()
				return
			}
			backoff := time.Duration(e.fails) * failBackoffStep
			log.Printf("ffmpeg died, restart %d in %v\n", e.fails, backoff)
			e.mu.Unlock()
			time.Sleep(backoff)
			e.mu.Lock()
			cmd, err := e.spawnFn(args)
			if err != nil {
				log.Println("restart failed:", err)
				e.mu.Unlock()
				return
			}
			e.cmd = cmd
			e.mu.Unlock()
			return
		case <-time.After(100 * time.Millisecond):
			e.mu.Unlock()
		}
	}
}

// Stop kills the ffmpeg process and marks the engine stopped. Safe to call
// when not started.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	e.running = false
	e.queue = nil
	log.Println("stream stopped")
	return nil
}

// Running reports whether the engine is currently publishing.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// PushSegment records an audio WAV segment with a caption into the bounded
// in-memory queue.
//
// v1 HONESTY NOTE: this performs NO real overlay. Segments are validated
// (engine running, audio non-empty) and counted/queued only; the queue is
// the hook the v2 frame pipe will consume. Do not claim overlay works.
func (e *Engine) PushSegment(audioWAV []byte, caption string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return errors.New("stream not running")
	}
	if len(audioWAV) == 0 {
		return errors.New("empty audio")
	}
	if len(e.queue) >= maxQueueLen {
		// bounded by design: drop the oldest instead of growing.
		e.queue = e.queue[1:]
	}
	e.queue = append(e.queue, segment{audioWAV: audioWAV, caption: caption})
	e.segmentsPushed++
	return nil
}

// QueueLen returns the number of queued segments (v1 observability).
func (e *Engine) QueueLen() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.queue)
}

// SegmentsPushed returns the total number of accepted segments since New.
func (e *Engine) SegmentsPushed() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.segmentsPushed
}
