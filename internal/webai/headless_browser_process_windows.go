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
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
		return
	case <-time.After(1500 * time.Millisecond):
	}
	// Kill entire Windows process tree (renderer, GPU, utility children) so
	// headless browser subprocesses never stay orphaned in memory.
	if pid := cmd.Process.Pid; pid > 0 {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	_ = cmd.Process.Kill()
	<-done
}
