package gmodels

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AutoUpdateReport ghi lại kết quả của lần quét kiểm tra cập nhật model Gemini mới nhất.
type AutoUpdateReport struct {
	CheckedAt   time.Time         `json:"checked_at"`
	Success     bool              `json:"success"`
	Error       string            `json:"error,omitempty"`
	Updated     bool              `json:"updated"`
	Changes     map[string]string `json:"changes,omitempty"`
	Current     map[string]string `json:"current"`
	Discovered  map[string]string `json:"discovered"`
	TotalModels int               `json:"total_models"`
}

var (
	reportMu       sync.RWMutex
	lastReport     *AutoUpdateReport
	updateInterval = 24 * time.Hour
	initialDelay   = 15 * time.Second
	apiBaseURL     = "https://generativelanguage.googleapis.com"
	apiHTTPClient  *http.Client
)

// SetAPIBaseURL cho phép tùy chỉnh Base URL của Gemini API (dùng trong test hoặc proxy).
func SetAPIBaseURL(u string) {
	apiBaseURL = u
}

// SetHTTPClient cho phép tùy chỉnh http.Client (dùng trong test).
func SetHTTPClient(c *http.Client) {
	apiHTTPClient = c
}

// SetUpdateInterval cho phép tùy chỉnh tần suất quét (dùng trong test).
func SetUpdateInterval(d time.Duration) {
	updateInterval = d
}

// SetInitialDelay cho phép tùy chỉnh thời gian chờ ban đầu (dùng trong test).
func SetInitialDelay(d time.Duration) {
	initialDelay = d
}

// LastReport trả về kết quả quét kiểm tra model gần nhất.
func LastReport() *AutoUpdateReport {
	reportMu.RLock()
	defer reportMu.RUnlock()
	if lastReport == nil {
		return nil
	}
	cp := *lastReport
	return &cp
}

func CheckAndUpdate(ctx context.Context, httpc *http.Client, baseURL string, keys []string, dataDir string) (*AutoUpdateReport, error) {
	if baseURL == "" {
		baseURL = apiBaseURL
	}
	if httpc == nil {
		if apiHTTPClient != nil {
			httpc = apiHTTPClient
		} else {
			httpc = &http.Client{Timeout: 30 * time.Second}
		}
	}

	report := &AutoUpdateReport{
		CheckedAt:  time.Now(),
		Changes:    make(map[string]string),
		Current:    make(map[string]string),
		Discovered: make(map[string]string),
	}

	if len(keys) == 0 {
		report.Success = false
		report.Error = "không có Gemini API key nào được cấu hình"
		reportMu.Lock()
		lastReport = report
		reportMu.Unlock()
		return report, fmt.Errorf("%s", report.Error)
	}

	var names []string
	var lastErr error
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		res, err := ListModels(ctx, httpc, baseURL, key)
		if err == nil && len(res) > 0 {
			names = res
			lastErr = nil
			break
		}
		lastErr = err
	}

	if len(names) == 0 {
		report.Success = false
		if lastErr != nil {
			report.Error = lastErr.Error()
		} else {
			report.Error = "không lấy được danh sách model từ Gemini API"
		}
		reportMu.Lock()
		lastReport = report
		reportMu.Unlock()
		return report, fmt.Errorf("check and update: %s", report.Error)
	}

	report.TotalModels = len(names)
	report.Success = true

	kinds := []Kind{Text, Image, TTS, Video}
	discMu.Lock()
	defer discMu.Unlock()

	for _, k := range kinds {
		best := PickLatest(names, k)
		cur := Resolve(k)
		label := kindLabel(k)

		if best != "" && best != cur {
			scoreBest := parseScore(best)
			scoreCur := parseScore(cur)
			if compareScores(scoreBest, scoreCur) > 0 {
				mu.Lock()
				discovered[k] = best
				mu.Unlock()
				report.Changes[label] = fmt.Sprintf("%s → %s", cur, best)
				report.Updated = true
				log.Printf("[gmodels] 🚀 Nâng cấp model %s: %s → %s", label, cur, best)
			}
		}
	}

	for _, k := range kinds {
		label := kindLabel(k)
		report.Current[label] = Resolve(k)
		mu.RLock()
		if d := discovered[k]; d != "" {
			report.Discovered[label] = d
		}
		mu.RUnlock()
	}

	if report.Updated && dataDir != "" {
		if err := SavePersisted(dataDir); err != nil {
			log.Printf("[gmodels] Lưu cache model thất bại: %v", err)
		}
	}

	reportMu.Lock()
	lastReport = report
	reportMu.Unlock()

	log.Printf("[gmodels] Hoàn tất kiểm tra model Gemini (Tổng %d models). Cập nhật: %v. Text=%s, Image=%s, TTS=%s, Video=%s",
		report.TotalModels, report.Updated, Resolve(Text), Resolve(Image), Resolve(TTS), Resolve(Video))
	return report, nil
}

// StartDailyUpdater bắt đầu tiến trình chạy ngầm quét cập nhật model mỗi 24h.
// Lần đầu tiên sẽ chạy sau initialDelay (mặc định 15s) để app khởi động trơn tru.
func StartDailyUpdater(ctx context.Context, getKeys func() []string, dataDir string) {
	log.Printf("[gmodels] Khởi động tiến trình tự động cập nhật model mỗi %v (kiểm tra đầu tiên sau %v)",
		updateInterval, initialDelay)

	// Lần chạy đầu tiên sau khi app boot
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay):
		keys := getKeys()
		if len(keys) > 0 {
			_, _ = CheckAndUpdate(ctx, apiHTTPClient, apiBaseURL, keys, dataDir)
		} else {
			log.Printf("[gmodels] Chưa có Gemini API key nào, hoãn quét cập nhật model.")
		}
	}

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[gmodels] Dừng tiến trình tự động cập nhật model.")
			return
		case <-ticker.C:
			keys := getKeys()
			if len(keys) > 0 {
				_, _ = CheckAndUpdate(ctx, apiHTTPClient, apiBaseURL, keys, dataDir)
			}
		}
	}
}
