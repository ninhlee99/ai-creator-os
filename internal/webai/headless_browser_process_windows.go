//go:build windows

package webai

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func configureHeadlessBrowserCommand(*exec.Cmd) {}

func terminateBrowser(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	_ = interruptBrowserProcessTree(cmd)
	select {
	case <-done:
		return
	case <-time.After(1500 * time.Millisecond):
	}
	// Kill entire Windows process tree (renderer, GPU, utility children) so
	// headless browser subprocesses never stay orphaned in memory.
	_ = killBrowserProcessTree(cmd)
	<-done
}

func interruptBrowserProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Signal(os.Interrupt)
}

func killBrowserProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	if pid := cmd.Process.Pid; pid > 0 {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	return cmd.Process.Kill()
}

func cleanupOrphanedHeadlessBrowsers() {
	// Trên Windows, taskkill /F /IM msedge.exe hoặc chrome.exe có thể quét theo cờ commandline nếu cần
}
