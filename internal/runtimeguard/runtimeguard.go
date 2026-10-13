// Package runtimeguard supervises long-running background goroutines.
// It prevents one unexpected panic from silently killing the whole process.
package runtimeguard

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
)

// Guard owns background jobs started by the application lifecycle.
type Guard struct {
	mu   sync.Mutex
	jobs map[string]int
}

func New() *Guard {
	return &Guard{jobs: make(map[string]int)}
}

// Go starts a supervised goroutine. A panic is recovered and logged with its
// stack trace (so the cause is findable in the log of an unattended machine);
// the job is then considered finished — callers that must keep running wrap
// their own retry loop inside fn. fn is expected to return when ctx is done.
func (g *Guard) Go(ctx context.Context, name string, fn func(context.Context)) {
	g.mu.Lock()
	g.jobs[name]++
	g.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("runtimeguard: job %q panic recovered: %v\n%s", name, r, debug.Stack())
			}
			// Đợt AM: job kết thúc khi ctx chưa done = "chết lâm sàng" —
			// log warning để không chết im lặng.
			select {
			case <-ctx.Done():
			default:
				log.Printf("runtimeguard: job %q đã kết thúc trước khi app dừng — kiểm tra log phía trên xem có lỗi khởi động không", name)
			}
			g.mu.Lock()
			g.jobs[name]--
			if g.jobs[name] <= 0 {
				delete(g.jobs, name)
			}
			g.mu.Unlock()
		}()
		fn(ctx)
	}()
}

// Running returns a snapshot of the jobs still alive, keyed by name.
func (g *Guard) Running() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]int, len(g.jobs))
	for k, v := range g.jobs {
		out[k] = v
	}
	return out
}
