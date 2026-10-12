// Package runtimeguard supervises long-running background goroutines.
// It prevents one unexpected panic from silently killing a worker.
package runtimeguard

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Guard owns background jobs started by the application lifecycle.
type Guard struct {
	mu      sync.Mutex
	jobs    map[string]int
}

func New() *Guard {
	return &Guard{jobs: make(map[string]int)}
}

// Go starts a supervised goroutine. Panics are recovered and logged.
// The goroutine is expected to stop when ctx is cancelled.
func (g *Guard) Go(ctx context.Context, name string, fn func(context.Context)) {
	g.mu.Lock()
	g.jobs[name]++
	g.mu.Unlock()

	go func() {
		defer func() {
			g.mu.Lock()
			g.jobs[name]--
			if g.jobs[name] <= 0 {
				delete(g.jobs, name)
			}
			g.mu.Unlock()

			if r := recover(); r != nil {
				log.Printf("runtimeguard: job %q panic recovered: %v", name, r)
			}
		}()

		defer func() {
			if r := recover(); r != nil {
				log.Printf("runtimeguard: job %q panic: %v", name, fmt.Sprint(r))
			}
		}()

		fn(ctx)
	}()
}

func (g *Guard) Running() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]int, len(g.jobs))
	for k, v := range g.jobs {
		out[k] = v
	}
	return out
}
