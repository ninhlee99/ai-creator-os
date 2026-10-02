package reup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ---------------------------------------------------------------------------
// TikWM fallback — API bên thứ ba https://www.tikwm.com, KHÔNG SLA.
//
//   - Public, không cần key. Trả link no-watermark (play/hdplay) + metadata.
//   - Link CDN có chữ ký, HẾT HẠN SAU VÀI PHÚT → caller phải tải ngay,
//     không lưu link để dùng sau.
//   - Có thể die/thay đổi format bất cứ lúc nào — parse defensively,
//     lỗi nào cũng trả về error rõ ràng (không đoán).
//
// Format response theo TikWM v2 (đã kiểm chứng qua tài liệu cộng đồng
// 2026; nếu TikWM đổi field, Lookup trả lỗi thay vì bịa dữ liệu).
// ---------------------------------------------------------------------------

// defaultTikWMBase là API TikWM chính thức.
const defaultTikWMBase = "https://www.tikwm.com"

// tikwmTimeout là timeout mỗi lần gọi TikWM.
const tikwmTimeout = 30 * time.Second

// TikWM gọi API tikwm.com.
type TikWM struct {
	// BaseURL cho phép trỏ sang server mock trong test.
	BaseURL string
	http    *http.Client
}

// NewTikWM tạo client TikWM.
func NewTikWM() *TikWM {
	return &TikWM{BaseURL: defaultTikWMBase, http: &http.Client{Timeout: tikwmTimeout}}
}

// TikWMVideo là metadata + link tải của một video.
type TikWMVideo struct {
	ID        string // video_id Douyin
	Title     string
	Cover     string
	PlayURL   string // no-watermark
	HDPlayURL string // no-watermark HD
	WMPlayURL string // có watermark
	Duration  float64
	PlayCount int64
	DiggCount int64
	AuthorID  string
	UniqueID  string
	Nickname  string
}

// tikwmEnvelope là khung response chung của TikWM.
type tikwmEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// tikwmVideoData là data của endpoint ?url= (một video).
type tikwmVideoData struct {
	ID        string       `json:"id"`
	VideoID   string       `json:"video_id"`
	Title     string       `json:"title"`
	Cover     string       `json:"cover"`
	Play      string       `json:"play"`
	HDPlay    string       `json:"hdplay"`
	WMPlay    string       `json:"wmplay"`
	Duration  float64      `json:"duration"`
	PlayCount int64        `json:"play_count"`
	DiggCount int64        `json:"digg_count"`
	Author    *tikwmAuthor `json:"author"`
}

type tikwmAuthor struct {
	ID       string `json:"id"`
	UniqueID string `json:"unique_id"`
	Nickname string `json:"nickname"`
}

// tikwmPostsData là data của /api/user/posts.
type tikwmPostsData struct {
	Videos []struct {
		VideoID   string       `json:"video_id"`
		ID        string       `json:"id"`
		Title     string       `json:"title"`
		Cover     string       `json:"cover"`
		Play      string       `json:"play"`
		HDPlay    string       `json:"hdplay"`
		Duration  float64      `json:"duration"`
		PlayCount int64        `json:"play_count"`
		DiggCount int64        `json:"digg_count"`
		Author    *tikwmAuthor `json:"author"`
	} `json:"videos"`
	Cursor  int64 `json:"cursor"`
	HasMore bool  `json:"hasMore"`
}

// get gọi GET và parse envelope (code phải = 0).
func (t *TikWM) get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	base := t.BaseURL
	if base == "" {
		base = defaultTikWMBase
	}
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("tikwm: tạo request: %w", err)
	}
	req.Header.Set("User-Agent", "aicos-reup/1.0")
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tikwm: gọi API thất bại: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tikwm: HTTP %d (bên thứ ba, không SLA)", resp.StatusCode)
	}
	var env tikwmEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("tikwm: parse response: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("tikwm: API báo lỗi (code=%d): %s", env.Code, env.Msg)
	}
	return env.Data, nil
}

// Lookup tra metadata + link no-watermark của 1 URL video Douyin.
func (t *TikWM) Lookup(ctx context.Context, shareURL string) (*TikWMVideo, error) {
	data, err := t.get(ctx, "/api/", url.Values{"url": {shareURL}})
	if err != nil {
		return nil, err
	}
	var d tikwmVideoData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("tikwm: parse video: %w", err)
	}
	id := d.VideoID
	if id == "" {
		id = d.ID
	}
	if id == "" {
		return nil, fmt.Errorf("tikwm: response thiếu video id")
	}
	if d.Play == "" {
		return nil, fmt.Errorf("tikwm: response thiếu link play (no-watermark)")
	}
	v := &TikWMVideo{
		ID: id, Title: d.Title, Cover: d.Cover,
		PlayURL: d.Play, HDPlayURL: d.HDPlay, WMPlayURL: d.WMPlay,
		Duration: d.Duration, PlayCount: d.PlayCount, DiggCount: d.DiggCount,
	}
	if d.Author != nil {
		v.AuthorID, v.UniqueID, v.Nickname = d.Author.ID, d.Author.UniqueID, d.Author.Nickname
	}
	return v, nil
}

// UserPosts lấy danh sách video mới nhất của 1 user Douyin (unique_id,
// không @). count tối đa mỗi lần gọi (TikWM giới hạn ~30).
func (t *TikWM) UserPosts(ctx context.Context, uniqueID string, count int) ([]TikWMVideo, error) {
	if count <= 0 || count > 30 {
		count = 30
	}
	data, err := t.get(ctx, "/api/user/posts", url.Values{
		"unique_id": {uniqueID},
		"count":     {fmt.Sprint(count)},
		"cursor":    {"0"},
	})
	if err != nil {
		return nil, err
	}
	var d tikwmPostsData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("tikwm: parse user posts: %w", err)
	}
	var out []TikWMVideo
	for _, p := range d.Videos {
		id := p.VideoID
		if id == "" {
			id = p.ID
		}
		if id == "" {
			continue // bỏ qua entry thiếu id — không đoán
		}
		v := TikWMVideo{
			ID: id, Title: p.Title, Cover: p.Cover,
			PlayURL: p.Play, HDPlayURL: p.HDPlay,
			Duration: p.Duration, PlayCount: p.PlayCount, DiggCount: p.DiggCount,
		}
		if p.Author != nil {
			v.AuthorID, v.UniqueID, v.Nickname = p.Author.ID, p.Author.UniqueID, p.Author.Nickname
		}
		out = append(out, v)
	}
	return out, nil
}
