//go:build unix

package webai

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureHeadlessBrowserCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := signalBrowserProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
		if err == nil {
			return nil
		}
		return err
	}
}

func terminateBrowser(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	processGroupID := cmd.Process.Pid
	_ = interruptBrowserProcessTree(cmd)
	deadline := time.Now().Add(3 * time.Second)
	for browserProcessGroupExists(processGroupID) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if browserProcessGroupExists(processGroupID) {
		_ = killBrowserProcessTree(cmd)
	}
	<-done
}

func interruptBrowserProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return signalBrowserProcessGroup(cmd.Process.Pid, syscall.SIGINT)
}

func killBrowserProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := signalBrowserProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
	return err
}

func signalBrowserProcessGroup(processGroupID int, signal syscall.Signal) error {
	err := syscall.Kill(-processGroupID, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func browserProcessGroupExists(processGroupID int) bool {
	err := syscall.Kill(-processGroupID, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
