// Package gmodels chọn model Gemini còn sống cho từng loại việc (viết chữ,
// tạo ảnh, giọng đọc).
//
// Google đổi/tắt model ID liên tục (gemini-2.0-flash tắt 01/06/2026,
// gemini-2.0-flash-preview-image-generation tắt 14/11/2025) — ghi cứng 1 ID
// là hỏng sau vài tháng. Thứ tự chọn:
//
//  1. Ghi đè thủ công: GEMINI_TEXT_MODEL / GEMINI_IMAGE_MODEL /
//     GEMINI_TTS_MODEL (sửa được ở Cài đặt · Hệ thống).
//  2. Model tự dò được qua ListModels sau lần gặp lỗi "model không còn".
//  3. Mặc định ghi trong code (cập nhật 12/10/2026).
//
// Caller gọi Resolve trước mỗi request; nếu request trả lỗi mà IsModelGone
// nhận ra (404 / NOT_FOUND / not supported) thì gọi Rediscover rồi thử lại
// đúng 1 lần.
package gmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Kind là loại việc cần model.
type Kind int

const (
	Text Kind = iota
	Image
	TTS
)

// Mặc định cập nhật theo các model chính thức mới nhất của Google Gemini.
// Khi Google cập nhật model mới hơn trên API, Rediscover sẽ tự động phát hiện và chuyển đổi.
var defaults = map[Kind]string{
	Text:  "gemini-3.8-flash",
	Image: "gemini-2.0-flash-preview-image-generation",
	TTS:   "gemini-2.5-flash-preview-tts",
}


// EnvName là biến môi trường ghi đè model cho kind.
func EnvName(k Kind) string {
	switch k {
	case Image:
		return "GEMINI_IMAGE_MODEL"
	case TTS:
		return "GEMINI_TTS_MODEL"
	default:
		return "GEMINI_TEXT_MODEL"
	}
}

var (
	mu         sync.RWMutex
	discovered = map[Kind]string{}
	// discMu serialize quá trình discovery — tránh N goroutine cùng gọi
	// ListModels khi model chết đồng loạt (đợt AM fix).
	discMu sync.Mutex
)

// Default trả model mặc định trong code (không tính ghi đè/tự dò).
func Default(k Kind) string { return defaults[k] }

// Resolve trả model nên dùng ngay lúc này.
func Resolve(k Kind) string {
	if v := strings.TrimSpace(os.Getenv(EnvName(k))); v != "" {
		return strings.TrimPrefix(v, "models/")
	}
	mu.RLock()
	d := discovered[k]
	mu.RUnlock()
	if d != "" {
		return d
	}
	return defaults[k]
}

// Reset xóa kết quả tự dò (test).
func Reset() {
	mu.Lock()
	discovered = map[Kind]string{}
	mu.Unlock()
}

// IsModelGone nhận ra lỗi do model ID không còn (bị tắt/đổi tên), khác lỗi
// key hay quota.
func IsModelGone(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "http 404") || strings.Contains(s, "not_found") {
		return true
	}
	return strings.Contains(s, "models/") &&
		(strings.Contains(s, "is not found") || strings.Contains(s, "not supported") ||
			strings.Contains(s, "deprecated") || strings.Contains(s, "no longer available"))
}

type listResp struct {
	Models []struct {
		Name    string   `json:"name"`
		Methods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

// ListModels gọi API ListModels, trả tên model (bỏ tiền tố "models/") hỗ
// trợ generateContent.
func ListModels(ctx context.Context, httpc *http.Client, baseURL, key string) ([]string, error) {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	var out []string
	page := ""
	for i := 0; i < 10; i++ {
		u := strings.TrimSuffix(baseURL, "/") + "/v1beta/models?pageSize=1000&key=" + key
		if page != "" {
			u += "&pageToken=" + page
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpc.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list models: HTTP %d", resp.StatusCode)
		}
		var lr listResp
		if err := json.Unmarshal(data, &lr); err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		for _, m := range lr.Models {
			for _, meth := range m.Methods {
				if meth == "generateContent" {
					out = append(out, strings.TrimPrefix(m.Name, "models/"))
					break
				}
			}
		}
		if lr.NextPageToken == "" {
			break
		}
		page = lr.NextPageToken
	}
	return out, nil
}

// Rediscover dò lại model còn sống cho kind bằng key cho trước, ghi nhớ
// và trả model mới. Model hiện tại vẫn còn trong danh sách thì giữ nguyên
// (lỗi không phải do model) — trả lỗi để caller không thử lại vô ích.
func Rediscover(ctx context.Context, httpc *http.Client, baseURL, key string, k Kind) (string, error) {
	discMu.Lock()
	defer discMu.Unlock()
	// Double-check: goroutine khác có thể đã dò xong trong lúc chờ lock.
	cur := Resolve(k)
	mu.RLock()
	if d := discovered[k]; d != "" && d != cur {
		mu.RUnlock()
		return d, nil
	}
	mu.RUnlock()
	names, err := ListModels(ctx, httpc, baseURL, key)
	if err != nil {
		return "", err
	}
	var filtered []string
	for _, n := range names {
		if n != cur {
			filtered = append(filtered, n)
		}
	}
	best := Pick(filtered, k)
	if best == "" {
		return "", fmt.Errorf("không tìm thấy model %s nào còn hoạt động", kindLabel(k))
	}
	mu.Lock()
	discovered[k] = best
	mu.Unlock()
	return best, nil

}

func kindLabel(k Kind) string {
	switch k {
	case Image:
		return "tạo ảnh"
	case TTS:
		return "giọng đọc"
	default:
		return "viết chữ"
	}
}

var verRe = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

func version(name string) float64 {
	m := verRe.FindString(name)
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

func has(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// Pick chọn model tốt nhất cho kind trong danh sách: ưu tiên mặc định nếu
// còn, rồi bản ổn định (không preview/exp), rồi phiên bản cao nhất.
func Pick(names []string, k Kind) string {
	var cands []string
	for _, raw := range names {
		n := strings.ToLower(raw)
		ok := false
		switch k {
		case Text:
			ok = strings.HasPrefix(n, "gemini-") && strings.Contains(n, "flash") &&
				!has(n, "image", "tts", "audio", "live", "embedding", "banana", "vision")
		case Image:
			ok = has(n, "banana", "image") && !has(n, "imagen", "tts", "embedding")
		case TTS:
			ok = strings.Contains(n, "tts")
		}
		if ok {
			cands = append(cands, raw)
		}
	}
	if len(cands) == 0 {
		return ""
	}
	for _, c := range cands {
		if c == defaults[k] {
			return c
		}
	}
	score := func(n string) (int, float64, int) {
		l := strings.ToLower(n)
		stable := 1
		if has(l, "preview", "exp") {
			stable = 0
		}
		lite := 0 // ưu tiên bản không-lite (chất lượng) khi cùng phiên bản
		if strings.Contains(l, "lite") {
			lite = -1
		}
		return stable, version(l), lite
	}
	sort.SliceStable(cands, func(i, j int) bool {
		si, vi, li := score(cands[i])
		sj, vj, lj := score(cands[j])
		if si != sj {
			return si > sj
		}
		if vi != vj {
			return vi > vj
		}
		return li > lj
	})
	return cands[0]
}
