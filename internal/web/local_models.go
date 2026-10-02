package web

import (
	"context"
	"log"
)

// Local runtime ensure (Đợt 3 / A7): local model availability takes care
// of itself. The Settings "tải model" buttons and the startup pass share
// the same kick helpers, so progress always lands in the same polled
// fields — no second, silent ensure path.

// kickVieneuEnsure starts the VieNeu model ensure in the background;
// progress is polled via /settings/vieneu/progress.
func (s *Server) kickVieneuEnsure() {
	if s.VieNeu == nil {
		return
	}
	s.vieneuMu.Lock()
	s.vieneuDownloaded, s.vieneuTotal, s.vieneuErr = 0, 0, ""
	s.vieneuDone = false
	s.vieneuMu.Unlock()
	go func() {
		err := s.VieNeu.EnsureModel(context.Background(), func(downloaded, total int64) {
			s.vieneuMu.Lock()
			s.vieneuDownloaded, s.vieneuTotal = downloaded, total
			s.vieneuMu.Unlock()
		})
		s.vieneuMu.Lock()
		s.vieneuDone = true
		if err != nil {
			s.vieneuErr = err.Error()
		}
		s.vieneuMu.Unlock()
	}()
}

// chainHasEnabled reports whether the named provider is an enabled tier
// of the chain (the same stored config the Settings chain editor writes).
func chainHasEnabled(c ChainConfig, name string) bool {
	for _, e := range c.Order {
		if e.Name == name && e.Enabled {
			return true
		}
	}
	return false
}

// EnsureLocalModels runs once at startup (Đợt 3 / A7): every enabled
// local tier whose model is not ready gets an ensure kicked in the
// background, so first use never dead-ends on a missing local model.
// VieNeu's ensure no-ops when the sidecar is already healthy, so it is
// only kicked when the sidecar is down.
// Progress lands in the same fields the Settings page polls.
func (s *Server) EnsureLocalModels(ctx context.Context) {
	if s.VieNeu != nil && chainHasEnabled(s.loadChain("tts"), "vieneu") {
		if state, _ := s.VieNeu.Status(); state != "running" && state != "downloading" {
			log.Printf("web: vieneu chưa sẵn sàng (%s) — tự ensure model nền", state)
			s.kickVieneuEnsure()
		}
	}
}

// localRuntimeView is one wired local runtime's homepage status: name +
// state (rendered as a badge); the detail rides in the tooltip.
type localRuntimeView struct {
	Name   string
	State  string
	Detail string
}

// localRuntimeViews lists every wired (non-nil) local runtime — VieNeu
// TTS. Unwired runtimes never appear, so the homepage block only ever
// states what is actually connected. (Avatar sidecar đã park — PIVOT
// 2026-10-02.)
func (s *Server) localRuntimeViews() []localRuntimeView {
	var out []localRuntimeView
	if v := s.vieneuView(); v.Connected {
		out = append(out, localRuntimeView{Name: "VieNeu TTS", State: v.State, Detail: v.Detail})
	}
	return out
}
