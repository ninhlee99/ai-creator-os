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

	// Tạo 2 file token tiktok hợp lệ
	tt1Path := filepath.Join(tmp, "tiktok_token_user1.json")
	_ = os.WriteFile(tt1Path, []byte(`{"refresh_token":"tt1-refresh","access_token":"tt1-access"}`), 0o600)

	tt2Path := filepath.Join(tmp, "tiktok_token_user2.json")
	_ = os.WriteFile(tt2Path, []byte(`{"refresh_token":"tt2-refresh","access_token":"tt2-access"}`), 0o600)

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
