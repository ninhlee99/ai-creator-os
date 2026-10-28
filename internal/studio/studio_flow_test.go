package studio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if _, err := st.insertJobWithID("queued-job", KindViral, "queued job", map[string]string{}); err != nil {
		t.Fatal(err)
	}

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

func TestStartTrackedJobRejectsDeletedJob(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	run := make(chan struct{}, 1)
	err = st.startTrackedJob("deleted-job", ctx, cancel, func(context.Context) { run <- struct{}{} })
	if err == nil {
		t.Fatal("startTrackedJob accepted a deleted job")
	}
	select {
	case <-run:
		t.Fatal("runner started for a deleted job")
	default:
	}
	if ctx.Err() == nil {
		t.Fatal("startTrackedJob did not cancel rejected job context")
	}
}

func TestDeleteJobPreservesAssetsUntilRunnerStops(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.insertJobWithID("blocked-delete", KindViral, "blocked delete", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce sync.Once
	releaseRunner := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRunner()
	ctx, cancel := context.WithCancel(context.Background())
	if err := st.startTrackedJob("blocked-delete", ctx, cancel, func(context.Context) {
		close(started)
		<-release
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	<-started

	if err := st.deleteJob("blocked-delete", 10*time.Millisecond); err == nil {
		t.Fatal("DeleteJob removed assets while runner was still active")
	}
	if _, ok := st.GetJob("blocked-delete"); !ok {
		t.Fatal("DeleteJob removed job row while runner was still active")
	}
	releaseRunner()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("runner did not stop after release")
	}
	if err := st.DeleteJob("blocked-delete"); err != nil {
		t.Fatalf("DeleteJob after runner exit: %v", err)
	}
	if _, ok := st.GetJob("blocked-delete"); ok {
		t.Fatal("DeleteJob retained job row after runner exited")
	}
}

func TestDeleteJobRemovesOutputWrittenBeforeRunnerExit(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.insertJobWithID("late-output", KindViral, "late output", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	if err := st.startTrackedJob("late-output", ctx, cancel, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		<-release
		output := filepath.Join(outDir, "late.mp4")
		if err := os.WriteFile(output, []byte("rendered"), 0o600); err != nil {
			t.Error(err)
		}
		st.setOutput("late-output", filepath.Base(output))
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	<-started

	deleted := make(chan error, 1)
	go func() { deleted <- st.deleteJob("late-output", time.Second) }()
	<-ctx.Done()
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("runner did not finish")
	}
	if err := <-deleted; err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "late.mp4")); !os.IsNotExist(err) {
		t.Fatalf("late output remains after job deletion: stat err=%v", err)
	}
}

func TestDetachedFlowPanicMarksRunFailed(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetFlowStore(); err != nil {
		t.Fatal(err)
	}

	const nodeType = "test_detached_flow_panic_executor"
	flow.RegisterNode(flow.NodeDefinition{
		Type: nodeType,
		Executor: func(*flow.ExecutionContext, *flow.FlowNode, map[string]any) (map[string]any, error) {
			panic("expected test panic")
		},
	})
	graph := &flow.FlowGraph{
		ID: "panic-graph", Name: "panic graph",
		Nodes: []flow.FlowNode{{ID: "panic", Type: nodeType}},
	}
	if err := st.StartFlowGraphWithInputs(context.Background(), "panic-run", graph, nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		run, err := st.GetFlowStore()
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := run.GetRun("panic-run")
		if err == nil && persisted.Status == "failed" {
			if !strings.Contains(persisted.Log, "expected test panic") {
				t.Fatalf("panic detail missing from run log: %q", persisted.Log)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached flow panic was not persisted as failed")
}

func TestSynchronousFlowPanicReturnsFailedRun(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const nodeType = "test_synchronous_flow_panic_executor"
	flow.RegisterNode(flow.NodeDefinition{
		Type: nodeType,
		Executor: func(*flow.ExecutionContext, *flow.FlowNode, map[string]any) (map[string]any, error) {
			panic("expected synchronous panic")
		},
	})
	graph := &flow.FlowGraph{
		ID: "sync-panic-graph", Name: "sync panic graph",
		Nodes: []flow.FlowNode{{ID: "panic", Type: nodeType}},
	}
	run, err := st.ExecuteFlowGraph(context.Background(), "sync-panic-run", graph)
	if err == nil || run == nil || run.Status != "failed" {
		t.Fatalf("ExecuteFlowGraph = (%+v, %v), want failed run and panic error", run, err)
	}
	if !strings.Contains(run.Log, "expected synchronous panic") {
		t.Fatalf("panic detail missing from workflow log: %q", run.Log)
	}
}
