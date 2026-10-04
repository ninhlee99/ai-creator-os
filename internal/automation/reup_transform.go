package automation

// Reup transform/post/kill automation (Đợt E) — zero-touch, Ninh không chạm.
//
//   - ReupTransformTick: 15 phút/lần — video downloaded chưa có bài →
//     transform theo mức mặc định (1 hoặc 2) → bài chờ đăng.
//   - ReupPostTick: 2 giờ/lần — bài đã transform → đăng lên kênh đã chọn
//     (giới hạn/ngày + warm-up), qua publishers hiện có (TikTok draft-first,
//     YouTube private-first — fail-closed như cũ).
//   - ReupKillTick: 1 giờ/lần — đồng bộ view thật (YouTube videos.list;
//     TikTok "chờ số liệu") → N bài liên tiếp 0-view → DỪNG ĐĂNG REUP +
//     alert + đề xuất. Không tự xoá bài đã đăng.
//
// Cùng cổng an toàn như các tick khác: kill switch + DRY-RUN chặn;
// công tắc riêng mặc định BẬT (unset = bật — tiền lệ Đợt 3).
//
// TRUNG THỰC: không chỗ nào hứa "an toàn bản quyền" — transform chỉ
// "giảm rủi ro". Không bao giờ đăng video 0 transform.

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// Setting keys cho reup transform/post/kill (ledger settings).
const (
	KeyReupTransformEnabled = "reup.transform_enabled"
	KeyReupTransformLevel   = "reup.transform_level" // 1 | 2
	KeyReupTransformLastRun = "reup.transform_last_run"
	KeyReupVoiceoverEnabled = "reup.voiceover_enabled"
	KeyReupPostEnabled      = "reup.post_enabled" // kill rule tắt khi trigger
	KeyReupPostLastRun      = "reup.post_last_run"
	KeyReupVideosPerDay     = "reup.videos_per_day" // mặc định 3
	KeyReupPostAccount      = "reup.post_account"   // username kênh đăng
	KeyReupKillEnabled      = "reup.killrule_enabled"
	KeyReupKillZeroN        = "reup.killrule_zeroview_n" // mặc định 5
	KeyReupKillLastRun      = "reup.kill_last_run"
	KeyReupWarmupEnabled    = "reup.warmup_enabled"
	KeyReupWarmupStart      = "reup.warmup_start" // YYYY-MM-DD bài đăng đầu

	defaultReupTransformLevel = 1
	defaultReupVideosPerDay   = 3
	transformTickEvery        = 15 * time.Minute
	postTickEvery             = 2 * time.Hour
	killTickEvery             = time.Hour
	maxTransformPerPass       = 2
)

// reupTransformer dựng transformer từ state hiện tại; nil khi kho chưa mở.
func (s *Service) reupTransformer() *reup.Transformer {
	if s.Reup == nil {
		return nil
	}
	dir := s.ReupWorkDir
	if dir == "" {
		dir = "."
	}
	return &reup.Transformer{
		Store:     s.Reup,
		Voice:     s.ReupTTS,
		LLM:       s.ReupLLM,
		MusicPath: s.ReupMusicPath,
		WorkDir:   filepath.Join(dir, "transform"),
	}
}

// ------------------------------------------------------- transform tick

// ReupTransformTick transform video đã tải → bài chờ đăng.
func (s *Service) ReupTransformTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyReupTransformEnabled, true) {
		return nil
	}
	if s.Reup == nil {
		return nil
	}
	if !dueSince(s.Settings, KeyReupTransformLastRun, transformTickEvery) {
		return nil
	}
	stampRun(s.Settings, KeyReupTransformLastRun)
	tr := s.reupTransformer()
	if tr == nil {
		return nil
	}
	level := atInt(s.Settings, KeyReupTransformLevel, defaultReupTransformLevel)
	if level != reup.Level2 {
		level = reup.Level1
	}
	voiceover := atOn(s.Settings, KeyReupVoiceoverEnabled, true)

	var done, failed int
	var notes []string
	if level == reup.Level2 {
		done, failed, notes = s.transformPassCompilation(ctx, tr, voiceover)
	} else {
		done, failed, notes = s.transformPassSingle(ctx, tr, voiceover)
	}
	out := []string{fmt.Sprintf("reup transform (mức %d): %d xong, %d lỗi", level, done, failed)}
	return append(out, notes...)
}

// transformPassSingle: mỗi video downloaded → 1 bài mức 1.
func (s *Service) transformPassSingle(ctx context.Context, tr *reup.Transformer, voiceover bool) (done, failed int, notes []string) {
	videos, err := s.Reup.VideosNeedingTransform(maxTransformPerPass)
	if err != nil {
		return 0, 0, []string{"đọc video chờ transform: " + err.Error()}
	}
	for _, v := range videos {
		post, cerr := s.Reup.CreatePost([]int64{v.ID}, reup.Level1)
		if cerr != nil {
			failed++
			notes = append(notes, fmt.Sprintf("tạo bài cho video #%d: %v", v.ID, cerr))
			continue
		}
		_ = s.Reup.SetPostStatus(post.ID, reup.PostTransforming, "")
		opts := reup.TransformOptions{
			Level:     reup.Level1,
			Seed:      v.ID,
			Title:     v.Title,
			Voiceover: voiceover,
		}
		out, terr := tr.TransformVideo(ctx, v, opts)
		if terr != nil {
			failed++
			_ = s.Reup.SetPostStatus(post.ID, reup.PostFailed, terr.Error())
			notes = append(notes, fmt.Sprintf("transform video #%d lỗi: %v", v.ID, terr))
			continue
		}
		_ = s.Reup.SetPostTransformed(post.ID, out)
		done++
	}
	return done, failed, notes
}

// transformPassSingle: 3 video cùng nguồn → 1 bài compilation mức 2.
func (s *Service) transformPassCompilation(ctx context.Context, tr *reup.Transformer, voiceover bool) (done, failed int, notes []string) {
	srcs, err := s.Reup.EnabledSources()
	if err != nil || len(srcs) == 0 {
		return 0, 0, []string{"không có nguồn bật để làm compilation"}
	}
	for _, src := range srcs {
		vs, verr := s.Reup.VideosForCompilation(src.ID, 3)
		if verr != nil || len(vs) < 3 {
			continue
		}
		ids := []int64{vs[0].ID, vs[1].ID, vs[2].ID}
		post, cerr := s.Reup.CreatePost(ids, reup.Level2)
		if cerr != nil {
			failed++
			notes = append(notes, "tạo bài compilation: "+cerr.Error())
			continue
		}
		_ = s.Reup.SetPostStatus(post.ID, reup.PostTransforming, "")
		opts := reup.TransformOptions{
			Level:     reup.Level2,
			Seed:      vs[0].ID,
			Title:     src.DisplayName,
			Voiceover: voiceover,
		}
		out, terr := tr.TransformCompilation(ctx, vs, opts)
		if terr != nil {
			failed++
			_ = s.Reup.SetPostStatus(post.ID, reup.PostFailed, terr.Error())
			notes = append(notes, fmt.Sprintf("compilation lỗi: %v", terr))
			continue
		}
		_ = s.Reup.SetPostTransformed(post.ID, out)
		done++
		if done >= maxTransformPerPass {
			break
		}
	}
	return done, failed, notes
}

// ----------------------------------------------------------- post tick

// reupDailyLimit tính giới hạn đăng hôm nay (warm-up: tăng dần ngày đầu).
func (s *Service) reupDailyLimit() int {
	limit := atInt(s.Settings, KeyReupVideosPerDay, defaultReupVideosPerDay)
	if limit < 1 {
		limit = 1
	}
	if !atOn(s.Settings, KeyReupWarmupEnabled, true) {
		return limit
	}
	start, _ := s.Settings.Get(KeyReupWarmupStart)
	if start == "" {
		return 1 // chưa đăng bao giờ → ngày đầu 1 video
	}
	d0, err := time.Parse("2006-01-02", start)
	if err != nil {
		return 1
	}
	days := int(time.Since(d0).Hours() / 24)
	switch {
	case days < 2:
		return min(1, limit)
	case days < 4:
		return min(2, limit)
	default:
		return limit
	}
}

// ReupPostTick đăng bài đã transform lên kênh đã chọn.
func (s *Service) ReupPostTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Reup == nil {
		return nil
	}
	if !atOn(s.Settings, KeyReupPostEnabled, true) {
		return []string{"reup post: TẠM DỪNG (kill rule 0-view đã kích hoạt — bật lại ở Cài đặt · Reup)"}
	}
	account := strings.TrimSpace(stringSetting(s.Settings, KeyReupPostAccount))
	if account == "" {
		return []string{"reup post: chưa chọn kênh đăng (Cài đặt · Reup) — bỏ qua"}
	}
	if !dueSince(s.Settings, KeyReupPostLastRun, postTickEvery) {
		return nil
	}
	// Giới hạn/ngày (+ warm-up).
	limit := s.reupDailyLimit()
	today := time.Now().Format("2006-01-02")
	n, err := s.Reup.CountPostedSince(account, today+"T00:00:00Z")
	if err != nil {
		return []string{"reup post: đếm bài đã đăng lỗi: " + err.Error()}
	}
	if n >= limit {
		return []string{fmt.Sprintf("reup post: đã đủ %d/%d bài hôm nay", n, limit)}
	}
	posts, err := s.Reup.ListPostsByStatus(reup.PostTransformed, 1)
	if err != nil {
		return []string{"reup post: đọc bài chờ đăng lỗi: " + err.Error()}
	}
	if len(posts) == 0 {
		return []string{"reup post: không có bài chờ đăng"}
	}
	stampRun(s.Settings, KeyReupPostLastRun)
	note, _ := s.PublishReupPostNow(ctx, posts[0].ID, account)
	return []string{note}
}

// PublishReupPostNow đăng 1 bài ngay (UI nút "Đăng" dùng chung logic với
// tick). Trả về (ghi chú, thành công?).
func (s *Service) PublishReupPostNow(ctx context.Context, postID int64, username string) (string, bool) {
	if s.killed() || s.dryRun() {
		return "reup post: bị chặn bởi kill switch / dry-run", false
	}
	if s.Reup == nil {
		return "reup post: kho reup chưa sẵn sàng", false
	}
	post, err := s.Reup.GetPost(postID)
	if err != nil {
		return fmt.Sprintf("reup post: %v", postID), false
	}
	if post.Status != reup.PostTransformed {
		return fmt.Sprintf("reup post #%d: chưa transform xong (trạng thái: %s)",
			postID, post.StatusLabel()), false
	}
	return s.publishReupPost(ctx, post, strings.TrimSpace(username))
}

// publishReupPost đăng 1 bài qua publishers hiện có. Trả về (ghi chú, ok?).
func (s *Service) publishReupPost(ctx context.Context, post reup.Post, username string) (string, bool) {
	if s.Accounts == nil {
		_ = s.Reup.SetPostStatus(post.ID, reup.PostFailed, "chưa nối quản lý kênh")
		return fmt.Sprintf("reup post #%d: chưa nối quản lý kênh", post.ID), false
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
		_ = s.Reup.SetPostStatus(post.ID, reup.PostFailed, "không tìm thấy kênh "+username)
		return fmt.Sprintf("reup post #%d: không tìm thấy kênh %q", post.ID, username), false
	}
	_ = s.Reup.SetPostStatus(post.ID, reup.PostPosting, "")
	title := fmt.Sprintf("Reup #%d", post.ID)
	if len(post.VideoIDs) > 0 {
		if v, verr := s.Reup.GetVideo(post.VideoIDs[0]); verr == nil && strings.TrimSpace(v.Title) != "" {
			title = v.Title
		}
	}
	desc := "Video reup đã transform (giảm rủi ro bản quyền — không đảm bảo)."
	for _, pub := range publishers.BuildPublishers(acct.Username, acct.YoutubeChannel, acct.YoutubeContentTypes, "short_video") {
		res := pub.Publish(ctx, post.FilePath, title, desc, "short_video")
		if !res.Ok {
			log.Printf("reup: đăng %s thất bại: %s", pub.Name(), res.Error)
			continue
		}
		_ = s.Reup.SetPostTarget(post.ID, username, pub.Name())
		_ = s.Reup.MarkPostPosted(post.ID, pub.Name(), res.RemoteID, res.URL)
		// Warm-up: ghi ngày đăng đầu.
		if cur, _ := s.Settings.Get(KeyReupWarmupStart); cur == "" {
			_ = s.Settings.Set(KeyReupWarmupStart, time.Now().Format("2006-01-02"))
		}
		where := pub.Name()
		if res.Draft {
			where += " (nháp)"
		}
		return fmt.Sprintf("reup post #%d: đã đăng %s", post.ID, where), true
	}
	_ = s.Reup.SetPostStatus(post.ID, reup.PostFailed, "không publisher nào đăng được (kiểm tra OAuth/kết nối)")
	return fmt.Sprintf("reup post #%d: đăng thất bại — kiểm tra OAuth/kết nối kênh", post.ID), false
}

// ------------------------------------------------------------ kill tick

// ReupKillTick đồng bộ view thật + đánh giá kill rule 0-view.
func (s *Service) ReupKillTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyReupKillEnabled, true) {
		return nil
	}
	if s.Reup == nil || s.Growth == nil {
		return nil
	}
	if !dueSince(s.Settings, KeyReupKillLastRun, killTickEvery) {
		return nil
	}
	stampRun(s.Settings, KeyReupKillLastRun)

	// 1. Đồng bộ view thật cho bài YouTube (TikTok: "chờ số liệu").
	s.syncReupMetrics(ctx)

	// 2. Đánh giá kill rule.
	posts, err := s.Reup.ListPostedPosts(100)
	if err != nil {
		return []string{"reup kill: đọc bài đã đăng lỗi: " + err.Error()}
	}
	var views []growth.ReupPostView
	for _, p := range posts {
		ts, _ := time.Parse(time.RFC3339, p.PostedAt)
		views = append(views, growth.ReupPostView{
			PostID: p.ID, Views: p.Views, PostedAt: ts,
		})
	}
	n := atInt(s.Settings, KeyReupKillZeroN, growth.DefaultReupZeroViewN)
	kill, reason, evaluated, skipped := growth.EvaluateReupZeroView(views, n)
	if evaluated == 0 {
		return []string{fmt.Sprintf("reup kill: chờ số liệu (%d bài chưa có view thật) — chưa đánh giá", skipped)}
	}
	if !kill {
		return []string{fmt.Sprintf("reup kill: ổn (%d bài có số liệu, %d chờ số liệu)", evaluated, skipped)}
	}
	// 3. KILL: dừng đăng reup + alert + ledger.
	_ = s.Settings.Set(KeyReupPostEnabled, "0")
	_ = s.Growth.InsertAlert(growth.Alert{
		Severity:    "warn",
		Kind:        "reup_kill",
		Message:     reason,
		ActionTaken: "Đã TẮT tự đăng reup (reup.post_enabled=0). Bài đã đăng giữ nguyên, không xoá.",
	})
	_ = s.decide("reup_kill", "stop_reup_posting", nil, reason, map[string]any{
		"threshold": n, "evaluated": evaluated, "skipped": skipped,
	})
	return []string{"reup kill: KÍCH HOẠT — " + reason}
}

// syncReupMetrics đồng bộ view thật cho bài YouTube đã đăng (videos.list).
func (s *Service) syncReupMetrics(ctx context.Context) {
	if s.Env == nil {
		return
	}
	apiKey, ok := s.Env.EffectiveEnv("YOUTUBE_API_KEY")
	if !ok || strings.TrimSpace(apiKey) == "" {
		return // chưa nối YouTube → bài YouTube ở "chờ số liệu", trung thực
	}
	posts, err := s.Reup.ListPostedPosts(100)
	if err != nil {
		return
	}
	for _, p := range posts {
		if p.Platform != "youtube" || p.RemoteID == "" || p.Views >= 0 {
			continue
		}
		v, err := growth.FetchYouTubeVideoViews(ctx, apiKey, p.RemoteID)
		if err != nil {
			log.Printf("reup: sync view bài #%d: %v", p.ID, err)
			continue
		}
		_ = s.Reup.SetPostMetrics(p.ID, v)
	}
}

// stringSetting đọc setting dạng chuỗi (trống khi lỗi).
func stringSetting(st Settings, key string) string {
	if st == nil {
		return ""
	}
	v, _ := st.Get(key)
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
