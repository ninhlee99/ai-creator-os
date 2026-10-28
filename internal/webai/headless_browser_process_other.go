//go:build !unix && !windows

package webai

import (
	"os"
	"os/exec"
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
	case <-time.After(3 * time.Second):
		_ = killBrowserProcessTree(cmd)
		<-done
	}
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
	return cmd.Process.Kill()
}
