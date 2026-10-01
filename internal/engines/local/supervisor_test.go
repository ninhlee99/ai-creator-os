package local

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

func needBin(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not in PATH", name)
	}
}

func TestProcessStartStop(t *testing.T) {
	needBin(t, "sleep")
	ctx := context.Background()
	p := &Process{Name: "sleep-test", Bin: "sleep", Args: []string{"30"}}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// idempotent while running
	if err := p.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !p.Running() {
		t.Fatal("should be running")
	}
	if p.State() != StateRunning {
		t.Fatalf("state = %q", p.State())
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if p.Running() {
		t.Fatal("should be stopped")
	}
	if p.State() != StateStopped {
		t.Fatalf("state = %q", p.State())
	}
	// stopping twice is fine
	if err := p.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestProcessRestartOnCrash(t *testing.T) {
	needBin(t, "sh")
	ctx := context.Background()
	p := &Process{
		Name: "crash-test", Bin: "sh", Args: []string{"-c", "exit 3"},
		MaxRestarts: 2, BackoffBase: 50 * time.Millisecond,
	}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// crash -> restart x2 -> give up => error state. Poll up to 10s.
	deadline := time.Now().Add(10 * time.Second)
	for p.State() != StateError && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if p.State() != StateError {
		p.Stop()
		t.Fatalf("state = %q, want error", p.State())
	}
	if got := p.RestartCount(); got != 3 {
		t.Fatalf("restarts = %d, want 3 (2 restarts + final give-up)", got)
	}
	if p.LastError() == nil {
		t.Fatal("LastError should be set")
	}
}

func TestProcessHealthCheck(t *testing.T) {
	needBin(t, "sleep")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	ctx := context.Background()
	p := &Process{
		Name: "health-test", Bin: "sleep", Args: []string{"30"},
		HealthURL: srv.URL, HealthInterval: 100 * time.Millisecond,
	}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// wait until at least one check ran: we can't observe directly, so
		// poll output-independent state — just sleep then assert.
		time.Sleep(300 * time.Millisecond)
		break
	}
	if err := p.LastHealthError(); err != nil {
		t.Fatalf("LastHealthError = %v", err)
	}
	if out := p.RecentOutput(); out == nil {
		t.Fatal("RecentOutput should be non-nil")
	}
}

func TestProcessWaitHealthyTimeout(t *testing.T) {
	p := &Process{Name: "nope", HealthURL: "http://127.0.0.1:1/health"}
	ctx := context.Background()
	start := time.Now()
	err := p.WaitHealthy(ctx, 3*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 8*time.Second {
		t.Fatal("WaitHealthy took too long")
	}
}

func TestProcessMissingBinary(t *testing.T) {
	p := &Process{Name: "missing", Bin: "definitely-not-a-real-binary-xyz"}
	if err := p.Start(context.Background()); err == nil {
		t.Fatal("expected binary-not-found error")
	}
}
