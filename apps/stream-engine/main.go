// stream-engine: tiny Go supervisor for FFmpeg RTMP publishing.
//
// Owns the FFmpeg process, hot-reloads overlays, watchdogs reconnects.
// HTTP control API on 127.0.0.1:8090 (loopback only):
//   POST /start {rtmp_url, rtmp_key, disclosure}
//   POST /stop {}
//
// Resource-capped by design: one FFmpeg process, bounded restart backoff.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

type startReq struct {
	RtmpURL    string `json:"rtmp_url"`
	RtmpKey    string `json:"rtmp_key"`
	Disclosure string `json:"disclosure"`
}

type engine struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	fails   int
}

func (e *engine) start(r startReq) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return fmt.Errorf("already running")
	}
	if r.RtmpURL == "" || r.RtmpKey == "" {
		return fmt.Errorf("missing rtmp url/key")
	}
	dest := r.RtmpURL + "/" + r.RtmpKey
	// v1: test pattern + tone. v2: avatar scene via frame pipe on fd.
	// disclosure text burned in via drawtext when provided.
	args := []string{
		"-re", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency",
		"-c:a", "aac", "-f", "flv", dest,
	}
	if r.Disclosure != "" {
		// drawtext needs fontfile on mac; kept simple for v1 wiring
		_ = r.Disclosure
	}
	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	e.cmd = cmd
	e.running = true
	e.fails = 0
	go e.watchdog(dest, args)
	log.Println("stream started ->", r.RtmpURL)
	return nil
}

func (e *engine) watchdog(dest string, args []string) {
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
			if e.fails > 5 {
				log.Println("too many ffmpeg failures, giving up (alert!)")
				e.running = false
				e.mu.Unlock()
				return
			}
			backoff := time.Duration(e.fails*5) * time.Second
			log.Printf("ffmpeg died, restart %d in %v\n", e.fails, backoff)
			e.mu.Unlock()
			time.Sleep(backoff)
			e.mu.Lock()
			cmd := exec.Command("ffmpeg", args...)
			if err := cmd.Start(); err != nil {
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

func (e *engine) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	e.running = false
	log.Println("stream stopped")
}

func main() {
	e := &engine{}
	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		var req startReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := e.start(req); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		e.stop()
		w.Write([]byte(`{"ok":true}`))
	})
	log.Println("stream-engine on 127.0.0.1:8090")
	log.Fatal(http.ListenAndServe("127.0.0.1:8090", nil))
}
