//go:build unix

package webai

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func findChildPIDs(parentPID int) []int {
	if parentPID <= 0 {
		return nil
	}
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return nil
	}
	var pids []int
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if pid, err := strconv.Atoi(l); err == nil && pid > 0 {
			pids = append(pids, pid)
			pids = append(pids, findChildPIDs(pid)...)
		}
	}
	return pids
}

func terminateBrowser(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	children := findChildPIDs(pid)

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	processGroupID := pid
	_ = interruptBrowserProcessTree(cmd)
	for _, c := range children {
		_ = syscall.Kill(c, syscall.SIGTERM)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for browserProcessGroupExists(processGroupID) && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}

	// Luôn đảm bảo gửi SIGKILL dứt điểm cho cả process group, process chính và các tiến trình con
	_ = killBrowserProcessTree(cmd)
	for _, c := range children {
		_ = syscall.Kill(c, syscall.SIGKILL)
	}
	for _, c := range findChildPIDs(pid) {
		_ = syscall.Kill(c, syscall.SIGKILL)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
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

func cleanupOrphanedHeadlessBrowsers() {
	out, err := exec.Command("ps", "-eo", "pid,ppid,command").Output()
	if err != nil {
		return
	}
	currentPID := os.Getpid()
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if (strings.Contains(line, "aicos-tt-crawler") || (strings.Contains(line, "aicos-") && strings.Contains(line, "headless"))) &&
			(strings.Contains(lower, "edge") || strings.Contains(lower, "chrome") || strings.Contains(lower, "chromium") || strings.Contains(lower, "msedge")) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				pid, err1 := strconv.Atoi(parts[0])
				ppid, err2 := strconv.Atoi(parts[1])
				if err1 == nil && err2 == nil && pid > 0 {
					if ppid == 1 || (ppid != currentPID && ppid != os.Getppid()) {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			}
		}
	}
}
