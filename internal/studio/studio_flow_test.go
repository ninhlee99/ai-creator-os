package studio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

func TestExecuteFlowGraphRejectsNilGraph(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.ExecuteFlowGraph(context.Background(), "nil-graph", nil); err == nil {
		t.Fatal("ExecuteFlowGraph accepted nil graph")
	}
}

func TestExecuteFlowGraphReturnsWorkDirectoryError(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetFlowStore(); err != nil {
		t.Fatal(err)
	}

	blockedRoot := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	st.workRoot = blockedRoot
	_, err = st.ExecuteFlowGraph(context.Background(), "workdir-error", &flow.FlowGraph{ID: "g", Name: "graph"})
	if err == nil || !strings.Contains(err.Error(), "tạo thư mục workflow") {
		t.Fatalf("ExecuteFlowGraph error = %v, want work directory error", err)
	}
}

func TestExecuteFlowGraphReturnsInitialRunPersistenceError(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetFlowStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DROP TABLE studio_workflow_runs`); err != nil {
		t.Fatal(err)
	}

	_, err = st.ExecuteFlowGraph(context.Background(), "persist-error", &flow.FlowGraph{ID: "g", Name: "graph"})
	if err == nil || !strings.Contains(err.Error(), "lưu trạng thái khởi tạo workflow") {
		t.Fatalf("ExecuteFlowGraph error = %v, want initial persistence error", err)
	}
}

func TestShutdownCancelsDetachedFlowAndRejectsNewFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st := &Studio{
		flowRuns: map[string]context.CancelFunc{"active-flow": cancel},
	}

	st.Shutdown(0)
	if ctx.Err() == nil {
		t.Fatal("Shutdown did not cancel active flow")
	}
	if err := st.StartFlowGraphWithInputs(context.Background(), "after-shutdown", &flow.FlowGraph{ID: "g"}, nil); err == nil {
		t.Fatal("StartFlowGraphWithInputs accepted a run after shutdown began")
	}
}

func TestShutdownTracksJobBeforeRunnerStarts(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	if err := st.startTrackedJob("queued-job", ctx, cancel, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	st.mu.Lock()
	_, tracked := st.running["queued-job"]
	st.mu.Unlock()
	if !tracked {
		t.Fatal("accepted job was not tracked while running")
	}

	st.Shutdown(0)
	if ctx.Err() == nil {
		t.Fatal("Shutdown did not cancel tracked job")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("job runner did not stop after cancellation")
	}
}
