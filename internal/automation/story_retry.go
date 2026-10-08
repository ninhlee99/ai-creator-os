package automation

// Đợt P — tự thử lại job story lỗi (retry có backoff).
//
// Trước đây: job kể chuyện fail lúc 3h sáng (429 hết quota, mạng chập
// chờn, TTS lỗi) thì nằm im — Ninh mất trọn 1 ngày truyện. Giờ máy tự
// thử lại: cooldown mặc định 6h (đủ để quota hồi / lỗi thoáng qua hết),
// tối đa 3 lần, mỗi lần là job MỚI với cùng params (job cũ giữ nguyên
// làm lịch sử). Hết 3 lần → dừng + alert để Ninh xem tay.
//
// Cổng an toàn như mọi tick: kill switch + dry-run chặn; công tắc riêng
// story.retry_enabled mặc định BẬT (unset = bật — tiền lệ Đợt 3).

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

const (
	KeyStoryRetryEnabled  = "story.retry_enabled"    // "1"/"0", mặc định bật
	KeyStoryRetryMax      = "story.retry_max"        // số lần thử lại tối đa, mặc định 3
	KeyStoryRetryCooldown = "story.retry_cooldown_h" // giờ nghỉ giữa các lần, mặc định 6
	KeyStoryRetriedPrefix = "story.retried."         // story.retried.<jobID> = "1" (đã thử lại)
	KeyStoryRetryAlerted  = "story.retry_alerted."   // đã báo hết lượt (tránh spam alert)
	defaultRetryMax       = 3
	defaultRetryCooldownH = 6
	retryStaleDays        = 7 // job lỗi quá 7 ngày thì không đào lại
)

// storyJobLister: khả năng liệt kê job story (soft-assert — *studio.Studio
// có, test inject fake).
type storyJobLister interface {
	StoryJobs(limit int) []studio.Job
}

// StoryRetryTick thử lại các job story lỗi còn trong giới hạn.
// Chạy trong goroutine story (5 phút/lần), mỗi tick tự kiểm tra điều kiện.
func (s *Service) StoryRetryTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyStoryRetryEnabled, true) {
		return nil
	}
	st, ok := s.Story.(storyJobLister)
	if !ok || st == nil || s.Settings == nil {
		return nil
	}
	maxAtt := atInt(s.Settings, KeyStoryRetryMax, defaultRetryMax)
	if maxAtt < 1 {
		maxAtt = 1
	}
	cooldown := time.Duration(atInt(s.Settings, KeyStoryRetryCooldown,
		defaultRetryCooldownH)) * time.Hour
	if cooldown < 0 {
		cooldown = 0
	}

	var failed []studio.Job
	for _, j := range st.StoryJobs(30) {
		if j.Status == studio.StatusFailed {
			failed = append(failed, j)
		}
	}
	// Thử lại job lỗi cũ nhất trước (công bằng).
	sort.Slice(failed, func(i, k int) bool {
		return failed[i].CreatedAt < failed[k].CreatedAt
	})

	var notes []string
	for _, j := range failed {
		if _, done := s.Settings.Get(KeyStoryRetriedPrefix + j.ID); done {
			continue // đã thử lại job này rồi
		}
		var sp studio.StoryParams
		if err := json.Unmarshal([]byte(j.Params), &sp); err != nil {
			continue // params hỏng → bỏ qua, không crash tick
		}
		if sp.Attempt >= maxAtt {
			// Hết lượt thử lại → báo 1 lần để Ninh xem tay (không spam).
			if _, alerted := s.Settings.Get(KeyStoryRetryAlerted + j.ID); !alerted {
				s.alert("story_retry_exhausted", "warn",
					fmt.Sprintf("Truyện %q lỗi sau %d lần thử lại — máy dừng để khỏi đốt quota", j.Title, maxAtt),
					"Xem log job ở trang Kể chuyện rồi tạo lại tay nếu cần.")
				_ = s.Settings.Set(KeyStoryRetryAlerted+j.ID, "1")
				notes = append(notes, fmt.Sprintf("story: %q hết %d lần thử lại — đã báo", j.Title, maxAtt))
			}
			continue
		}
		failedAt, err := time.ParseInLocation("2006-01-02T15:04:05",
			j.FinishedAt, time.Local)
		if err != nil {
			continue // không rõ thời điểm lỗi → không thử bừa
		}
		if time.Since(failedAt) < cooldown {
			continue // chưa hết cooldown
		}
		if time.Since(failedAt) > retryStaleDays*24*time.Hour {
			continue // lỗi quá cũ → để yên, Ninh xem tay nếu cần
		}
		sp.Attempt++
		newID, err := s.Story.CreateStoryJob(sp)
		if err != nil {
			notes = append(notes, fmt.Sprintf("story: thử lại %q lỗi: %v", j.Title, err))
			continue
		}
		_ = s.Settings.Set(KeyStoryRetriedPrefix+j.ID, "1")
		s.Story.AppendLog(j.ID, fmt.Sprintf("Tự thử lại lần %d → job %s (đợt P)", sp.Attempt, newID))
		log.Printf("story: retry job lỗi %s (%q) → job mới %s (lần %d)",
			j.ID, j.Title, newID, sp.Attempt)
		notes = append(notes, fmt.Sprintf("story: tự thử lại %q (lần %d/%d)",
			j.Title, sp.Attempt, maxAtt))
	}
	return notes
}
