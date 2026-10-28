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

func TestTerminateBrowserAfterWaitKillsTreeWithoutSecondWait(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 60 & wait")
	configureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	processDone := make(chan error, 1)
	go func() {
		processDone <- cmd.Wait()
		close(processDone)
	}()
	started := time.Now()
	terminateBrowserAfterWait(cmd, processDone)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("process tree shutdown took %s; expected graceful group signal without a second Wait", elapsed)
	}
	select {
	case <-processDone:
	case <-time.After(time.Second):
		t.Fatal("original Wait goroutine did not finish after process tree shutdown")
	}
}
