package publishers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func (p *YouTubePublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	return p.PublishMeta(ctx, videoPath, VideoMeta{Title: title, Description: description}, kind)
}

// VideoMeta là metadata đầy đủ cho 1 video (Đợt M1): tags + thumbnail.
// Publish giữ chữ ký cũ để tương thích, PublishMeta là đường đầy đủ.
type VideoMeta struct {
	Title         string
	Description   string
	Tags          []string
	ThumbnailPath string // "" = không đặt thumbnail tùy chỉnh
}

// PublishMeta đăng video kèm tags + thumbnail tùy chỉnh (nếu có).
func (p *YouTubePublisher) PublishMeta(ctx context.Context, videoPath string, meta VideoMeta, kind string) PublishResult {
	if !p.IsConfigured() {
		return PublishResult{Ok: false, Platform: p.Name(), Error: "youtube not configured"}
	}
	if !p.Handles(kind) {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: fmt.Sprintf("kind %s not enabled for this channel", kind)}
	}
	fi, err := os.Stat(videoPath)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	size := fi.Size()
	token, err := p.accessToken()
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	title := meta.Title
	if title == "" {
		title = "AI video"
	}
	category := CategoryByKind[kind]
	if category == "" {
		category = "22"
	}
	statusObj := map[string]any{
		"privacyStatus":           p.privacy,
		"selfDeclaredMadeForKids": false,
	}
	if p.Synthetic {
		statusObj["containsSyntheticMedia"] = true
	}
	// Giờ vàng: chỉ áp cho private (YouTube yêu cầu) — tự công khai đúng lịch.
	if p.privacy == "private" && goldenHourEnabled(p.Username) {
		statusObj["publishAt"] = NextGoldenHour(time.Now()).UTC().Format(time.RFC3339)
	}
	snippet := map[string]any{
		"title":       truncate(title, 100),
		"description": truncate(meta.Description, 5000),
		"categoryId":  category,
	}
	if len(meta.Tags) > 0 {
		tags := make([]string, 0, len(meta.Tags))
		for _, t := range meta.Tags {
			if t = truncate(t, 60); t != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) > 0 {
			snippet["tags"] = tags
		}
	}
	metaJSON, _ := json.Marshal(map[string]any{
		"snippet": snippet,
		"status":  statusObj,
	})
	status, headers, raw, err := p.http()("POST", youtubeUploadURL,
		map[string]string{
			"Authorization":           "Bearer " + token,
			"Content-Type":            "application/json; charset=UTF-8",
			"X-Upload-Content-Length": fmt.Sprintf("%d", size),
			"X-Upload-Content-Type":   "video/mp4",
		}, metaJSON)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300), UploadRequestAttempted: true}
	}
	session := headers["Location"]
	if session == "" {
		session = headers["location"]
	}
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	if status != 200 || session == "" {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: truncate(fmt.Sprintf("upload init failed: HTTP %d %.200v", status, data), 300), UploadRequestAttempted: true}
	}
	blob, err := os.ReadFile(videoPath)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300), UploadRequestAttempted: true}
	}
	status, _, raw, err = p.http()("PUT", session,
		map[string]string{
			"Content-Length": fmt.Sprintf("%d", size),
			"Content-Range":  fmt.Sprintf("bytes 0-%d/%d", size-1, size),
		}, blob)
	if err != nil {
		uncertain := status == 0 || status == http.StatusRequestTimeout || status >= http.StatusInternalServerError
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300),
			UploadRequestAttempted: true, DeliveryUncertain: uncertain}
	}
	data = nil
	_ = json.Unmarshal(raw, &data)
	if status != 200 && status != 201 {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error:                  truncate(fmt.Sprintf("upload failed: HTTP %d %.200v", status, data), 300),
			UploadRequestAttempted: true,
			DeliveryUncertain:      status == http.StatusRequestTimeout || status >= http.StatusInternalServerError}
	}
	var vid, videoURL string
	if data != nil {
		vid, _ = data["id"].(string)
	}
	if vid == "" {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: "upload response missing video id", UploadRequestAttempted: true, DeliveryUncertain: true}
	}
	if vid != "" {
		videoURL = "https://youtu.be/" + vid
	}
	res := PublishResult{Ok: true, Platform: p.Name(), RemoteID: vid,
		URL: videoURL, Draft: p.privacy != "public", UploadRequestAttempted: true}
	// Thumbnail là phần tăng thêm: thất bại thì video vẫn đã đăng,
	// ghi rõ để automation log trung thực.
	if vid != "" && meta.ThumbnailPath != "" {
		if err := p.setThumbnail(vid, meta.ThumbnailPath); err != nil {
			res.ThumbnailError = truncate(err.Error(), 200)
		} else {
			res.ThumbnailSet = true
		}
	}
	return res
}

// youtubeThumbnailSetURL đặt thumbnail tùy chỉnh cho video.
