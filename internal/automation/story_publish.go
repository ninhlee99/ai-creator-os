package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"os"
	"strconv"
	"strings"
	"time"
)

func (s *Service) PublishStoryNow(ctx context.Context, jobID, username string) (string, bool) {
	if s.killed() || s.dryRun() {
		return "story: bị chặn bởi kill switch / dry-run", false
	}
	unlock, locked := tryStoryPublishLock(jobID)
	if !locked {
		return fmt.Sprintf("story %q: đang đăng rồi — bỏ qua để tránh trùng", jobID), true
	}
	defer unlock()
	if s.Story == nil {
		return "story: studio chưa sẵn sàng", false
	}
	stalePublishing := false
	staleTitle := ""
	j, ok := s.Story.GetJob(jobID)
	if !ok {
		return fmt.Sprintf("story: không tìm thấy job %s", jobID), false
	}
	if j.Kind != studio.KindStory {
		return fmt.Sprintf("story: job %s không phải truyện kể", jobID), false
	}
	if j.Status != studio.StatusDone || j.Output == "" {
		return fmt.Sprintf("story %q: chưa dựng xong (trạng thái: %s)", j.Title, j.Status), false
	}
	if s.Accounts == nil {
		return "story: chưa nối quản lý kênh", false
	}
	var acct *network.Account
	if accts, lerr := s.Accounts.List(); lerr == nil {
		for _, a := range accts {
			if a.Username == username {
				acct = a
				break
			}
		}
	}
	if acct == nil {
		return fmt.Sprintf("story: không tìm thấy kênh %q", username), false
	}
	if s.Uploader == nil {
		return "story: chưa nối YouTube uploader", false
	}
	// Đợt Y: chống đăng trùng khi bấm 2 lần (double-click / đua với
	// auto-publish). Marker "đang đăng": "title|RFC3339" — phân biệt "đang
	// đăng thật" (mới < 30 phút → bỏ qua) và "đăng dở do crash" (cũ →
	// kiểm tra trùng rồi đăng lại, đợt S/V). Đặt marker ngay trước bước
	// chậm (SEO/LLM, upload) để lần gọi đồng thời thứ hai thấy và bỏ qua.
	if s.Settings != nil {
		if _, done := s.Settings.Get(KeyStoryPublishedPrefix + jobID); done {
			return fmt.Sprintf("story %q: đã đăng rồi — bỏ qua để tránh trùng", jobID), true
		}
		if _, needsReview := s.Settings.Get(KeyStoryNeedsReviewPrefix + jobID); needsReview {
			return fmt.Sprintf("story %q: kết quả upload chưa rõ — cần đối chiếu trước khi thử lại", j.Title), false
		}
		if raw, uploading := s.Settings.Get(KeyStoryPublishingPrefix + jobID); uploading {
			title, ts := parsePublishingMarker(raw)
			if freshPublishingMarker(ts) {
				return fmt.Sprintf("story %q: đang đăng rồi — bỏ qua để tránh trùng", jobID), true
			}
			stalePublishing = true
			staleTitle = title
		} else {
			_ = s.Settings.Set(KeyStoryPublishingPrefix+jobID, "|"+time.Now().Format(time.RFC3339))
		}
	}
	// Đợt M1: SEO + thumbnail tự động. LLM viết title/desc/tags tiếng Việt;
	// thiếu LLM thì dùng template trung thực. Thumbnail = ảnh cảnh đầu tiên.
	vmeta := s.storySEOMeta(ctx, j)
	vmeta.ThumbnailPath = s.storyThumbnailPath(j.ID)
	// Đợt Y: khi đăng lại sau lần dở, GIỮ NGUYÊN title đã dùng lần trước
	// (lưu trong marker) thay vì sinh mới bằng LLM — search trùng mới trúng.
	if staleTitle != "" {
		vmeta.Title = staleTitle
	}
	if s.Settings != nil {
		_ = s.Settings.Set(KeyStoryPublishingPrefix+jobID,
			vmeta.Title+"|"+time.Now().Format(time.RFC3339))
	}
	var res publishers.PublishResult
	if stalePublishing {
		// Đợt V: lần trước đăng dở (crash/kill giữa upload và marker) — trước
		// khi đăng lại, hỏi YouTube xem video cùng tiêu đề đã có chưa để
		// tránh bản trùng tích lũy im lặng (mất mạng sau khi YouTube đã nhận
		// đủ bytes nhưng trước khi response về là kịch bản thật).
		if dup, derr := s.checkDuplicateUpload(ctx, acct, vmeta.Title); derr != nil {
			note := "Không đăng lại vì kiểm tra trùng thất bại: " + derr.Error()
			s.Story.AppendLog(jobID, note)
			if s.Settings != nil {
				_ = s.Settings.Set(KeyStoryNeedsReviewPrefix+jobID, derr.Error())
				_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
			}
			_ = s.decide("story_publisher", "publish_youtube_failed", &username,
				"không đăng lại vì chưa xác minh được lần upload trước.", map[string]any{
					"job_id": jobID, "title": vmeta.Title, "error": note,
					"delivery_uncertain": true,
				})
			return fmt.Sprintf("story %q: cần kiểm tra YouTube trước khi thử lại", j.Title), false
		} else if dup {
			s.Story.AppendLog(jobID, "Phát hiện video cùng tiêu đề đã có trên kênh (lần đăng dở trước đó) — bỏ qua đăng lại để tránh trùng.")
			if s.Settings != nil {
				_ = s.Settings.Set(KeyStoryPublishedPrefix+jobID, "1")
				_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
				_ = s.Settings.Delete(KeyStoryNeedsReviewPrefix + jobID)
			}
			return fmt.Sprintf("story %q: đã có trên kênh (lần đăng dở trước đó) — bỏ qua để tránh trùng", j.Title), true
		}
	}
	videoPath := s.OutputPath(j.Output)
	if mu, ok := s.Uploader.(MetaUploader); ok && mu != nil {
		res = mu.UploadMeta(ctx, acct, videoPath, vmeta, "short_film")
	} else {
		res = s.Uploader.Upload(ctx, acct, videoPath, vmeta.Title, vmeta.Description, "short_film")
	}
	if !res.Ok {
		// Đợt V: token chết (invalid_grant) thì retry vô ích — alert critical
		// "cần nối lại YouTube" ngay thay vì đốt 3 lần retry cách nhau 6h.
		if strings.Contains(res.Error, "cần nối lại YouTube") {
			s.alert("youtube_oauth_revoked", "critical",
				"YouTube: refresh token hết hạn hoặc bị thu hồi — video không đăng được.",
				"Nối lại YouTube ở trang Đa nền tảng rồi chạy lại job.")
			if s.Settings != nil {
				_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
				// Đợt AA: đánh dấu token chết để trang Đa nền tảng hiện
				// badge đỏ "Hết hạn — cần nối lại" thay vì "Có" xanh.
				// Xóa cờ khi nối lại thành công (youtube_oauth.go).
				_ = s.Settings.Set("token_dead:"+acct.Username, time.Now().Format(time.RFC3339))
			}
			return fmt.Sprintf("story %q: YouTube cần nối lại — đã dừng retry", j.Title), false
		}
		note := "Đăng YouTube thất bại: " + res.Error
		if res.DeliveryUncertain {
			note = "Kết quả upload YouTube chưa xác định; dừng retry và yêu cầu đối chiếu trước: " + res.Error
		}
		s.Story.AppendLog(jobID, note)
		_ = s.decide("story_publisher", "publish_youtube_failed", &username,
			"đăng truyện lên YouTube thất bại.", map[string]any{
				"job_id": jobID, "title": vmeta.Title, "error": res.Error,
				"delivery_uncertain": res.DeliveryUncertain,
			})
		if s.Settings != nil {
			if res.DeliveryUncertain {
				// needs_review blocks every retry until an operator resolves it;
				// remove the active marker so the dashboard shows this blocker now.
				_ = s.Settings.Set(KeyStoryNeedsReviewPrefix+jobID, res.Error)
				_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
			} else if res.UploadRequestAttempted {
				s.Story.AppendLog(jobID, "Giữ marker đăng dở; lần sau sẽ kiểm tra video trùng trước khi thử lại.")
			} else {
				_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
			}
		}
		if res.DeliveryUncertain {
			return fmt.Sprintf("story %q: kết quả upload chưa rõ — kiểm tra YouTube trước khi thử lại", j.Title), false
		}
		return fmt.Sprintf("story %q: đăng thất bại — %s", j.Title, res.Error), false
	}
	privacyLabel := "công khai"
	if res.Draft {
		privacyLabel = "chưa công khai"
	}
	logLine := "Đã đăng YouTube (" + privacyLabel + "): " + res.RemoteID
	if stalePublishing {
		logLine += " (đã kiểm tra trùng trước khi đăng lại)"
	}
	if vmeta.ThumbnailPath != "" {
		if res.ThumbnailSet {
			logLine += " + thumbnail"
		} else if res.ThumbnailError != "" {
			logLine += " (thumbnail lỗi: " + res.ThumbnailError + ")"
		}
	}
	s.Story.AppendLog(jobID, logLine)
	if s.Settings != nil {
		_ = s.Settings.Set(KeyStoryPublishedPrefix+jobID, "1")
		_ = s.Settings.Delete(KeyStoryNeedsReviewPrefix + jobID)
		// Đợt S: đăng xong → xóa marker "đang đăng".
		_ = s.Settings.Delete(KeyStoryPublishingPrefix + jobID)
		// Đợt N: hẹn giờ chuyển public (nếu bật). Không có videoID thì bỏ qua.
		s.enqueuePublic(jobID, res.RemoteID)
	}
	note := fmt.Sprintf("story %q: đã đăng YouTube (private)", j.Title)
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	if h := atInt(s.Settings, KeyStoryPublicAfterHours, 0); h > 0 && res.RemoteID != "" {
		note += fmt.Sprintf(" — tự chuyển công khai sau %d giờ", h)
	}
	return note, true
}

// enqueuePublic thêm video vào hàng chờ chuyển public (đợt N).
// Chỉ hẹn khi Ninh đã bật story.public_after_hours > 0.
func (s *Service) enqueuePublic(jobID, videoID string) {
	if s.Settings == nil || videoID == "" {
		return
	}
	if atInt(s.Settings, KeyStoryPublicAfterHours, 0) <= 0 {
		return
	}
	pending := s.pendingPublic()
	pending[jobID] = time.Now().Format("2006-01-02T15:04:05") + "|" + videoID
	s.savePendingPublic(pending)
}

// pendingPublic đọc hàng chờ chuyển public (map rỗng khi chưa có/lỗi).
func (s *Service) pendingPublic() map[string]string {
	out := map[string]string{}
	if s.Settings == nil {
		return out
	}
	raw, ok := s.Settings.Get(KeyStoryPendingPublic)
	if !ok || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (s *Service) savePendingPublic(m map[string]string) {
	if s.Settings == nil {
		return
	}
	raw, _ := json.Marshal(m)
	_ = s.Settings.Set(KeyStoryPendingPublic, string(raw))
}

// PrivacyUpdater: uploader đổi được quyền riêng tư video (soft-assert).
type PrivacyUpdater interface {
	UpdatePrivacy(ctx context.Context, a *network.Account, videoID, privacy string) error
}

// StoryPublicTick chuyển public các video private đã quá thời gian chờ
// (đợt N). 0 giờ = tắt. Tôn trọng kill switch + dry-run.
func (s *Service) StoryPublicTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	hours := atInt(s.Settings, KeyStoryPublicAfterHours, 0)
	if hours <= 0 || s.Settings == nil {
		return nil
	}
	pending := s.pendingPublic()
	if len(pending) == 0 {
		return nil
	}
	account := atStr(s.Settings, KeyStoryAccount, "")
	if account == "" || s.Accounts == nil || s.Uploader == nil {
		return nil
	}
	var acct *network.Account
	if accts, err := s.Accounts.List(); err == nil {
		for _, a := range accts {
			if a.Username == account {
				acct = a
				break
			}
		}
	}
	if acct == nil {
		return []string{fmt.Sprintf("story: không tìm thấy kênh %q để chuyển public", account)}
	}
	pu, ok := s.Uploader.(PrivacyUpdater)
	if !ok || pu == nil {
		return []string{"story: uploader không hỗ trợ đổi quyền riêng tư"}
	}
	// So sánh chuỗi theo format "2006-01-02T15:04:05" (cùng cách ghi khi
	// enqueue bằng time.Now().Format) — tránh bẫy timezone của time.Parse
	// (parse ra UTC trong khi Now() là giờ local).
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02T15:04:05")
	var notes []string
	changed := false
	const maxPublicFails = 10 // quá 10 lần lỗi liên tiếp → bỏ entry + báo 1 lần
	for jobID, val := range pending {
		// Đợt Y: value "ts|vid|fails" — đếm lỗi liên tiếp mỗi entry.
		ts, rest, _ := strings.Cut(val, "|")
		vid, failsStr, _ := strings.Cut(rest, "|")
		fails, _ := strconv.Atoi(failsStr)
		if vid == "" || ts == "" || ts >= cutoff {
			continue // chưa tới giờ → giữ lại
		}
		if _, err := time.ParseInLocation("2006-01-02T15:04:05", ts, time.Local); err != nil {
			continue // dữ liệu hỏng → giữ lại, không crash tick
		}
		if err := pu.UpdatePrivacy(ctx, acct, vid, "public"); err != nil {
			fails++
			if fails > maxPublicFails {
				delete(pending, jobID)
				changed = true
				s.alert("story_public_failed_"+jobID, "warn",
					fmt.Sprintf("Video %s chuyển công khai thất bại %d lần liên tiếp — đã dừng thử (có thể video bị xoá tay trên YouTube)", jobID, fails),
					"Kiểm tra video trên YouTube rồi đăng/chuyển tay nếu cần.")
				notes = append(notes, fmt.Sprintf("story %s: chuyển public lỗi %d lần — đã dừng và báo", jobID, fails))
			} else {
				pending[jobID] = ts + "|" + vid + "|" + strconv.Itoa(fails)
				changed = true
				notes = append(notes, fmt.Sprintf("story %s: chuyển public thất bại — %v (thử lại vòng sau, lần %d/%d)", jobID, err, fails, maxPublicFails))
			}
			continue
		}
		delete(pending, jobID)
		changed = true
		if s.Story != nil {
			s.Story.AppendLog(jobID, "Tự chuyển công khai sau "+strconv.Itoa(hours)+" giờ (đợt N)")
		}
		notes = append(notes, fmt.Sprintf("story %s: đã tự chuyển công khai", jobID))
	}
	if changed {
		s.savePendingPublic(pending)
	}
	return notes
}

// atStr đọc setting chuỗi (def khi chưa có).
func atStr(st Settings, key, def string) string {
	if st == nil {
		return def
	}
	if v, ok := st.Get(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// ---------------------------------------------------------------------------
// Đợt M1: SEO + thumbnail tự động khi đăng story.

// seoGenerator: Studio viết metadata YouTube bằng LLM (soft-assert).
type seoGenerator interface {
	GenerateSEOMeta(ctx context.Context, topic, genre, fallbackTitle string) (studio.SEOMeta, error)
}

// assetLister: Studio liệt kê asset của job (soft-assert).
type assetLister interface {
	ListAssets(jobID string) []studio.Asset
}

// MetaUploader: uploader hỗ trợ metadata đầy đủ (tags + thumbnail).
// Không bắt buộc — uploader cũ vẫn chạy qua Upload thường.
type MetaUploader interface {
	UploadMeta(ctx context.Context, a *network.Account, videoPath string, meta publishers.VideoMeta, kind string) publishers.PublishResult
}

// DuplicateChecker: uploader có thể kiểm tra video trùng đã đăng gần đây.
// Không bắt buộc — uploader không hỗ trợ thì bỏ qua bước kiểm tra.
type DuplicateChecker interface {
	HasRecentUpload(ctx context.Context, a *network.Account, title string, within time.Duration) (bool, error)
}

// parsePublishingMarker tách marker "đang đăng" dạng "title|RFC3339".
// Legacy (đợt S/V) lưu "1" → trả ("", "") để caller coi như đăng dở cũ.
func parsePublishingMarker(raw string) (title, ts string) {
	if raw == "" || raw == "1" {
		return "", ""
	}
	title, ts, _ = strings.Cut(raw, "|")
	return title, ts
}

// freshPublishingMarker: marker mới (< 30 phút) = đang đăng thật — lần gọi
// đồng thời thứ hai phải bỏ qua. Cũ hơn = đăng dở do crash (đợt S/V).
func freshPublishingMarker(ts string) bool {
	if ts == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	return time.Since(t) < 30*time.Minute
}

// checkDuplicateUpload hỏi uploader xem video cùng tiêu đề đã có trên kênh
// chưa. Không có checker phải trả lỗi để caller không retry mù.
func (s *Service) checkDuplicateUpload(ctx context.Context, acct *network.Account, title string) (bool, error) {
	dc, ok := s.Uploader.(DuplicateChecker)
	if !ok || dc == nil {
		return false, fmt.Errorf("uploader không hỗ trợ kiểm tra video gần đây")
	}
	return dc.HasRecentUpload(ctx, acct, title, 24*time.Hour)
}

// storySEOMeta dựng metadata đăng YouTube: LLM viết, lỗi/thiếu LLM thì
// dùng template trung thực (không bịa "tối ưu").
func (s *Service) storySEOMeta(ctx context.Context, j studio.Job) publishers.VideoMeta {
	var sp studio.StoryParams
	_ = json.Unmarshal([]byte(j.Params), &sp)
	topic := strings.TrimSpace(sp.Topic)
	if topic == "" {
		topic = strings.TrimSpace(j.Title)
	}
	if g, ok := s.Story.(seoGenerator); ok && g != nil {
		if m, err := g.GenerateSEOMeta(ctx, topic, sp.Genre, j.Title); err == nil && m.Title != "" {
			desc := strings.TrimSpace(m.Description)
			if desc != "" {
				desc += "\n\n"
			}
			return publishers.VideoMeta{
				Title:       m.Title,
				Description: desc + growth.DisclosureLine,
				Tags:        m.Tags,
			}
		}
		s.Story.AppendLog(j.ID, "SEO: không viết được bằng LLM — dùng tiêu đề/mô tả cơ bản")
	}
	t := studio.TemplateSEOMeta(topic, sp.Genre, growth.DisclosureLine)
	return publishers.VideoMeta{Title: t.Title, Description: t.Description, Tags: t.Tags}
}

// storyThumbnailPath chọn ảnh cảnh đầu tiên còn file làm thumbnail YouTube.
// "" = không đặt thumbnail tùy chỉnh (YouTube tự lấy khung hình).
func (s *Service) storyThumbnailPath(jobID string) string {
	al, ok := s.Story.(assetLister)
	if !ok || al == nil {
		return ""
	}
	for _, a := range al.ListAssets(jobID) {
		if a.Kind != "photo" || a.Status != studio.StatusDone || a.Path == "" {
			continue
		}
		if fi, err := os.Stat(a.Path); err == nil && !fi.IsDir() {
			return a.Path
		}
	}
	return ""
}
