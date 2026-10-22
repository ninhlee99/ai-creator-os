package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func AnalyzeTikTokVideoURL(ctx context.Context, rawURL string) (*TikTokTrendVideo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("vui lòng cung cấp link video TikTok")
	}

	apiURL := fmt.Sprintf("https://www.tikwm.com/api/?url=%s", strings.TrimSpace(rawURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối TikTok API: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			PlayCount int64  `json:"play_count"`
			DiggCount int64  `json:"digg_count"`
			Cover     string `json:"cover"`
			Play      string `json:"play"`
			Author    struct {
				Nickname string `json:"nickname"`
				UniqueID string `json:"unique_id"`
			} `json:"author"`
			MusicInfo struct {
				Title string `json:"title"`
			} `json:"music_info"`
		} `json:"data"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, fmt.Errorf("lỗi đọc dữ liệu TikTok: %w", err)
	}

	if payload.Code != 0 || payload.Data.ID == "" {
		return nil, fmt.Errorf("không tìm thấy video TikTok này (mã lỗi: %s)", payload.Msg)
	}

	title := payload.Data.Title
	if title == "" {
		title = fmt.Sprintf("Video của @%s", payload.Data.Author.UniqueID)
	}

	return &TikTokTrendVideo{
		VideoID:       payload.Data.ID,
		Title:         title,
		Author:        payload.Data.Author.Nickname,
		UniqueID:      payload.Data.Author.UniqueID,
		PlayCount:     payload.Data.PlayCount,
		Views:         FormatCount(payload.Data.PlayCount),
		DiggCount:     payload.Data.DiggCount,
		Likes:         FormatCount(payload.Data.DiggCount),
		Music:         payload.Data.MusicInfo.Title,
		VideoURL:      fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", payload.Data.Author.UniqueID, payload.Data.ID),
		CoverURL:      payload.Data.Cover,
		PlayURL:       payload.Data.Play,
		SuggestedHook: fmt.Sprintf("Video @%s: %s", payload.Data.Author.UniqueID, title),
	}, nil
}
