package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/reup"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// KeyDraftDigestLastDate — ngày gửi digest "bản nháp chờ đăng" gần nhất
// (YYYY-MM-DD). Digest chỉ gửi 1 lần/ngày để không spam Telegram.
const KeyDraftDigestLastDate = "draft_digest.last_date"

// DraftDigestTick — mỗi ngày 1 lần, báo Telegram số bản nháp/video đang chờ
// Ninh bấm Đăng tay.
//
// Bối cảnh (Ninh chốt 2026-10-12): KHÔNG dùng Playwright tự đăng TikTok
// (rủi ro ban kênh + gánh bảo trì). Luồng chính thức: hệ thống chuẩn bị hết
// bản nháp (TikTok/Facebook draft-only, YouTube private-first) → Telegram báo
// 1 lần/ngày → Ninh mở app bấm Đăng.
//
// Đếm:
//  1. Bài reup đã "đăng" trong hôm nay lên TikTok/Facebook — thực chất là bản
//     nháp nằm trong app, chờ bấm tay.
//  2. Job kể chuyện đã dựng xong nhưng chưa đăng YouTube (video riêng tư chờ
//     duyệt).
func (s *Service) DraftDigestTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Settings == nil {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	// So sánh theo UTC vì posted_at lưu UTC (nowUTC) — giờ local +07 sẽ bỏ
	// sót bài đăng 0h–7h sáng.
	todayUTC := time.Now().UTC().Format("2006-01-02")
	if last, _ := s.Settings.Get(KeyDraftDigestLastDate); last == today {
		return nil // đã báo hôm nay
	}

	var parts []string

	// 1. Bản nháp TikTok/Facebook/YouTube mới trong hôm nay.
	if s.Reup != nil {
		counts := map[string]int{}
		// ListPostsByStatusSince: lọc theo posted_at + mới nhất trước — tránh
		// LIMIT cắt mất bài mới khi tổng bài đã vượt giới hạn.
		if posts, err := s.Reup.ListPostsByStatusSince(reup.PostPosted, todayUTC, 200); err == nil {
			for _, p := range posts {
				switch p.Platform {
				case "tiktok", "facebook", "youtube":
					counts[p.Platform]++
				}
			}
		}
		if n := counts["tiktok"]; n > 0 {
			parts = append(parts, fmt.Sprintf("TikTok: %d bản nháp mới — mở app TikTok để đăng", n))
		}
		if n := counts["facebook"]; n > 0 {
			parts = append(parts, fmt.Sprintf("Facebook: %d bản nháp mới — mở Page để đăng", n))
		}
		if n := counts["youtube"]; n > 0 {
			parts = append(parts, fmt.Sprintf("YouTube: %d video reup riêng tư mới chờ duyệt", n))
		}
	}

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
