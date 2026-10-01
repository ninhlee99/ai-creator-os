package local

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Process states.
const (
	StateStopped = "stopped"
	StateRunning = "running"
	StateError   = "error"
)

const maxOutLines = 200

// Process is a supervised long-running child process: llama-server, the
// VieNeu TTS sidecar, whisper.cpp later.
//
//   - Start launches it (idempotent while running).
//   - Stop terminates it gracefully (SIGTERM, then SIGKILL after 5s).
//   - On unexpected exit it restarts with increasing backoff
//     (BackoffBase, doubling, capped at 1 minute); after MaxRestarts
//     consecutive crashes (default 5) it gives up and reports StateError.
//   - When HealthURL is set, a health check runs every HealthInterval
//     (default 30s); the last result is visible via LastHealthError.
//   - Recent stdout/stderr lines are kept in a ring buffer (RecentOutput)
//     so callers can parse startup/download progress best-effort.
type Process struct {
	Name string
	Bin  string
	Args []string
	Dir  string
	// Env holds extra environment variables merged over os.Environ().
	Env map[string]string

	HealthURL      string
	HealthInterval time.Duration
	MaxRestarts    int
	BackoffBase    time.Duration

	mu          sync.Mutex
	cmd         *exec.Cmd
	state       string
	stopping    bool
	stopCh      chan struct{}
	monitorDone chan struct{}
	healthStop  chan struct{}
	startedAt   time.Time
	restarts    int
	lastErr     error
	lastHealth  error

	outMu    sync.Mutex
	outLines []string
}

func (p *Process) maxRestarts() int {
	if p.MaxRestarts <= 0 {
		return 5
	}
	return p.MaxRestarts
}

func (p *Process) backoffBase() time.Duration {
	if p.BackoffBase <= 0 {
		return 2 * time.Second
	}
	return p.BackoffBase
}

func (p *Process) healthInterval() time.Duration {
	if p.HealthInterval <= 0 {
		return 30 * time.Second
	}
	return p.HealthInterval
}

// Start launches the process. It is idempotent while running; a previous
// error state is cleared and the restart counter reset.
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return nil
	}
	if _, err := exec.LookPath(p.Bin); err != nil {
		return fmt.Errorf("%s: binary %q not found in PATH: %w", p.Name, p.Bin, err)
	}
	p.stopping = false
	p.stopCh = make(chan struct{})
	p.monitorDone = make(chan struct{})
	p.healthStop = make(chan struct{})
	p.restarts = 0
	p.lastErr = nil
	p.lastHealth = nil
	if err := p.spawnLocked(); err != nil {
		return err
	}
	p.state = StateRunning
	p.startedAt = time.Now()
	go p.supervise()
	if p.HealthURL != "" {
		go p.healthLoop()
	}
	return nil
}

// spawnLocked creates and starts the OS process. Caller holds p.mu.
func (p *Process) spawnLocked() error {
	cmd := exec.Command(p.Bin, p.Args...)
	if p.Dir != "" {
		cmd.Dir = p.Dir
	}
	if len(p.Env) > 0 {
		env := os.Environ()
		for k, v := range p.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start: %w", p.Name, err)
	}
	p.cmd = cmd
	go p.drain(stdout)
	go p.drain(stderr)
	return nil
}

func (p *Process) drain(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		p.appendOut(sc.Text())
	}
}

func (p *Process) appendOut(line string) {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	p.outLines = append(p.outLines, line)
	if len(p.outLines) > maxOutLines {
		p.outLines = p.outLines[len(p.outLines)-maxOutLines:]
	}
}

// RecentOutput returns the last captured stdout/stderr lines (oldest first).
func (p *Process) RecentOutput() []string {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	out := make([]string, len(p.outLines))
	copy(out, p.outLines)
	return out
}

// supervise waits for the process and restarts it on unexpected exit.
func (p *Process) supervise() {
	defer close(p.monitorDone)
	for {
		err := p.cmd.Wait()
		p.mu.Lock()
		if p.stopping {
			p.state = StateStopped
			p.cmd = nil
			p.mu.Unlock()
			return
		}
		p.restarts++
		n := p.restarts
		if n > p.maxRestarts() {
			p.state = StateError
			p.lastErr = fmt.Errorf("%s exited (%v); gave up after %d restarts", p.Name, err, p.maxRestarts())
			p.cmd = nil
			p.mu.Unlock()
			return
		}
		delay := p.backoffBase() * time.Duration(1<<(n-1))
		if delay <= 0 || delay > time.Minute {
			delay = time.Minute
		}
		p.lastErr = fmt.Errorf("%s exited (%v); restart %d/%d in %s", p.Name, err, n, p.maxRestarts(), delay)
		stopCh := p.stopCh
		p.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-stopCh:
			timer.Stop()
			p.mu.Lock()
			p.state = StateStopped
			p.cmd = nil
			p.mu.Unlock()
			return
		}

		p.mu.Lock()
		if p.stopping {
			p.state = StateStopped
			p.cmd = nil
			p.mu.Unlock()
			return
		}
		if err := p.spawnLocked(); err != nil {
			p.state = StateError
			p.lastErr = fmt.Errorf("%s: respawn failed: %w", p.Name, err)
			p.cmd = nil
			p.mu.Unlock()
			return
		}
		p.state = StateRunning
		p.startedAt = time.Now()
		p.mu.Unlock()
	}
}

// Stop terminates the process: SIGTERM, then SIGKILL after a 5s grace period.
// It also cancels a pending restart backoff.
func (p *Process) Stop() error {
	p.mu.Lock()
	if p.cmd == nil {
		p.mu.Unlock()
		return nil
	}
	if p.stopping {
		done := p.monitorDone
		p.mu.Unlock()
		<-done
		return nil
	}
	p.stopping = true
	cmd := p.cmd
	stopCh := p.stopCh
	hs := p.healthStop
	p.healthStop = nil
	done := p.monitorDone
	p.mu.Unlock()

	close(stopCh)
	if hs != nil {
		close(hs)
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	return nil
}

// Restart is Stop followed by Start.
func (p *Process) Restart(ctx context.Context) error {
	if err := p.Stop(); err != nil {
		return err
	}
	return p.Start(ctx)
}

// Running reports whether the process is currently up.
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && !p.stopping
}

// State returns "stopped", "running" or "error".
func (p *Process) State() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == "" {
		return StateStopped
	}
	return p.state
}

// Uptime returns how long the current incarnation has been running.
func (p *Process) Uptime() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.startedAt.IsZero() {
		return 0
	}
	return time.Since(p.startedAt)
}

// RestartCount returns the number of crash restarts so far.
func (p *Process) RestartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restarts
}

// LastError returns the last recorded process error, if any.
func (p *Process) LastError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

// LastHealthError returns the last health-check result (nil = healthy).
func (p *Process) LastHealthError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastHealth
}

func (p *Process) healthLoop() {
	t := time.NewTicker(p.healthInterval())
	defer t.Stop()
	for {
		select {
		case <-p.healthStop:
			return
		case <-t.C:
			p.checkHealth()
		}
	}
}

func (p *Process) checkHealth() {
	p.mu.Lock()
	running := p.cmd != nil && !p.stopping
	url := p.HealthURL
	p.mu.Unlock()
	if !running || url == "" {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	var herr error
	switch {
	case err != nil:
		herr = err
	case resp.StatusCode != http.StatusOK:
		herr = fmt.Errorf("health %s: HTTP %d", url, resp.StatusCode)
	}
	if resp != nil {
		resp.Body.Close()
	}
	p.mu.Lock()
	p.lastHealth = herr
	p.mu.Unlock()
}

// WaitHealthy polls HealthURL until it returns 200, ctx is cancelled, or
// timeout elapses.
func (p *Process) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	if p.HealthURL == "" {
		return fmt.Errorf("%s: no health URL configured", p.Name)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.HealthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: not healthy after %s: %v", p.Name, timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
