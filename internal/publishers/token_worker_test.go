package publishers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenWorkerInspect(t *testing.T) {
	tmp := t.TempDir()
	old := tokenDir
	tokenDir = tmp
	defer func() { tokenDir = old }()

	// Tạo 1 file token youtube hợp lệ
	ytPath := filepath.Join(tmp, "youtube_token_testuser.json")
	_ = os.WriteFile(ytPath, []byte(`{"refresh_token":"fake-refresh","access_token":"fake-access"}`), 0o600)

	// Tạo 1 file token tiktok hợp lệ
	ttPath := filepath.Join(tmp, "tiktok_token_testuser.json")
	_ = os.WriteFile(ttPath, []byte(`{"refresh_token":"tt-refresh","access_token":"tt-access"}`), 0o600)

	reports := InspectTokens()
	if len(reports) != 2 {
		t.Fatalf("want 2 reports, got %d", len(reports))
	}

	for _, r := range reports {
		if r.Status != "healthy" {
			t.Errorf("token %s want healthy, got %s", r.File, r.Status)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartTokenWorker(ctx, 100*time.Millisecond)
	latest := GetLatestTokenHealth()
	if len(latest) != 2 {
		t.Fatalf("want 2 latest reports, got %d", len(latest))
	}
}
