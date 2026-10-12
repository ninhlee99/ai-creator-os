package runtimeguard

import (
	"context"
	"testing"
	"time"
)

func waitGone(t *testing.T, g *Guard, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := g.Running()[name]; !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %q still running: %v", name, g.Running())
}

func TestGoRecoversPanic(t *testing.T) {
	g := New()
	g.Go(context.Background(), "boom", func(context.Context) { panic("x") })
	waitGone(t, g, "boom") // process still alive and bookkeeping cleaned up
}

func TestGoTracksUntilCancel(t *testing.T) {
	g := New()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	g.Go(ctx, "loop", func(c context.Context) {
		close(started)
		<-c.Done()
	})
	<-started
	if n := g.Running()["loop"]; n != 1 {
		t.Fatalf("Running[loop] = %d, want 1", n)
	}
	cancel()
	waitGone(t, g, "loop")
}
