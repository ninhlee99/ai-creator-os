package reup

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Discoverer: tìm video viral từ nguồn Douyin đang bật.
//
//   - Nguồn user: lấy danh sách video mới qua TikWM /api/user/posts →
//     auto-pick theo play_count cao nhất, bỏ qua video đã có (douyin_id).
//   - Nguồn hashtag: TikWM không có endpoint list hashtag ổn định →
//     KHÔNG bịa danh sách. Trả note trung thực "nhập URL trực tiếp"
//     (UI có sẵn ô nhập URL).
//   - Video pick được lưu vào kho ở trạng thái queued — tải thật do
//     Downloader (tick hoặc nút "Quét ngay").
// ---------------------------------------------------------------------------

// Candidate là video được pick để tải.
// URL luôn là link share gốc (douyin.com/video/<id>) — Downloader tự
// thử yt-dlp trước (giữ watermark), gãy → TikWM Lookup lấy link wmplay
// (có watermark). Không bao giờ nhúng link CDN no-watermark vào kho.
type Candidate struct {
	SourceID  int64
	DouyinID  string
	URL       string
	PlayCount int64
	DiggCount int64
	Duration  float64
	Author    string
	Title     string
}

// Discoverer tìm video mới từ nguồn.
type Discoverer struct {
	Store *Store
	TikWM *TikWM
}

// NewDiscoverer tạo discoverer.
func NewDiscoverer(store *Store, tw *TikWM) *Discoverer {
	return &Discoverer{Store: store, TikWM: tw}
}

// Discover quét mọi nguồn đang bật, trả về candidates đã pick + notes
// trung thực (nguồn nào không quét được thì ghi rõ lý do).
func (d *Discoverer) Discover(ctx context.Context, perSource int) ([]Candidate, []string) {
	var out []Candidate
	var notes []string
	if d.Store == nil {
		return nil, []string{"kho reup chưa sẵn sàng"}
	}
	if perSource <= 0 {
		perSource = 3
	}
	srcs, err := d.Store.EnabledSources()
	if err != nil {
		return nil, []string{"đọc nguồn: " + err.Error()}
	}
	if len(srcs) == 0 {
		return nil, []string{"chưa có nguồn Douyin đang bật — thêm nguồn ở trang Reup"}
	}
	for _, src := range srcs {
		cands, note := d.discoverSource(ctx, src, perSource)
		if note != "" {
			notes = append(notes, note)
		}
		out = append(out, cands...)
	}
	return out, notes
}

// discoverSource quét 1 nguồn.
func (d *Discoverer) discoverSource(ctx context.Context, src Source, perSource int) ([]Candidate, string) {
	switch src.Kind {
	case "user":
		return d.discoverUser(ctx, src, perSource)
	case "hashtag":
		// Trung thực: chưa có đường lấy list hashtag tự động ổn định.
		return nil, fmt.Sprintf("nguồn #%s (%s): chưa tự lấy được danh sách video theo hashtag — nhập URL trực tiếp ở trang Reup",
			shortID(src.ID), src.Value)
	default:
		return nil, fmt.Sprintf("nguồn #%s: loại %q không hỗ trợ", shortID(src.ID), src.Kind)
	}
}

// discoverUser lấy video của user qua TikWM, pick theo điểm engagement
// (không chỉ lượt xem thuần — video nhiều like/view hơn thường "chất"
// hơn), bỏ qua video quá ngắn/dài. Video pick được lưu vào kho ở trạng
// thái queued — tải thật do Downloader (tick hoặc nút "Quét ngay").
func (d *Discoverer) discoverUser(ctx context.Context, src Source, perSource int) ([]Candidate, string) {
	if d.TikWM == nil {
		return nil, fmt.Sprintf("nguồn @%s: TikWM chưa sẵn sàng", src.Value)
	}
	videos, err := d.TikWM.UserPosts(ctx, strings.TrimPrefix(src.Value, "@"), 30)
	if err != nil {
		return nil, fmt.Sprintf("nguồn @%s: không lấy được danh sách video (%v)", src.Value, err)
	}
	// Bỏ video đã có trong kho + video độ dài không phù hợp transform
	// (quá ngắn <5s không đủ làm bài; quá dài >180s tốn quota mà Shorts
	// chỉ cần ≤60s — giữ 180s cho linh hoạt).
	var fresh []TikWMVideo
	for _, v := range videos {
		if d.Store.HasDouyinID(v.ID) {
			continue
		}
		if v.Duration > 0 && (v.Duration < 5 || v.Duration > 180) {
			continue
		}
		fresh = append(fresh, v)
	}
	if len(fresh) == 0 {
		return nil, ""
	}
	// Auto-pick theo điểm engagement cao nhất.
	sort.Slice(fresh, func(i, j int) bool {
		return engagementScore(fresh[i]) > engagementScore(fresh[j])
	})
	if len(fresh) > perSource {
		fresh = fresh[:perSource]
	}
	var out []Candidate
	for _, v := range fresh {
		author := v.Nickname
		if author == "" {
			author = v.UniqueID
		}
		// Luôn queue URL share gốc — link tải do Downloader tự lấy
		// (yt-dlp giữ watermark; TikWM chỉ dùng wmplay có watermark).
		url := "https://www.douyin.com/video/" + v.ID
		c := Candidate{
			SourceID: src.ID, DouyinID: v.ID, URL: url,
			PlayCount: v.PlayCount, DiggCount: v.DiggCount,
			Duration: v.Duration, Author: author, Title: v.Title,
		}
		if _, qerr := d.Store.QueueVideo(Video{
			SourceID: src.ID, DouyinID: v.ID, URL: url,
			Title: v.Title, Author: author, PlayCount: v.PlayCount,
			Status: StatusQueued,
		}); qerr != nil {
			log.Printf("reup: queue video %s: %v", v.ID, qerr)
			continue
		}
		out = append(out, c)
	}
	return out, ""
}

func shortID(id int64) string { return fmt.Sprint(id) }

// engagementScore chấm điểm video để pick: lượt xem nhân với
// (1 + tỷ lệ like/view). Video ít view nhưng tỷ lệ like cao (nội dung
// "chất", đang lên) được ưu tiên hơn video view cao nhưng like thấp.
// Pure function — test được không cần mạng.
func engagementScore(v TikWMVideo) float64 {
	if v.PlayCount <= 0 {
		return 0
	}
	likeRate := float64(v.DiggCount) / float64(v.PlayCount)
	if likeRate < 0 {
		likeRate = 0
	}
	return float64(v.PlayCount) * (1 + likeRate)
}
