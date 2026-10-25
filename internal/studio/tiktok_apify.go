package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	TikTokApifySource        = "Apify TikTok Scraper"
	tikTokApifyActorID       = "clockworks~tiktok-scraper"
	tikTokApifyAPIBaseURL    = "https://api.apify.com/v2/actors/"
	tikTokApifyPerTopic      = 20
	tikTokApifyMaxRunTime    = 120 * time.Second
	tikTokApifyResponseLimit = 4 << 20
)

type tikTokApifyConfig struct {
	APIToken string
}

func tikTokApifyConfigFromEnv() tikTokApifyConfig {
	return tikTokApifyConfig{APIToken: strings.TrimSpace(os.Getenv("APIFY_API_TOKEN"))}
}

type tikTokApifyInput struct {
	SearchQueries        []string `json:"searchQueries"`
	SearchSection        string   `json:"searchSection"`
	ResultsPerPage       int      `json:"resultsPerPage"`
	ProxyCountryCode     string   `json:"proxyCountryCode"`
	ShouldDownloadVideos bool     `json:"shouldDownloadVideos"`
}

type tikTokApifyVideo struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	WebVideoURL  string `json:"webVideoUrl"`
	PlayCount    *int64 `json:"playCount"`
	DiggCount    *int64 `json:"diggCount"`
	CommentCount *int64 `json:"commentCount"`
	ShareCount   *int64 `json:"shareCount"`
	AuthorMeta   struct {
		Name     string `json:"name"`
		NickName string `json:"nickName"`
	} `json:"authorMeta"`
	MusicMeta struct {
		MusicName string `json:"musicName"`
	} `json:"musicMeta"`
	VideoMeta struct {
		CoverURL string `json:"coverUrl"`
	} `json:"videoMeta"`
	AuthorNameFlat string `json:"authorMeta.name"`
	AuthorNickFlat string `json:"authorMeta.nickName"`
	MusicNameFlat  string `json:"musicMeta.musicName"`
	CoverURLFlat   string `json:"videoMeta.coverUrl"`
}

func fetchApifyTikTokVideos(ctx context.Context, topics []string, cfg tikTokApifyConfig) ([]TikTokTrendVideo, error) {
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("chưa cấu hình Apify API token")
	}
	if len(topics) == 0 || len(topics) > tikTokRadarMaxTopics {
		return nil, fmt.Errorf("Apify tìm tối đa %d chủ đề mỗi lần", tikTokRadarMaxTopics)
	}

	ctx, cancel := context.WithTimeout(ctx, tikTokApifyMaxRunTime)
	defer cancel()

	input := tikTokApifyInput{
		SearchQueries:        topics,
		SearchSection:        "/video",
		ResultsPerPage:       tikTokApifyPerTopic,
		ProxyCountryCode:     "None",
		ShouldDownloadVideos: false,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("không tạo được yêu cầu tìm kiếm Apify: %w", err)
	}

	query := url.Values{}
	query.Set("format", "json")
	query.Set("limit", fmt.Sprint(len(topics)*tikTokApifyPerTopic))
	query.Set("timeout", fmt.Sprint(int(tikTokApifyMaxRunTime.Seconds())))
	endpoint := tikTokApifyAPIBaseURL + tikTokApifyActorID + "/run-sync-get-dataset-items?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: tikTokApifyMaxRunTime, Transport: tikTokRadarHTTPTransport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("không gọi được Apify TikTok Scraper: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apifyTikTokHTTPError(resp.StatusCode)
	}

	var records []tikTokApifyVideo
	if err := json.NewDecoder(io.LimitReader(resp.Body, tikTokApifyResponseLimit)).Decode(&records); err != nil {
		return nil, fmt.Errorf("Apify TikTok Scraper trả dữ liệu không hợp lệ: %w", err)
	}
	videos := make([]TikTokTrendVideo, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		id := strings.TrimSpace(record.ID)
		videoURL := strings.TrimSpace(record.WebVideoURL)
		if id == "" {
			id = TikTokVideoIDFromURL(videoURL)
		}
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if videoURL == "" {
			videoURL = "https://www.tiktok.com/video/" + id
		}

		views, viewsKnown := apifyCount(record.PlayCount)
		likes, likesKnown := apifyCount(record.DiggCount)
		comments, commentsKnown := apifyCount(record.CommentCount)
		shares, sharesKnown := apifyCount(record.ShareCount)
		uniqueID := firstNonEmpty(record.AuthorMeta.Name, record.AuthorNameFlat)
		author := firstNonEmpty(record.AuthorMeta.NickName, record.AuthorNickFlat, uniqueID)
		music := firstNonEmpty(record.MusicMeta.MusicName, record.MusicNameFlat)
		coverURL := firstNonEmpty(record.VideoMeta.CoverURL, record.CoverURLFlat)
		caption := strings.TrimSpace(record.Text)
		if caption == "" {
			caption = "Video TikTok " + id
		}

		video := TikTokTrendVideo{
			Rank:            len(videos) + 1,
			VideoID:         id,
			Title:           caption,
			Caption:         strings.TrimSpace(record.Text),
			Author:          author,
			UniqueID:        uniqueID,
			PlayCount:       views,
			PlayCountKnown:  viewsKnown,
			Views:           FormatCount(views),
			Provider:        TikTokApifySource,
			Likes:           FormatCount(likes),
			DiggCount:       likes,
			DiggCountKnown:  likesKnown,
			Comments:        FormatCount(comments),
			CommentCount:    comments,
			CommentKnown:    commentsKnown,
			Shares:          FormatCount(shares),
			ShareCount:      shares,
			ShareCountKnown: sharesKnown,
			Music:           music,
			VideoURL:        videoURL,
			CoverURL:        coverURL,
		}
		videos = append(videos, video)
	}
	if len(videos) == 0 {
		return nil, fmt.Errorf("Apify không trả video TikTok cho chủ đề đã nhập; thử từ khóa khác")
	}
	return videos, nil
}

func apifyCount(value *int64) (int64, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func apifyTikTokHTTPError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("Apify từ chối token (HTTP %d); kiểm tra API token trong Cài đặt → Kênh mạng xã hội", status)
	case http.StatusPaymentRequired:
		return fmt.Errorf("Apify đã chạm giới hạn credit/chi tiêu (HTTP %d); kiểm tra tài khoản Apify", status)
	case http.StatusForbidden:
		return fmt.Errorf("tài khoản Apify không có quyền chạy TikTok Scraper (HTTP %d)", status)
	case http.StatusRequestTimeout:
		return fmt.Errorf("Apify tìm kiếm quá thời gian chờ (HTTP %d); thử lại sau", status)
	case http.StatusTooManyRequests:
		return fmt.Errorf("Apify đang giới hạn số lần tìm kiếm (HTTP %d); thử lại sau", status)
	default:
		return fmt.Errorf("Apify TikTok Scraper trả HTTP %d", status)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
