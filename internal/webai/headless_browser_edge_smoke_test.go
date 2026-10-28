package webai

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestEdgeHeadlessIsolationSmoke is an opt-in host integration check:
// AICOS_TEST_EDGE_HEADLESS=1 go test ./internal/webai -run TestEdgeHeadlessIsolationSmoke
func TestEdgeHeadlessIsolationSmoke(t *testing.T) {
	if os.Getenv("AICOS_TEST_EDGE_HEADLESS") != "1" {
		t.Skip("set AICOS_TEST_EDGE_HEADLESS=1 to launch installed Edge/Chromium")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("this smoke test targets the macOS Edge launch path")
	}
	binary, err := findChromiumBinary()
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, optimizedHeadlessBrowserArgs(0, profile, "800,600")...)
	closeOutput, err := ConfigureHeadlessBrowserOutput(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer closeOutput()
	ConfigureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start headless browser: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		terminateBrowser(cmd)
		if browserProcessGroupExists(pid) {
			t.Errorf("headless process group %d survived cleanup", pid)
		}
	}()
	if _, err := waitDebuggerURL(ctx, 0, profile); err != nil {
		t.Fatalf("headless browser did not publish CDP endpoint: %v", err)
	}
}
