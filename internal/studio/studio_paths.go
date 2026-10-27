package studio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s *Studio) Close() error { return s.db.Close() }

// UploadDir is where the web layer saves uploaded model/product photos and
// music files.
func (s *Studio) UploadDir() string { return filepath.Join(s.workRoot, "uploads") }

// ModelLibrary returns the per-account model photo library (Ninh's uploads).
func (s *Studio) ModelLibrary() (*ModelStore, error) {
	return NewModelStore(s.db, s.workRoot)
}

// DefaultModelPhoto returns first available uploaded model image for automated
// affiliate jobs that do not target a specific channel account.
func (s *Studio) DefaultModelPhoto() string {
	photos := s.uploadedModelPhotos(0)
	if len(photos) == 0 {
		return ""
	}
	return photos[0]
}

// WorkDir is the per-job work root (storyboard assets live under it).
func (s *Studio) WorkDir() string { return s.workRoot }

// MediaGen exposes the provider (dashboard health card).
func (s *Studio) MediaGen() MediaGen { return s.mg }

// MuseVideoReady reports whether the real Muse browser session can render.
func (s *Studio) MuseVideoReady(ctx context.Context) (bool, string) {
	cliPath := filepath.Join("data", "third_party", "Muse-Chat-MCP", "muse-cli.mjs")
	serverPath := filepath.Join("data", "third_party", "Muse-Chat-MCP", "muse-server.mjs")
	if !fileExists(cliPath) || !fileExists(serverPath) {
		return false, "Muse-Chat-MCP chưa được cài đầy đủ"
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return false, "chưa cài Node.js để chạy Muse-Chat-MCP"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, node, serverPath, "--self-test").CombinedOutput()
	status := string(output)
	if err != nil {
		return false, "không kiểm tra được phiên Muse AI"
	}
	if !strings.Contains(status, `"loggedIn":true`) || !strings.Contains(status, `"composerReady":true`) {
		return false, "Muse AI chưa đăng nhập hoặc composer chưa sẵn sàng"
	}
	return true, "Muse AI đã đăng nhập và sẵn sàng tạo video"
}

// ViralCharacterEditorStatus reports whether Muse rendering and
// the required visual QC analyzer are ready.
func (s *Studio) ViralCharacterEditorStatus(ctx context.Context) (bool, string) {
	if ready, reason := s.MuseVideoReady(ctx); !ready {
		return false, reason
	}
	if s.mg == nil || !s.mg.Healthy(ctx) {
		return false, "media engine chưa sẵn sàng để đối chiếu nhân vật, cảnh và chuyển động"
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok {
		return false, "media engine chưa hỗ trợ visual QC video viral"
	}
	return true, "Muse AI và visual QC sẵn sàng"
}

// OutputPath resolves a job's output filename to its full filesystem path.
func (s *Studio) OutputPath(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		return name
	}
	if s.outDir != "" {
		cand := filepath.Join(s.outDir, name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if _, err := os.Stat(name); err == nil {
		return name
	}
	if s.outDir != "" {
		return filepath.Join(s.outDir, name)
	}
	return name
}

// ---------------------------------------------------------------------------
// job bookkeeping
// ---------------------------------------------------------------------------
