package publishers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

const youtubeThumbnailSetURL = "https://www.googleapis.com/upload/youtube/v3/thumbnails/set"

// youtubeVideosURL cập nhật metadata/trạng thái video (đợt N: chuyển public).
const youtubeVideosURL = "https://www.googleapis.com/youtube/v3/videos?part=status"

// UpdatePrivacy đổi chế độ riêng tư của video (private/public/unlisted).
// Dùng cho "tự chuyển công khai sau N giờ" — video đã đăng private trước đó.
func (p *YouTubePublisher) UpdatePrivacy(videoID, privacy string) error {
	if videoID == "" {
		return fmt.Errorf("thiếu video ID")
	}
	switch privacy {
	case "private", "public", "unlisted":
	default:
		return fmt.Errorf("privacy %q không hợp lệ", privacy)
	}
	token, err := p.accessToken()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"id":     videoID,
		"status": map[string]any{"privacyStatus": privacy},
	})
	status, _, raw, err := p.http()("PUT", youtubeVideosURL,
		map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "application/json; charset=UTF-8",
		}, body)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("đổi quyền riêng tư: HTTP %d %.200s", status, raw)
	}
	return nil
}

// setThumbnail tải ảnh lên làm thumbnail YouTube (yêu cầu kênh đã xác minh;
// chưa xác minh → API trả 403, caller ghi log rõ).
func (p *YouTubePublisher) setThumbnail(videoID, imagePath string) error {
	img, err := os.ReadFile(imagePath)
	if err != nil {
		return fmt.Errorf("đọc ảnh thumbnail: %w", err)
	}
	if len(img) > 2*1024*1024 {
		return fmt.Errorf("ảnh thumbnail quá 2MB (YouTube giới hạn)")
	}
	mime := "image/jpeg"
	switch {
	case len(img) > 3 && img[0] == 0x89 && img[1] == 0x50:
		mime = "image/png"
	case len(img) > 3 && img[0] == 0x47 && img[1] == 0x49:
		mime = "image/gif"
	}
	token, err := p.accessToken()
	if err != nil {
		return err
	}
	u := youtubeThumbnailSetURL + "?videoId=" + url.QueryEscape(videoID)
	status, _, raw, err := p.http()("POST", u,
		map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  mime,
		}, img)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("đặt thumbnail: HTTP %d %.200s", status, raw)
	}
	return nil
}
