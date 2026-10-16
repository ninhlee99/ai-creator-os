package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// KeyDraftDigestLastDate — ngày gửi digest "bản nháp chờ đăng" gần nhất
// (YYYY-MM-DD). Digest chỉ gửi 1 lần/ngày để không spam Telegram.
const KeyDraftDigestLastDate = "draft_digest.last_date"

// DraftDigestTick — mỗi ngày 1 lần, báo Telegram số bản nháp/video đang chờ
// bấm Đăng tay.
func (s *Service) DraftDigestTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Settings == nil {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	if last, _ := s.Settings.Get(KeyDraftDigestLastDate); last == today {
		return nil // đã báo hôm nay
	}

	var parts []string

	// 2. Video kể chuyện riêng tư chờ duyệt/đăng YouTube.
	if st, ok := s.Story.(*studio.Studio); ok && st != nil {
		n := 0
		for _, j := range st.StoryJobs(200) {
			if j.Status != studio.StatusDone || j.Output == "" {
				continue
			}
			if _, done := s.Settings.Get(KeyStoryPublishedPrefix + j.ID); !done {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("YouTube: %d video riêng tư chờ duyệt/đăng", n))
		}
	}

	if len(parts) == 0 {
		_ = s.Settings.Set(KeyDraftDigestLastDate, today)
		return nil
	}
	msg := "Có bản nháp chờ đăng: " + strings.Join(parts, "; ") + "."
	s.alert("draft_digest", "warn", msg, "")
	_ = s.Settings.Set(KeyDraftDigestLastDate, today)
	return []string{"draft_digest: " + msg}
}
