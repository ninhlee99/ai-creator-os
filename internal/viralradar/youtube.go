// Package viralradar discovers recent high-view YouTube videos from channels
// with a public subscriber count above the configured threshold.
package viralradar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultSubscriberThreshold int64 = 100_000

var isoDurationPattern = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)

type Source struct {
	VideoID         string    `json:"video_id"`
	Title           string    `json:"title"`
	ChannelID       string    `json:"channel_id"`
	ChannelTitle    string    `json:"channel_title"`
	URL             string    `json:"url"`
	ThumbnailURL    string    `json:"thumbnail_url,omitempty"`
	PublishedAt     time.Time `json:"published_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Views           int64     `json:"views"`
	Subscribers     int64     `json:"subscribers"`
}

type YouTube struct {
	APIKey  string
	Client  *http.Client
	BaseURL string
}

func NewYouTube(apiKey string) *YouTube {
	return &YouTube{APIKey: strings.TrimSpace(apiKey)}
}

func (y *YouTube) Discover(ctx context.Context, query string) ([]Source, error) {
	if y == nil || y.APIKey == "" {
		return nil, fmt.Errorf("YouTube Data API key is missing; add YOUTUBE_API_KEY in /settings/system")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is empty")
	}

	searchParams := url.Values{
		"part":              {"snippet"},
		"type":              {"video"},
		"order":             {"viewCount"},
		"q":                 {query},
		"regionCode":        {"VN"},
		"relevanceLanguage": {"vi"},
		"publishedAfter":    {time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)},
		"maxResults":        {"50"},
		"key":               {y.APIKey},
	}
	var search struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				ChannelID  string `json:"channelId"`
				Channel    string `json:"channelTitle"`
				Published  string `json:"publishedAt"`
				Thumbnails struct {
					High struct {
						URL string `json:"url"`
					} `json:"high"`
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := y.get(ctx, "/youtube/v3/search", searchParams, &search); err != nil {
		return nil, err
	}
	if len(search.Items) == 0 {
		return []Source{}, nil
	}

	ids := make([]string, 0, len(search.Items))
	channelSet := make(map[string]struct{}, len(search.Items))
	byID := make(map[string]Source, len(search.Items))
	for _, item := range search.Items {
		videoID := strings.TrimSpace(item.ID.VideoID)
		channelID := strings.TrimSpace(item.Snippet.ChannelID)
		if videoID == "" || channelID == "" {
			continue
		}
		published, _ := time.Parse(time.RFC3339, item.Snippet.Published)
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Medium.URL
		}
		byID[videoID] = Source{
			VideoID: videoID, Title: item.Snippet.Title,
			ChannelID: channelID, ChannelTitle: item.Snippet.Channel,
			URL:          "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID),
			ThumbnailURL: thumb, PublishedAt: published,
		}
		ids = append(ids, videoID)
		channelSet[channelID] = struct{}{}
	}
	if len(ids) == 0 {
		return []Source{}, nil
	}

	var videos struct {
		Items []struct {
			ID             string `json:"id"`
			ContentDetails struct {
				Duration string `json:"duration"`
			} `json:"contentDetails"`
			Statistics struct {
				ViewCount string `json:"viewCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	videoParams := url.Values{"part": {"contentDetails,statistics"}, "id": {strings.Join(ids, ",")}, "key": {y.APIKey}}
	if err := y.get(ctx, "/youtube/v3/videos", videoParams, &videos); err != nil {
		return nil, err
	}
	for _, item := range videos.Items {
		source, ok := byID[item.ID]
		if !ok {
			continue
		}
		source.DurationSeconds = parseISO8601Duration(item.ContentDetails.Duration)
		source.Views, _ = strconv.ParseInt(item.Statistics.ViewCount, 10, 64)
		byID[item.ID] = source
	}

	channelIDs := make([]string, 0, len(channelSet))
	for id := range channelSet {
		channelIDs = append(channelIDs, id)
	}
	sort.Strings(channelIDs)
	var channels struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			Statistics struct {
				SubscriberCount       string `json:"subscriberCount"`
				HiddenSubscriberCount bool   `json:"hiddenSubscriberCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	channelParams := url.Values{"part": {"snippet,statistics"}, "id": {strings.Join(channelIDs, ",")}, "key": {y.APIKey}}
	if err := y.get(ctx, "/youtube/v3/channels", channelParams, &channels); err != nil {
		return nil, err
	}
	channelStats := make(map[string]struct {
		title string
		subs  int64
	}, len(channels.Items))
	for _, item := range channels.Items {
		if item.Statistics.HiddenSubscriberCount {
			continue
		}
		subs, err := strconv.ParseInt(item.Statistics.SubscriberCount, 10, 64)
		if err != nil {
			continue
		}
		channelStats[item.ID] = struct {
			title string
			subs  int64
		}{title: item.Snippet.Title, subs: subs}
	}

	results := make([]Source, 0)
	for _, id := range ids {
		source := byID[id]
		stats, ok := channelStats[source.ChannelID]
		if !ok || stats.subs < DefaultSubscriberThreshold || source.DurationSeconds <= 0 {
			continue
		}
		if stats.title != "" {
			source.ChannelTitle = stats.title
		}
		source.Subscribers = stats.subs
		results = append(results, source)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Views > results[j].Views })
	return results, nil
}

func (y *YouTube) get(ctx context.Context, path string, params url.Values, out any) error {
	base := strings.TrimRight(y.BaseURL, "/")
	if base == "" {
		base = "https://www.googleapis.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	client := y.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("YouTube Data API request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("YouTube Data API HTTP %d: %s", resp.StatusCode, responseSnippet(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("YouTube Data API returned invalid JSON: %w", err)
	}
	return nil
}

func parseISO8601Duration(value string) int {
	m := isoDurationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(m) != 4 {
		return 0
	}
	hours, _ := strconv.Atoi(m[1])
	minutes, _ := strconv.Atoi(m[2])
	seconds, _ := strconv.Atoi(m[3])
	return hours*3600 + minutes*60 + seconds
}

func responseSnippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 250 {
		return s[:250]
	}
	return s
}
