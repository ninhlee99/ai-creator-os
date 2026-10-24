package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxDisplayVideoPages = 3

type DisplayUserStats struct {
	Followers *int64
	Videos    *int64
	Likes     *int64
}

type DisplayVideoMetrics struct {
	ID          string
	Title       string
	ShareURL    string
	PublishedAt time.Time
	Views       *int64
	Likes       *int64
	Comments    *int64
	Shares      *int64
}

// DisplayUserStats reads the authorized creator's public profile statistics.
func (c *PostingClient) DisplayUserStats(ctx context.Context) (DisplayUserStats, error) {
	var out DisplayUserStats
	fields := url.Values{}
	fields.Set("fields", "follower_count,video_count,likes_count")
	raw, err := c.displayRequest(ctx, http.MethodGet, "/v2/user/info/?"+fields.Encode(), nil)
	if err != nil {
		return out, err
	}
	if err := checkDisplayAPIError(raw); err != nil {
		return out, err
	}
	var response struct {
		Data struct {
			User struct {
				Followers *int64 `json:"follower_count"`
				Videos    *int64 `json:"video_count"`
				Likes     *int64 `json:"likes_count"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return out, fmt.Errorf("tiktok display: response user info không hợp lệ")
	}
	return DisplayUserStats{
		Followers: response.Data.User.Followers,
		Videos:    response.Data.User.Videos,
		Likes:     response.Data.User.Likes,
	}, nil
}

// ListDisplayVideoMetrics reads recent public creator videos and their current
// lifetime counters. TikTok Display API does not return retention or audience
// demographics through this endpoint.
func (c *PostingClient) ListDisplayVideoMetrics(ctx context.Context, limit int) ([]DisplayVideoMetrics, error) {
	if limit <= 0 || limit > maxDisplayVideoPages*20 {
		limit = maxDisplayVideoPages * 20
	}
	fields := url.Values{}
	fields.Set("fields", "id,create_time,share_url,title,video_description,view_count,like_count,comment_count,share_count")
	endpoint := "/v2/video/list/?" + fields.Encode()
	var out []DisplayVideoMetrics
	seen := make(map[string]bool)
	var cursor int64
	for page := 0; page < maxDisplayVideoPages && len(out) < limit; page++ {
		payload := map[string]any{"max_count": 20, "cursor": cursor}
		raw, err := c.displayRequest(ctx, http.MethodPost, endpoint, payload)
		if err != nil {
			return nil, err
		}
		if err := checkDisplayAPIError(raw); err != nil {
			return nil, err
		}
		var response struct {
			Data struct {
				Videos []struct {
					ID          string `json:"id"`
					CreateTime  int64  `json:"create_time"`
					ShareURL    string `json:"share_url"`
					Title       string `json:"title"`
					Description string `json:"video_description"`
					Views       *int64 `json:"view_count"`
					Likes       *int64 `json:"like_count"`
					Comments    *int64 `json:"comment_count"`
					Shares      *int64 `json:"share_count"`
				} `json:"videos"`
				Cursor  int64 `json:"cursor"`
				HasMore bool  `json:"has_more"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return nil, fmt.Errorf("tiktok display: response video list không hợp lệ")
		}
		for _, video := range response.Data.Videos {
			id := strings.TrimSpace(video.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			title := strings.TrimSpace(video.Title)
			if title == "" {
				title = strings.TrimSpace(video.Description)
			}
			var published time.Time
			if video.CreateTime > 0 {
				published = time.Unix(video.CreateTime, 0).UTC()
			}
			out = append(out, DisplayVideoMetrics{
				ID: id, Title: title, ShareURL: strings.TrimSpace(video.ShareURL),
				PublishedAt: published, Views: video.Views, Likes: video.Likes,
				Comments: video.Comments, Shares: video.Shares,
			})
			if len(out) >= limit {
				break
			}
		}
		if !response.Data.HasMore || response.Data.Cursor == cursor {
			break
		}
		cursor = response.Data.Cursor
	}
	return out, nil
}

func (c *PostingClient) displayRequest(ctx context.Context, method, endpoint string, payload any) ([]byte, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, fmt.Errorf("tiktok display: không lấy được access token: %w", err)
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	if method == http.MethodPost {
		headers["Content-Type"] = "application/json; charset=UTF-8"
	}
	if c.HTTP != nil {
		status, raw, err := c.HTTP(method, Base+endpoint, headers, body)
		if err != nil {
			return nil, err
		}
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("tiktok display: HTTP %d", status)
		}
		return raw, nil
	}
	var requestBody io.Reader
	if payload != nil {
		requestBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, Base+endpoint, requestBody)
	if err != nil {
		return nil, err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: apiHTTPTransport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("tiktok display: HTTP %d", resp.StatusCode)
	}
	return raw, nil
}

func checkDisplayAPIError(raw []byte) error {
	var response struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("tiktok display: response không phải JSON hợp lệ")
	}
	code := strings.TrimSpace(string(response.Error.Code))
	if code == "" || code == `"ok"` || code == `"0"` || code == "0" || code == "null" {
		return nil
	}
	message := strings.TrimSpace(response.Error.Message)
	if message == "" {
		message = "API từ chối quyền hoặc yêu cầu"
	}
	return fmt.Errorf("tiktok display (%s): %s", strings.Trim(code, `"`), message)
}
