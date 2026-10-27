package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FindYtDlp tìm đường dẫn nhị phân của yt-dlp trên hệ thống (dùng cho trường hợp video mạng xã hội mã hóa phức tạp).
func FindYtDlp() string {
	candidates := []string{
		"/Users/ninh.le/.local/bin/yt-dlp",
		"/usr/local/bin/yt-dlp",
		"/opt/homebrew/bin/yt-dlp",
		"yt-dlp",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// IsSocialMediaURL kiểm tra link có phải nền tảng mạng xã hội video (TikTok, Reels, Instagram, YouTube Shorts).
func IsSocialMediaURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "tiktok.com") ||
		strings.Contains(lower, "instagram.com") ||
		strings.Contains(lower, "instagr.am") ||
		strings.Contains(lower, "youtube.com/shorts") ||
		strings.Contains(lower, "youtu.be") ||
		strings.Contains(lower, "facebook.com/reel") ||
		strings.Contains(lower, "fb.watch")
}

// IsVideoURL kiểm tra URL có phải là file video trực tiếp hay không.
func IsVideoURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	return ext == ".mp4" || ext == ".mov" || ext == ".webm" || ext == ".mkv" || ext == ".m4v"
}

// IsImageURL kiểm tra URL có phải là ảnh trực tiếp hay không.
func IsImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif" || ext == ".gif"
}

func isAudioURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(filepath.Ext(u.Path)) {
	case ".mp3", ".aac", ".wav", ".m4a", ".ogg", ".flac":
		return true
	default:
		return false
	}
}

// extractMediaURLFromHTML phân tích cú pháp HTML thuần Go (không cần thư viện bên ngoài) để tìm video/ảnh từ trang web.
func extractMediaURLFromHTML(htmlContent, pageURL, preferredType string) string {
	baseU, _ := url.Parse(pageURL)

	resolveURL := func(link string) string {
		link = strings.TrimSpace(link)
		if link == "" {
			return ""
		}
		if strings.HasPrefix(link, "//") {
			if baseU != nil && baseU.Scheme != "" {
				return baseU.Scheme + ":" + link
			}
			return "https:" + link
		}
		if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
			return link
		}
		if baseU != nil {
			rel, err := url.Parse(link)
			if err == nil {
				return baseU.ResolveReference(rel).String()
			}
		}
		return link
	}

	// 1. Nếu ưu tiên tìm Video
	if preferredType == "video" {
		videoPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)<meta\s+property=["']og:video(?::secure_url)?["']\s+content=["']([^"']+)["']`),
			regexp.MustCompile(`(?i)<meta\s+content=["']([^"']+)["']\s+property=["']og:video(?::secure_url)?["']`),
			regexp.MustCompile(`(?i)<meta\s+name=["']twitter:player:stream["']\s+content=["']([^"']+)["']`),
			regexp.MustCompile(`(?i)<source\s+[^>]*src=["']([^"']+\.(?:mp4|webm|mov)[^"']*)["']`),
			regexp.MustCompile(`(?i)<video\s+[^>]*src=["']([^"']+)["']`),
		}
		for _, re := range videoPatterns {
			if match := re.FindStringSubmatch(htmlContent); len(match) > 1 {
				return resolveURL(match[1])
			}
		}
	}
	if preferredType == "audio" {
		audioPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)<audio\s+[^>]*src=["']([^"']+)["']`),
			regexp.MustCompile(`(?i)<source\s+[^>]*src=["']([^"']+\.(?:mp3|aac|wav|m4a|ogg|flac)(?:\?[^"']*)?)["']`),
		}
		for _, re := range audioPatterns {
			if match := re.FindStringSubmatch(htmlContent); len(match) > 1 {
				return resolveURL(match[1])
			}
		}
	}

	// 2. Tìm Ảnh đại diện / Sản phẩm / Khung hình từ trang Web
	imagePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<meta\s+property=["']og:image(?::secure_url)?["']\s+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta\s+content=["']([^"']+)["']\s+property=["']og:image(?::secure_url)?["']`),
		regexp.MustCompile(`(?i)<meta\s+name=["']twitter:image["']\s+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta\s+itemprop=["']image["']\s+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<img\s+[^>]*src=["']([^"']+\.(?:jpg|jpeg|png|webp)[^"']*)["']`),
	}
	for _, re := range imagePatterns {
		if match := re.FindStringSubmatch(htmlContent); len(match) > 1 {
			return resolveURL(match[1])
		}
	}

	return ""
}

// ExtractFrameFromVideo trích xuất 1 khung hình sắc nét từ video tại thời điểm chỉ định.
func ExtractFrameFromVideo(ctx context.Context, videoPath, outPath string, timestampSec float64) error {
	if timestampSec <= 0 {
		timestampSec = 1.8 // Mặc định thời điểm 1.8s (thường đã ổn định khung hình, rõ nét nhân vật/sản phẩm)
	}

	timeStr := fmt.Sprintf("%.3f", timestampSec)
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-ss", timeStr, "-i", videoPath, "-vframes", "1", "-q:v", "2", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback trích xuất frame đầu tiên nếu timestamp vượt quá độ dài video
		cmd2 := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", videoPath, "-vframes", "1", "-q:v", "2", outPath)
		if err2 := cmd2.Run(); err2 != nil {
			return fmt.Errorf("ffmpeg extract frame failed: %v: %s", err, string(out))
		}
	}
	return nil
}

// DownloadMediaResource tải tài nguyên video hoặc ảnh từ URL (TikTok, Reels, Instagram, YouTube Shorts, Web).
// Tự động ưu tiên tải thuần Go HTTP Client; chỉ dùng công cụ chuyên dụng đối với link mạng xã hội có mã hóa chống cào bot phức tạp.
func DownloadMediaResource(ctx context.Context, rawURL, destDir, targetType string) (string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", "", errors.New("địa chỉ URL rỗng")
	}

	_ = os.MkdirAll(destDir, 0o755)

	// 1. Nếu là đường dẫn file cục bộ đã tồn tại
	if fileExists(rawURL) {
		if isAudioFile(rawURL) {
			return rawURL, "audio", nil
		}
		if isVideoFile(rawURL) {
			if targetType == "image" {
				framePath := filepath.Join(destDir, fmt.Sprintf("%d_extracted_frame.jpg", time.Now().UnixNano()))
				if err := ExtractFrameFromVideo(ctx, rawURL, framePath, 1.8); err == nil {
					return framePath, "image", nil
				}
			}
			return rawURL, "video", nil
		}
		return rawURL, "image", nil
	}

	// 2. Nếu là Data URL Base64
	if strings.HasPrefix(rawURL, "data:image/") || strings.HasPrefix(rawURL, "data:video/") || strings.HasPrefix(rawURL, "data:audio/") {
		isVid := strings.HasPrefix(rawURL, "data:video/")
		isAudio := strings.HasPrefix(rawURL, "data:audio/")
		ext := ".jpg"
		mType := "image"
		if isVid {
			ext = ".mp4"
			mType = "video"
		} else if isAudio {
			ext = audioExtension(rawURL)
			mType = "audio"
		}
		targetPath := filepath.Join(destDir, fmt.Sprintf("%d_base64_upload%s", time.Now().UnixNano(), ext))
		if err := decodeBase64DataURL(rawURL, targetPath); err != nil {
			return "", "", fmt.Errorf("không thể giải mã dữ liệu base64: %w", err)
		}
		return targetPath, mType, nil
	}

	// 3. Nếu là đường dẫn nội bộ /media/
	if strings.HasPrefix(rawURL, "/media/") {
		fn := strings.TrimPrefix(rawURL, "/media/")
		candidates := []string{
			filepath.Join("data", "uploads", fn),
			filepath.Join("data", "output", "studio", fn),
			filepath.Join(destDir, fn),
		}
		for _, cand := range candidates {
			if fileExists(cand) {
				if isVideoFile(cand) && targetType == "image" {
					framePath := filepath.Join(destDir, fmt.Sprintf("%d_frame.jpg", time.Now().UnixNano()))
					if err := ExtractFrameFromVideo(ctx, cand, framePath, 1.8); err == nil {
						return framePath, "image", nil
					}
				}
				if isVideoFile(cand) {
					return cand, "video", nil
				}
				return cand, "image", nil
			}
		}
	}

	// 4. Nếu là link mạng xã hội video (TikTok, Instagram Reels, YouTube Shorts) có cơ chế token mã hóa phức tạp:
	// "trường hợp build phức tạp và tốn thời gian quá thì hãy sử dụng thư viện/công cụ nhé"
	isSocial := IsSocialMediaURL(rawURL)
	if isSocial {
		ytDlp := FindYtDlp()
		if ytDlp != "" {
			outTemplate := filepath.Join(destDir, fmt.Sprintf("social_%d_%%(id)s.%%(ext)s", time.Now().UnixNano()))
			cmd := exec.CommandContext(ctx, ytDlp,
				"--no-playlist",
				"-f", "b[ext=mp4]/b/bestvideo+bestaudio/best",
				"--merge-output-format", "mp4",
				"-o", outTemplate,
				rawURL,
			)
			out, err := cmd.CombinedOutput()
			if err == nil {
				matches, _ := filepath.Glob(filepath.Join(destDir, "social_*"))
				if len(matches) > 0 {
					downloadedVideo := matches[len(matches)-1]
					if targetType == "image" {
						framePath := filepath.Join(destDir, fmt.Sprintf("frame_%d.jpg", time.Now().UnixNano()))
						if err := ExtractFrameFromVideo(ctx, downloadedVideo, framePath, 2.0); err == nil {
							return framePath, "image", nil
						}
					}
					return downloadedVideo, "video", nil
				}
			} else {
				fmt.Printf("[Downloader] yt-dlp fallback warning: %v: %s\n", err, string(out))
			}
		}
	}

	// 5. Ưu tiên số 1: Tự tải và bóc tách trực tiếp bằng Go Standard Library (net/http)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("tạo yêu cầu tải URL thất bại: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("không thể kết nối tải từ URL '%s': %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("máy chủ từ chối yêu cầu (mã HTTP %d)", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))

	// 6. Nếu phản hồi là trang HTML (người dùng dán link bài viết / web sản phẩm):
	// Tự bóc tách link media bên trong HTML mà không cần bất kỳ thư viện bên ngoài nào
	if strings.Contains(contentType, "text/html") {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // Đọc tối đa 5MB HTML
		if err == nil && len(bodyBytes) > 0 {
			extractedMediaURL := extractMediaURLFromHTML(string(bodyBytes), rawURL, targetType)
			if extractedMediaURL != "" && extractedMediaURL != rawURL {
				// Đệ quy tải file media thực tế vừa bóc tách được
				return DownloadMediaResource(ctx, extractedMediaURL, destDir, targetType)
			}
		}
		return "", "", fmt.Errorf("không tìm thấy liên kết video hoặc hình ảnh trong trang web '%s'", rawURL)
	}

	isDirectVideo := IsVideoURL(rawURL)
	isVid := strings.Contains(contentType, "video") || isDirectVideo
	isAudio := strings.Contains(contentType, "audio") || targetType == "audio" && isAudioURL(rawURL)
	ext := ".jpg"
	mType := "image"

	if isAudio {
		ext = audioExtensionFromContentType(contentType, rawURL)
		mType = "audio"
	} else if isVid {
		ext = ".mp4"
		mType = "video"
	} else if strings.Contains(contentType, "png") {
		ext = ".png"
	} else if strings.Contains(contentType, "webp") {
		ext = ".webp"
	}

	destPath := filepath.Join(destDir, fmt.Sprintf("dl_%d%s", time.Now().UnixNano(), ext))
	dstFile, err := os.Create(destPath)
	if err != nil {
		return "", "", err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, resp.Body); err != nil {
		return "", "", fmt.Errorf("lỗi ghi tệp tải về: %w", err)
	}

	// Nếu tải về là video nhưng node đang cần ảnh (Character / Product)
	if mType == "video" && targetType == "image" {
		framePath := filepath.Join(destDir, fmt.Sprintf("frame_from_vid_%d.jpg", time.Now().UnixNano()))
		if err := ExtractFrameFromVideo(ctx, destPath, framePath, 2.0); err == nil {
			return framePath, "image", nil
		}
	}

	return destPath, mType, nil
}

func audioExtension(dataURL string) string {
	lower := strings.ToLower(dataURL)
	switch {
	case strings.HasPrefix(lower, "data:audio/mpeg"), strings.HasPrefix(lower, "data:audio/mp3"):
		return ".mp3"
	case strings.HasPrefix(lower, "data:audio/wav"), strings.HasPrefix(lower, "data:audio/x-wav"):
		return ".wav"
	case strings.HasPrefix(lower, "data:audio/mp4"), strings.HasPrefix(lower, "data:audio/m4a"):
		return ".m4a"
	case strings.HasPrefix(lower, "data:audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(lower, "data:audio/flac"):
		return ".flac"
	default:
		return ".aac"
	}
}

func audioExtensionFromContentType(contentType, rawURL string) string {
	switch {
	case strings.Contains(contentType, "mpeg"), strings.Contains(contentType, "mp3"):
		return ".mp3"
	case strings.Contains(contentType, "wav"):
		return ".wav"
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "m4a"):
		return ".m4a"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	case strings.Contains(contentType, "flac"):
		return ".flac"
	case strings.Contains(contentType, "aac"):
		return ".aac"
	default:
		return filepath.Ext(rawURL)
	}
}
