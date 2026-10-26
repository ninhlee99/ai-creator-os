package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const TikTokBusinessDiscoverySource = "TikTok Business Discovery API"

const tikTokBusinessDiscoveryBaseURL = "https://business-api.tiktok.com/open_api/v1.3/discovery"

type tikTokBusinessDiscoveryConfig struct {
	AccessToken  string
	AdvertiserID string
	CountryCode  string
}

func tikTokBusinessDiscoveryConfigFromEnv() (tikTokBusinessDiscoveryConfig, bool, error) {
	cfg := tikTokBusinessDiscoveryConfig{
		AccessToken:  strings.TrimSpace(os.Getenv("TIKTOK_DISCOVERY_ACCESS_TOKEN")),
		AdvertiserID: strings.TrimSpace(os.Getenv("TIKTOK_DISCOVERY_ADVERTISER_ID")),
		CountryCode:  strings.ToUpper(strings.TrimSpace(os.Getenv("TIKTOK_DISCOVERY_COUNTRY_CODE"))),
	}
	configured := cfg.AccessToken != "" || cfg.AdvertiserID != ""
	if !configured {
		return cfg, false, nil
	}
	if cfg.AccessToken == "" || cfg.AdvertiserID == "" {
		return cfg, true, fmt.Errorf("TikTok Business Discovery cần cả access token và advertiser ID; cấu hình trong Cài đặt → Kênh mạng xã hội")
	}
	if cfg.CountryCode == "" {
		cfg.CountryCode = "VN"
	}
	if len(cfg.CountryCode) != 2 || cfg.CountryCode[0] < 'A' || cfg.CountryCode[0] > 'Z' || cfg.CountryCode[1] < 'A' || cfg.CountryCode[1] > 'Z' {
		return cfg, true, fmt.Errorf("mã khu vực TikTok Discovery phải gồm 2 chữ cái, ví dụ VN")
	}
	return cfg, true, nil
}

// TikTokRadarSourceLabel reports which provider the current process will use.
func TikTokRadarSourceLabel() string {
	return TikTokRadarSourceLabelForTopic(false)
}

// TikTokRadarSourceConfigured reports whether the automated crawler is ready.
func TikTokRadarSourceConfigured() bool {
	return TikTokCrawlerConfigured()
}

// TikTokRadarSourceLabelForTopic reports the provider selected for a regional
// feed or a user-entered topic search.
func TikTokRadarSourceLabelForTopic(_ bool) string {
	if TikTokCrawlerConfigured() {
		return TikTokCrawlerSource
	}
	return "Chưa tìm thấy trình duyệt để chạy TikTok Crawler"
}

type tikTokBusinessEnvelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

type tikTokBusinessHashtag struct {
	ID             string   `json:"hashtag_id"`
	Name           string   `json:"hashtag_name"`
	TopCountryList []string `json:"top_country_list"`
}

type tikTokBusinessVideo struct {
	ID       string `json:"video_id"`
	ShareURL string `json:"share_url"`
}

// fetchTikTokBusinessDiscoveryVideos uses TikTok's official Discovery API.
// It matches the entered topic against TikTok's current popular hashtags,
// then requests the provider-ranked videos for those hashtags.
func fetchTikTokBusinessDiscoveryVideos(ctx context.Context, topic string, cfg tikTokBusinessDiscoveryConfig) ([]TikTokTrendVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	hashtags, err := fetchTikTokBusinessHashtags(ctx, cfg)
	if err != nil {
		return nil, err
	}
	selected := selectTikTokBusinessHashtags(hashtags, topic, cfg.CountryCode)
	if len(selected) == 0 {
		if strings.TrimSpace(topic) == "" {
			return nil, fmt.Errorf("TikTok Discovery chưa trả hashtag phổ biến cho khu vực %s", cfg.CountryCode)
		}
		return nil, fmt.Errorf("không có hashtag TikTok đang thịnh hành khớp chủ đề %q tại khu vực %s; thử chủ đề khác", strings.TrimSpace(topic), cfg.CountryCode)
	}
	if len(selected) > 10 {
		selected = selected[:10]
	}

	videos, err := fetchTikTokBusinessHashtagVideos(ctx, cfg, selected)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(videos))
	results := make([]TikTokTrendVideo, 0, len(videos))
	for _, item := range videos {
		id := strings.TrimSpace(item.video.ID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}

		videoURL := strings.TrimSpace(item.video.ShareURL)
		if TikTokVideoIDFromURL(videoURL) != id {
			videoURL = "https://www.tiktok.com/video/" + id
		}
		title := strings.TrimSpace(item.hashtag.Name)
		if title != "" && !strings.HasPrefix(title, "#") {
			title = "#" + title
		}
		results = append(results, TikTokTrendVideo{
			VideoID:  id,
			Title:    title,
			Caption:  title,
			Region:   cfg.CountryCode,
			VideoURL: videoURL,
			Provider: TikTokBusinessDiscoverySource,
		})
		if len(results) == tikTokRadarFeedLimit {
			break
		}
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results, nil
}

func fetchTikTokBusinessHashtags(ctx context.Context, cfg tikTokBusinessDiscoveryConfig) ([]tikTokBusinessHashtag, error) {
	query := url.Values{}
	query.Set("advertiser_id", cfg.AdvertiserID)
	query.Set("discovery_type", "HASHTAG")
	query.Set("country_code", cfg.CountryCode)
	query.Set("category_name", "ALL")
	query.Set("date_range", "7DAY")
	var response struct {
		List []tikTokBusinessHashtag `json:"list"`
	}
	if err := fetchTikTokBusinessJSON(ctx, "/trending_list/", query, cfg.AccessToken, &response); err != nil {
		return nil, err
	}
	return response.List, nil
}

type tikTokBusinessTaggedVideo struct {
	hashtag tikTokBusinessHashtag
	video   tikTokBusinessVideo
}

func fetchTikTokBusinessHashtagVideos(ctx context.Context, cfg tikTokBusinessDiscoveryConfig, hashtags []tikTokBusinessHashtag) ([]tikTokBusinessTaggedVideo, error) {
	ids := make([]string, 0, len(hashtags))
	byID := make(map[string]tikTokBusinessHashtag, len(hashtags))
	for _, hashtag := range hashtags {
		id := strings.TrimSpace(hashtag.ID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
		byID[id] = hashtag
	}
	if len(ids) == 0 {
		return nil, nil
	}
	encodedIDs, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("advertiser_id", cfg.AdvertiserID)
	query.Set("discovery_type", "HASHTAG")
	query.Set("hashtag_ids", string(encodedIDs))
	query.Set("country_code", cfg.CountryCode)
	query.Set("date_range", "7DAY")
	var response struct {
		List []struct {
			HashtagID    string                `json:"hashtag_id"`
			HashtagName  string                `json:"hashtag_name"`
			TopVideoList []tikTokBusinessVideo `json:"top_video_list"`
		} `json:"list"`
	}
	if err := fetchTikTokBusinessJSON(ctx, "/video_list/", query, cfg.AccessToken, &response); err != nil {
		return nil, err
	}
	var videos []tikTokBusinessTaggedVideo
	for _, group := range response.List {
		hashtag, ok := byID[group.HashtagID]
		if !ok {
			hashtag = tikTokBusinessHashtag{ID: group.HashtagID, Name: group.HashtagName}
		}
		for _, video := range group.TopVideoList {
			videos = append(videos, tikTokBusinessTaggedVideo{hashtag: hashtag, video: video})
		}
	}
	return videos, nil
}

func fetchTikTokBusinessJSON(ctx context.Context, path string, query url.Values, accessToken string, data any) error {
	endpoint := tikTokBusinessDiscoveryBaseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Access-Token", accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doTikTokRadarRequest(req)
	if err != nil {
		return fmt.Errorf("TikTok Business Discovery không truy cập được: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TikTok Business Discovery trả HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	var envelope tikTokBusinessEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("TikTok Business Discovery trả JSON không hợp lệ: %w", err)
	}
	if envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "yêu cầu bị từ chối"
		}
		if envelope.RequestID != "" {
			return fmt.Errorf("TikTok Business Discovery (%d): %s · request %s", envelope.Code, message, envelope.RequestID)
		}
		return fmt.Errorf("TikTok Business Discovery (%d): %s", envelope.Code, message)
	}
	if err := json.Unmarshal(envelope.Data, data); err != nil {
		return fmt.Errorf("TikTok Business Discovery trả data không hợp lệ: %w", err)
	}
	return nil
}

func selectTikTokBusinessHashtags(hashtags []tikTokBusinessHashtag, topic, countryCode string) []tikTokBusinessHashtag {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	localHashtags := make([]tikTokBusinessHashtag, 0, len(hashtags))
	for _, hashtag := range hashtags {
		if len(hashtag.TopCountryList) == 0 {
			localHashtags = append(localHashtags, hashtag)
			continue
		}
		for _, country := range hashtag.TopCountryList {
			if strings.EqualFold(strings.TrimSpace(country), countryCode) {
				localHashtags = append(localHashtags, hashtag)
				break
			}
		}
	}
	if strings.TrimSpace(topic) == "" {
		if len(localHashtags) > 10 {
			return append([]tikTokBusinessHashtag(nil), localHashtags[:10]...)
		}
		return localHashtags
	}
	topics, err := splitTikTokRadarTopics(topic)
	if err != nil {
		return nil
	}
	topicKeys := make([]string, 0, len(topics))
	for _, value := range topics {
		key := normalizeTikTokRadarTopic(value)
		if len([]rune(key)) >= 2 {
			topicKeys = append(topicKeys, key)
		}
	}
	if len(topicKeys) == 0 {
		return nil
	}
	var selected []tikTokBusinessHashtag
	for _, hashtag := range localHashtags {
		hashtagKey := normalizeTikTokRadarTopic(hashtag.Name)
		for _, topicKey := range topicKeys {
			if strings.Contains(hashtagKey, topicKey) {
				selected = append(selected, hashtag)
				break
			}
		}
	}
	return selected
}

func normalizeTikTokRadarTopic(value string) string {
	value = norm.NFD.String(strings.ToLower(value))
	var normalized strings.Builder
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}
