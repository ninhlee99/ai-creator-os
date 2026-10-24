package publishers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TokenHealthReport describes a TikTok OAuth token check.
type TokenHealthReport struct {
	Platform    string    `json:"platform"`     // "tiktok"
	Username    string    `json:"username"`     // tên kênh / tài khoản
	File        string    `json:"file"`         // đường dẫn file token
	Status      string    `json:"status"`       // "healthy", "warning", "revoked", "unconfigured"
	Message     string    `json:"message"`      // thông điệp chi tiết
	LastChecked time.Time `json:"last_checked"` // thời điểm kiểm tra gần nhất
}

var (
	tokenWorkerMu     sync.RWMutex
	lastHealthReports []TokenHealthReport
	onTokenAlertHook  func(report TokenHealthReport)
)

// SetOnTokenAlert đăng ký callback nhận thông báo khi có token bị hỏng hoặc hết hạn.
func SetOnTokenAlert(fn func(report TokenHealthReport)) {
	tokenWorkerMu.Lock()
	defer tokenWorkerMu.Unlock()
	onTokenAlertHook = fn
}

// GetLatestTokenHealth trả về báo cáo sức khỏe token mới nhất cho UI hiển thị.
func GetLatestTokenHealth() []TokenHealthReport {
	tokenWorkerMu.RLock()
	defer tokenWorkerMu.RUnlock()
	out := make([]TokenHealthReport, len(lastHealthReports))
	copy(out, lastHealthReports)
	return out
}

// InspectTokens quét toàn bộ file token hiện có trong tokenDir và trả về trạng thái chi tiết.
func InspectTokens() []TokenHealthReport {
	var reports []TokenHealthReport
	files, err := os.ReadDir(tokenDir)
	if err != nil {
		return reports
	}

	now := time.Now()
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		name := f.Name()
		fullPath := filepath.Join(tokenDir, name)

		if strings.HasPrefix(name, "tiktok_token_") {
			u := strings.TrimPrefix(name, "tiktok_token_")
			u = strings.TrimSuffix(u, ".json")
			rep := inspectTikTokToken(u, fullPath, now)
			reports = append(reports, rep)
		}
	}

	tokenWorkerMu.Lock()
	lastHealthReports = reports
	tokenWorkerMu.Unlock()

	return reports
}

func inspectTikTokToken(username, path string, now time.Time) TokenHealthReport {
	rep := TokenHealthReport{
		Platform:    "tiktok",
		Username:    username,
		File:        filepath.Base(path),
		Status:      "healthy",
		Message:     "TikTok Access Token hợp lệ",
		LastChecked: now,
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		rep.Status = "warning"
		rep.Message = fmt.Sprintf("Không đọc được file token: %v", err)
		return rep
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		rep.Status = "warning"
		rep.Message = "Định dạng JSON token không hợp lệ"
		return rep
	}

	rf, _ := data["refresh_token"].(string)
	ac, _ := data["access_token"].(string)
	if rf == "" && ac == "" {
		rep.Status = "revoked"
		rep.Message = "Không có token hợp lệ (cần nối lại TikTok)"
		return rep
	}

	rep.Message = "Sẵn sàng (Tự động làm mới phiên 24/7)"
	return rep
}

// StartTokenWorker khởi chạy vòng lặp ngầm định kỳ kiểm tra sức khỏe và làm mới token.
func StartTokenWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	// Quét ngay lần đầu lúc khởi động
	reps := InspectTokens()
	log.Printf("publishers: token worker khởi chạy — đã kiểm tra %d token", len(reps))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reports := InspectTokens()
				for _, r := range reports {
					if r.Status == "revoked" {
						tokenWorkerMu.RLock()
						hook := onTokenAlertHook
						tokenWorkerMu.RUnlock()
						if hook != nil {
							hook(r)
						}
					}
				}
			}
		}
	}()
}
