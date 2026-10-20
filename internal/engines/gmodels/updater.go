package gmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

var versionMatcher = regexp.MustCompile(`(\d+)(?:\.(\d+))?`)

type candidateScore struct {
	name    string
	major   int
	minor   int
	stable  int // 2 = GA/stable, 1 = preview, 0 = exp
	nonLite int // 1 = standard, 0 = lite
}

func parseScore(name string) candidateScore {
	l := strings.ToLower(name)
	m := versionMatcher.FindStringSubmatch(l)
	major, minor := 0, 0
	if len(m) > 1 {
		major, _ = strconv.Atoi(m[1])
	}
	if len(m) > 2 && m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}

	// Xếp hạng ưu tiên cho dòng Nano Banana tạo ảnh & Veo video
	if strings.Contains(l, "nano-banana-2.1") {
		major = 3
		minor = 9
	} else if strings.Contains(l, "nano-banana-2") {
		major = 3
		minor = 2
	} else if strings.Contains(l, "banana") && major < 3 {
		major = 3
		minor = 1
	}

	stable := 2
	if strings.Contains(l, "preview") {
		stable = 1
	} else if strings.Contains(l, "exp") {
		stable = 0
	}

	nonLite := 1
	if strings.Contains(l, "lite") {
		nonLite = 0
	}

	return candidateScore{
		name:    name,
		major:   major,
		minor:   minor,
		stable:  stable,
		nonLite: nonLite,
	}
}

func compareScores(a, b candidateScore) int {
	if a.major != b.major {
		if a.major > b.major {
			return 1
		}
		return -1
	}
	if a.minor != b.minor {
		if a.minor > b.minor {
			return 1
		}
		return -1
	}
	if a.stable != b.stable {
		if a.stable > b.stable {
			return 1
		}
		return -1
	}
	if a.nonLite != b.nonLite {
		if a.nonLite > b.nonLite {
			return 1
		}
		return -1
	}
	return 0
}

// PickLatest chọn model mới nhất khả dụng từ danh sách API trả về.
// So sánh theo: Phiên bản (major.minor) > Độ ổn định (GA > Preview > Exp) > Phiên bản đầy đủ (Chuẩn > Lite).
func PickLatest(names []string, k Kind) string {
	var cands []candidateScore
	for _, raw := range names {
		n := strings.ToLower(raw)
		ok := false
		switch k {
		case Text:
			ok = strings.HasPrefix(n, "gemini-") && strings.Contains(n, "flash") &&
				!has(n, "image", "tts", "audio", "live", "embedding", "banana", "vision", "veo")
		case Image:
			ok = has(n, "banana", "image") && !has(n, "imagen", "tts", "audio", "live", "embedding", "veo")
		case TTS:
			ok = strings.Contains(n, "tts")
		case Video:
			ok = strings.Contains(n, "veo") || strings.Contains(n, "video")
		}
		if ok {
			cands = append(cands, parseScore(raw))
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.SliceStable(cands, func(i, j int) bool {
		cmp := compareScores(cands[i], cands[j])
		if cmp != 0 {
			return cmp > 0
		}
		// Nếu điểm bằng nhau, ưu tiên tên ngắn gọn (alias chính thức như gemini-3.8-flash thay vì gemini-3.8-flash-001)
		return len(cands[i].name) < len(cands[j].name)
	})
	return cands[0].name
}

type persistedModels struct {
	UpdatedAt  time.Time         `json:"updated_at"`
	Discovered map[string]string `json:"discovered"`
}

const persistedFile = "discovered_models.json"

// LoadPersisted tải các model đã tự dò được lưu trước đó từ thư mục dataDir.
func LoadPersisted(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	p := filepath.Join(dataDir, persistedFile)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var pm persistedModels
	if err := json.Unmarshal(data, &pm); err != nil {
		return err
	}
	mu.Lock()
	for kStr, model := range pm.Discovered {
		if model == "" {
			continue
		}
		switch kStr {
		case "text":
			discovered[Text] = model
		case "image":
			discovered[Image] = model
		case "tts":
			discovered[TTS] = model
		case "video":
			discovered[Video] = model
		}
	}
	mu.Unlock()
	log.Printf("[gmodels] Đã nạp danh sách model đã cập nhật từ cache: Text=%s, Image=%s, TTS=%s, Video=%s",
		Resolve(Text), Resolve(Image), Resolve(TTS), Resolve(Video))
	return nil
}

// SavePersisted lưu danh sách model hiện tại vào dataDir.
func SavePersisted(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	mu.RLock()
	discCopy := make(map[string]string)
	for k, v := range discovered {
		if v != "" {
			switch k {
			case Text:
				discCopy["text"] = v
			case Image:
				discCopy["image"] = v
			case TTS:
				discCopy["tts"] = v
			case Video:
				discCopy["video"] = v
			}
		}
	}
	mu.RUnlock()

	pm := persistedModels{
		UpdatedAt:  time.Now(),
		Discovered: discCopy,
	}
	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(dataDir, persistedFile)
	return os.WriteFile(p, data, 0644)
}

// CheckAndUpdate quét Google API để tìm các model mới nhất và tự động nâng cấp nếu phát hiện bản mới hơn.
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
