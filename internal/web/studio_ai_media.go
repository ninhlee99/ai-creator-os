package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func museProductionPrompt(kind, prompt, aspect string) string {
	switch kind {
	case "script":
		return "Viết kịch bản video bằng tiếng Việt theo yêu cầu dưới đây. Trả lời trực tiếp bằng kịch bản hoàn chỉnh, có hook, lời thoại, phân cảnh và CTA; không chỉ đưa hướng dẫn hoặc prompt.\n\n" + prompt
	case "image":
		return "Tạo một hình ảnh theo yêu cầu dưới đây bằng công cụ tạo ảnh của Muse. Ưu tiên ảnh dọc 9:16, không tự thêm chữ hoặc watermark. Sau khi tạo, trả link chia sẻ tải trực tiếp tới file ảnh.\n\n" + prompt
	default:
		return "Tạo một video " + studioVideoAspectLabel(aspect) + " theo yêu cầu dưới đây bằng công cụ tạo video của Muse. Hãy tạo media thực tế, không chỉ viết kịch bản hay prompt. Sau khi tạo, trả link chia sẻ tải trực tiếp tới file MP4.\n\n" + prompt
	}
}

func normalizedStudioVideoAspect(value string) (string, error) {
	switch value {
	case "VIDEO_ASPECT_RATIO_PORTRAIT", "9:16":
		return "9:16", nil
	case "VIDEO_ASPECT_RATIO_LANDSCAPE", "16:9":
		return "16:9", nil
	case "VIDEO_ASPECT_RATIO_SQUARE", "1:1":
		return "1:1", nil
	default:
		return "", fmt.Errorf("tỷ lệ video không hỗ trợ: %q", value)
	}
}

func studioVideoAspectLabel(value string) string {
	switch value {
	case "16:9":
		return "ngang 16:9"
	case "1:1":
		return "vuông 1:1"
	default:
		return "dọc 9:16"
	}
}

func defaultAIGenerationTitle(kind, prompt string) string {
	labels := map[string]string{"script": "Kịch bản Muse", "image": "Ảnh Muse", "video": "Video AI"}
	text := strings.Join(strings.Fields(prompt), " ")
	if len([]rune(text)) > 72 {
		text = string([]rune(text)[:72]) + "…"
	}
	return labels[kind] + ": " + text
}

func safeAIFileID(value string) string {
	sum := sha256.Sum256([]byte(value + fmt.Sprint(time.Now().UnixNano())))
	return fmt.Sprintf("%x", sum[:8])
}

func downloadMuseMedia(ctx context.Context, cookie, rawURL, output string) error {
	return downloadMuseMediaWithCookieSnapshot(ctx, cookie, nil, rawURL, output, nil)
}

func downloadMuseMediaWithCookieSnapshot(ctx context.Context, cookie string, cookies []webai.BrowserCookie, rawURL, output string, updater webai.CookieSnapshotUpdater) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "muse.ai" && u.Hostname() != "www.muse.ai") || !strings.HasPrefix(u.Path, "/files/") {
		return fmt.Errorf("Muse media link is not a trusted muse.ai file URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("invalid Muse media URL")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AICOS/1.0)")
	client, err := webai.NewCookieSnapshotHTTPClient(webai.PlatformMuseAI, cookie, cookies, 3*time.Minute, updater)
	if err != nil {
		return fmt.Errorf("could not initialize Muse cookie jar")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Muse media download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Muse media HTTP %d", resp.StatusCode)
	}
	maxBytes := int64(250 << 20)
	if ext := strings.ToLower(filepath.Ext(u.Path)); ext != ".mp4" && ext != ".webm" {
		maxBytes = 50 << 20
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(output), ".aicos-muse-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if copyErr != nil {
		_ = f.Close()
		return copyErr
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if written < 12 || written > maxBytes {
		return fmt.Errorf("Muse media size is invalid or exceeds allowed limit")
	}
	if err := verifyMuseMedia(tmpPath, filepath.Ext(u.Path)); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, output); err != nil {
		return err
	}
	return nil
}

func verifyMuseMedia(path, ext string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	ext = strings.ToLower(ext)
	switch ext {
	case ".mp4", ".m4v":
		var signature [4]byte
		if info.Size() < 12 {
			return fmt.Errorf("Muse media is not an MP4 file")
		}
		if _, err := f.ReadAt(signature[:], 4); err != nil || string(signature[:]) != "ftyp" {
			return fmt.Errorf("Muse media is not an MP4 file")
		}
	case ".png":
		var signature [8]byte
		if info.Size() < int64(len(signature)) {
			return fmt.Errorf("Muse media is not a PNG image")
		}
		if _, err := f.ReadAt(signature[:], 0); err != nil || string(signature[:]) != "\x89PNG\r\n\x1a\n" {
			return fmt.Errorf("Muse media is not a PNG image")
		}
	case ".jpg", ".jpeg":
		var header [2]byte
		var tail [2]byte
		_, headerErr := f.ReadAt(header[:], 0)
		_, tailErr := f.ReadAt(tail[:], info.Size()-2)
		if info.Size() < 4 || headerErr != nil || tailErr != nil || header != [2]byte{0xff, 0xd8} || tail != [2]byte{0xff, 0xd9} {
			return fmt.Errorf("Muse media is not a JPEG image")
		}
	case ".webp":
		var signature [12]byte
		_, readErr := f.ReadAt(signature[:], 0)
		if info.Size() < int64(len(signature)) || readErr != nil || string(signature[:4]) != "RIFF" || string(signature[8:12]) != "WEBP" {
			return fmt.Errorf("Muse media is not a WebP image")
		}
	case ".webm":
		var signature [4]byte
		_, readErr := f.ReadAt(signature[:], 0)
		if info.Size() < int64(len(signature)) || readErr != nil || signature != [4]byte{0x1a, 0x45, 0xdf, 0xa3} {
			return fmt.Errorf("Muse media is not a WebM video")
		}
	default:
		return fmt.Errorf("Muse media format %q is not supported", ext)
	}
	return nil
}
