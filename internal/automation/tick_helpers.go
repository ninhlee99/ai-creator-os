package automation

import (
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// StudioRunner là biên render affiliate mà background tick cần.
// *studio.Studio thỏa mãn; test inject fake.
type StudioRunner interface {
	CreateAffiliateJob(p studio.AffiliateParams) (string, error)
}

// atOn đọc công tắc UI; unset dùng giá trị mặc định của workflow.
func atOn(st Settings, key string, def bool) bool {
	if st == nil {
		return def
	}
	v, ok := st.Get(key)
	if !ok {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}

// atInt đọc số nguyên từ settings; sai/thiếu → def.
func atInt(st Settings, key string, def int) int {
	if st == nil {
		return def
	}
	if v, ok := st.Get(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// dueSince kiểm tra watermark lastRun đã quá interval chưa.
func dueSince(st Settings, lastKey string, interval time.Duration) bool {
	if st == nil {
		return true
	}
	v, ok := st.Get(lastKey)
	if !ok || v == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return true
	}
	return time.Since(t) >= interval
}

func stampRun(st Settings, lastKey string) {
	if st == nil {
		return
	}
	_ = st.Set(lastKey, time.Now().Format(time.RFC3339))
}
