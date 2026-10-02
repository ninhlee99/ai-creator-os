//go:build parked

package avatar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/local"
)

// sidecarLifecycle manages the avatar sidecar process: model download,
// start/stop/restart and health. It mirrors the VieNeu sidecar pattern
// with two deliberate differences:
//
//   - NO Docker: the sidecar is a plain native binary launched directly
//     (the owner starts aicos itself; the app never depends on Docker).
//   - The model URL is explicit configuration (AVATAR_MODEL_URL), never a
//     guessed default: the weights archive depends on which third-party
//     server build the owner installs on their Mac.
//
// Default layout under <data>:
//
//	third_party/avatar-sidecar/avatar-server   (the HTTP server binary)
//	models/avatar/                             (weights, from AVATAR_MODEL_URL)
type sidecarLifecycle struct {
	mu      sync.Mutex
	dataDir string
	baseURL string
	reg     *local.Registry
	proc    *local.Process
}

// newSidecarLifecycle builds the lifecycle manager for dataDir.
func newSidecarLifecycle(dataDir, baseURL string) *sidecarLifecycle {
	return &sidecarLifecycle{
		dataDir: dataDir,
		baseURL: baseURL,
		reg:     local.NewRegistry(dataDir),
	}
}

// sidecarDir is where the server binary lives.
func (l *sidecarLifecycle) sidecarDir() string {
	if d := strings.TrimSpace(os.Getenv("AVATAR_SIDECAR_DIR")); d != "" {
		return d
	}
	return filepath.Join(l.reg.ThirdPartyDir(), "avatar-sidecar")
}

// modelDir holds the downloaded weights.
func (l *sidecarLifecycle) modelDir() string {
	return filepath.Join(l.reg.ModelsDir(), "avatar")
}

// outputDir receives finished mp4s.
func (l *sidecarLifecycle) outputDir() string {
	return filepath.Join(l.dataDir, "output")
}

// modelURL is the weights archive URL. Empty = not configured: EnsureModel
// fails honestly instead of downloading from a guessed address.
func (l *sidecarLifecycle) modelURL() string {
	return strings.TrimSpace(os.Getenv("AVATAR_MODEL_URL"))
}

// ModelConfigured reports whether a model URL is set (dashboard display).
func (l *sidecarLifecycle) ModelConfigured() bool { return l.modelURL() != "" }

// ModelPresent reports whether the weights directory is non-empty.
func (l *sidecarLifecycle) ModelPresent() bool {
	entries, err := os.ReadDir(l.modelDir())
	return err == nil && len(entries) > 0
}

// EnsureModel downloads the weights archive for the dashboard "tải model"
// button. It refuses to guess a URL: without AVATAR_MODEL_URL it returns a
// clear configuration error.
func (l *sidecarLifecycle) EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error {
	if l.ModelPresent() {
		return nil
	}
	url := l.modelURL()
	if url == "" {
		return fmt.Errorf("chưa cấu hình AVATAR_MODEL_URL — trỏ nó tới file weights của avatar sidecar (ví dụ bản build MuseTalk v1.5 cho Apple Silicon), rồi thử lại")
	}
	if err := os.MkdirAll(l.modelDir(), 0o755); err != nil {
		return err
	}
	name := "avatar-weights" + archiveExt(url)
	// 1GB floor: real talking-head weights are several GB; anything
	// smaller is a corrupt/HTML download — fail loudly.
	if err := l.reg.Download(ctx, filepath.Join("avatar", name), url, 1<<30, onProgress); err != nil {
		return fmt.Errorf("tải model avatar: %w", err)
	}
	return nil
}

func archiveExt(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, ".tar.gz"):
		return ".tar.gz"
	case strings.Contains(lower, ".zip"):
		return ".zip"
	default:
		return ".bin"
	}
}

// serverBin resolves the sidecar binary: AVATAR_SIDECAR_CMD wins, otherwise
// <sidecarDir>/avatar-server.
func (l *sidecarLifecycle) serverBin() string {
	if cmd := strings.TrimSpace(os.Getenv("AVATAR_SIDECAR_CMD")); cmd != "" {
		return cmd
	}
	return filepath.Join(l.sidecarDir(), "avatar-server")
}

// Start launches the sidecar and waits until /health is ok. It does NOT
// auto-download the model: call EnsureModel first (the dashboard guides
// the owner through it).
func (l *sidecarLifecycle) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.proc != nil && l.proc.Running() {
		l.mu.Unlock()
		return nil
	}
	bin := l.serverBin()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("không tìm thấy avatar sidecar tại %s — cài sidecar (xem docs/RESEARCH.md §6) hoặc đặt AVATAR_SIDECAR_CMD", bin)
	}
	if !l.ModelPresent() {
		return fmt.Errorf("chưa có model avatar tại %s — bấm \"Tải model\" trong Settings trước", l.modelDir())
	}
	proc := &local.Process{
		Name:      "avatar-sidecar",
		Bin:       bin,
		Args:      []string{"--host", "127.0.0.1", "--port", "18080", "--model-dir", l.modelDir()},
		Dir:       l.sidecarDir(),
		HealthURL: l.baseURL + "/health",
		Env: map[string]string{
			// MuseTalk-class servers built on torch need the MPS fallback
			// flag on Apple Silicon.
			"PYTORCH_ENABLE_MPS_FALLBACK": "1",
		},
	}
	if err := proc.Start(ctx); err != nil {
		return fmt.Errorf("khởi động avatar sidecar: %w", err)
	}
	l.proc = proc
	// WaitHealthy polls over HTTP; run it without holding the lock.
	l.mu.Unlock()
	return proc.WaitHealthy(ctx, 5*time.Minute)
}

// Stop terminates the sidecar.
func (l *sidecarLifecycle) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.proc == nil {
		return nil
	}
	return l.proc.Stop()
}

// Restart is Stop followed by Start.
func (l *sidecarLifecycle) Restart(ctx context.Context) error {
	if err := l.Stop(); err != nil {
		return err
	}
	return l.Start(ctx)
}

// Status returns a short state + detail for the dashboard.
func (l *sidecarLifecycle) Status() (state, detail string) {
	l.mu.Lock()
	proc := l.proc
	l.mu.Unlock()
	if proc != nil && proc.Running() {
		return "running", fmt.Sprintf("sidecar đang chạy tại %s (uptime %s)", l.baseURL, proc.Uptime().Round(time.Second))
	}
	if !l.ModelPresent() {
		if !l.ModelConfigured() {
			return "missing", "chưa cấu hình AVATAR_MODEL_URL và chưa tải model"
		}
		return "missing", "chưa tải model — bấm \"Tải model\" trong Settings"
	}
	return "stopped", "sidecar chưa chạy — bấm Start để khởi động"
}
