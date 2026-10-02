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
type Candidate struct {
	SourceID  int64
	DouyinID  string
	URL       string
	DirectURL bool // true: URL đã là link file trực tiếp (TikWM play) —
	// downloader tải thẳng, không Lookup lại
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

// discoverUser lấy video của user qua TikWM, pick top play_count chưa có.
func (d *Discoverer) discoverUser(ctx context.Context, src Source, perSource int) ([]Candidate, string) {
	if d.TikWM == nil {
		return nil, fmt.Sprintf("nguồn @%s: TikWM chưa sẵn sàng", src.Value)
	}
	videos, err := d.TikWM.UserPosts(ctx, strings.TrimPrefix(src.Value, "@"), 30)
	if err != nil {
		return nil, fmt.Sprintf("nguồn @%s: không lấy được danh sách video (%v)", src.Value, err)
	}
	// Bỏ video đã có trong kho.
	var fresh []TikWMVideo
	for _, v := range videos {
		if !d.Store.HasDouyinID(v.ID) {
			fresh = append(fresh, v)
		}
	}
	if len(fresh) == 0 {
		return nil, ""
	}
	// Auto-pick theo play_count cao nhất.
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].PlayCount > fresh[j].PlayCount })
	if len(fresh) > perSource {
		fresh = fresh[:perSource]
	}
	var out []Candidate
	for _, v := range fresh {
		author := v.Nickname
		if author == "" {
			author = v.UniqueID
		}
		url := v.PlayURL
		direct := true
		if url == "" {
			url = v.HDPlayURL
		}
		if url == "" {
			// Không có link tải trực tiếp — vẫn queue theo URL gốc để
			// Downloader thử yt-dlp rồi TikWM Lookup.
			url = "https://www.douyin.com/video/" + v.ID
			direct = false
		}
		c := Candidate{
			SourceID: src.ID, DouyinID: v.ID, URL: url, DirectURL: direct,
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
