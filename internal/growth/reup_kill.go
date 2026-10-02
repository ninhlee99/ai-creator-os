package growth

// ReupKill (Đợt E): kill rule 0-view cho pipeline reup — MỞ RỘNG engine,
// không viết lại. Rule: N bài đăng liên tiếp 0-view → dừng đăng reup +
// alert + đề xuất đổi nguồn/phong cách. Không tự xoá bài đã đăng.
//
// TRUNG THỰC:
//   - Rule chỉ đếm bài ĐÃ CÓ số liệu view thật. Bài "chờ số liệu"
//     (views = -1) không được tính là 0 và cũng không reset chuỗi —
//     không bịa số liệu để giết hay để cứu.
//   - Chưa có bài nào có số liệu → rule ở chế độ "chờ số liệu", không
//     hành động (fail-closed).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultReupZeroViewN là ngưỡng mặc định: 5 bài 0-view liên tiếp.
const DefaultReupZeroViewN = 5

// ReupPostView là dữ liệu tối thiểu rule cần cho 1 bài đã đăng.
type ReupPostView struct {
	PostID   int64
	Views    int64 // -1 = chưa có số liệu thật
	PostedAt time.Time
}

// EvaluateReupZeroView đếm chuỗi 0-view liên tiếp (cũ → mới) trên các bài
// ĐÃ CÓ số liệu. Trả về (kill, lý do, số bài đã đánh giá, số bài bỏ qua
// vì chưa có số liệu).
func EvaluateReupZeroView(posts []ReupPostView, n int) (kill bool, reason string, evaluated, skipped int) {
	if n < 1 {
		n = DefaultReupZeroViewN
	}
	sorted := append([]ReupPostView(nil), posts...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PostedAt.Before(sorted[j].PostedAt)
	})
	streak := 0
	for _, p := range sorted {
		if p.Views < 0 {
			skipped++
			continue
		}
		evaluated++
		if p.Views == 0 {
			streak++
		} else {
			streak = 0
		}
	}
	if streak >= n {
		return true, fmt.Sprintf(
			"%d bài reup liên tiếp 0 view (ngưỡng %d) — dừng đăng reup. "+
				"Đề xuất: đổi nguồn Douyin hoặc đổi phong cách transform/voiceover.",
			streak, n), evaluated, skipped
	}
	return false, "", evaluated, skipped
}

// ---------------------------------------------------------------------------
// Metrics thật cho bài reup: YouTube Data API v3 (videos.list).
// TikTok chưa có nguồn metrics (Display API cần OAuth + audit) — bài đăng
// TikTok ở trạng thái "chờ số liệu", rule không đếm chúng.
// ---------------------------------------------------------------------------

// youtubeAPIBase là base URL YouTube Data API — var để test override
// bằng httptest (production: https://www.googleapis.com).
var youtubeAPIBase = "https://www.googleapis.com"

// FetchYouTubeVideoViews đọc viewCount thật của 1 video qua YouTube Data
// API v3. Không key → ErrNotConnected (fail-closed, không bịa số).
func FetchYouTubeVideoViews(ctx context.Context, apiKey, videoID string) (int64, error) {
	if strings.TrimSpace(apiKey) == "" {
		return -1, fmt.Errorf("%w: YouTube (chưa nhập YOUTUBE_API_KEY trong Cài đặt)", ErrNotConnected)
	}
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return -1, fmt.Errorf("reup kill: thiếu video id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(youtubeAPIBase, "/")+"/youtube/v3/videos?part=statistics&id="+
			videoID+"&key="+apiKey, nil)
	if err != nil {
		return -1, err
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return -1, fmt.Errorf("youtube videos.list: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return -1, err
	}
	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("youtube videos.list: HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	var parsed struct {
		Items []struct {
			Statistics struct {
				ViewCount string `json:"viewCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return -1, fmt.Errorf("youtube videos.list: phản hồi xấu: %w", err)
	}
	if len(parsed.Items) == 0 {
		return -1, fmt.Errorf("youtube videos.list: không tìm thấy video %q", videoID)
	}
	n, err := strconv.ParseInt(parsed.Items[0].Statistics.ViewCount, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("youtube videos.list: viewCount không phải số: %w", err)
	}
	return n, nil
}
