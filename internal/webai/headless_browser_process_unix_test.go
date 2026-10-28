//go:build unix

package webai

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTerminateBrowserKillsEntireProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 60 & echo $!; wait")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	configureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { terminateBrowser(cmd) })

	childText, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(childText))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", childText, err)
	}
	if !browserProcessGroupExists(cmd.Process.Pid) {
		t.Fatal("browser process group disappeared before termination")
	}
	cancel()
	terminateBrowser(cmd)

	deadline := time.Now().Add(2 * time.Second)
	for browserProcessGroupExists(cmd.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if browserProcessGroupExists(cmd.Process.Pid) {
		t.Fatalf("browser process group %d still exists after termination (child %d)", cmd.Process.Pid, childPID)
	}
}
