package web

import (
	"context"
	"crypto/sha256"
	"errors"
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

func (s *Server) handleStudioAICreate(w http.ResponseWriter, r *http.Request) {
	museReady := s.webAccountReady(webai.PlatformMuseAI)
	flowReady := s.webAccountReady(webai.PlatformGoogleFlow)
	s.render(w, "studio_ai_create", s.ctx(
		"Title", "Tạo nội dung AI",
		"Path", "/studio/create",
		"MuseReady", museReady,
		"FlowReady", flowReady,
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
	))
}

func (s *Server) webAccountReady(platform string) bool {
	if s.WebAI == nil {
		return false
	}
	accounts, err := s.WebAI.GetAccounts(platform)
	if err != nil {
		return false
	}
	for _, account := range accounts {
		if account.IsActive && strings.TrimSpace(account.Cookie) != "" && account.Status != "expired" && account.Status != "invalid" && account.Status != "disconnected" {
			return true
		}
	}
	return false
}

func (s *Server) handleStudioAIGenerate(w http.ResponseWriter, r *http.Request) {
	back := func(key, message string) {
		seeOther(w, r, "/studio/create?"+key+"="+url.QueryEscape(message))
	}
	if s.Studio == nil || s.WebAI == nil {
		back("err", "Studio hoặc kho tài khoản AI chưa sẵn sàng.")
		return
	}
	if err := r.ParseForm(); err != nil {
		back("err", "Không đọc được yêu cầu tạo nội dung.")
		return
	}
	provider := strings.TrimSpace(r.PostFormValue("provider"))
	kind := strings.TrimSpace(r.PostFormValue("kind"))
	prompt := strings.TrimSpace(r.PostFormValue("prompt"))
	title := strings.TrimSpace(r.PostFormValue("title"))
	aspect := strings.TrimSpace(r.PostFormValue("aspect"))
	if provider != webai.PlatformMuseAI && provider != webai.PlatformGoogleFlow {
		back("err", "Chọn Muse Chat hoặc Google Flow.")
		return
	}
	if kind != "script" && kind != "image" && kind != "video" {
		back("err", "Chọn kịch bản, hình ảnh hoặc video.")
		return
	}
	if provider == webai.PlatformGoogleFlow && kind != "video" {
		back("err", "Google Flow hiện dùng để tạo video; dùng Muse Chat cho kịch bản và hình ảnh.")
		return
	}
	if prompt == "" || len([]rune(prompt)) > 5000 {
		back("err", "Nhập prompt từ 1 đến 5.000 ký tự.")
		return
	}
	if title == "" {
		title = defaultAIGenerationTitle(kind, prompt)
	}
	if len([]rune(title)) > 120 {
		title = string([]rune(title)[:120])
	}
	if aspect == "" {
		aspect = "VIDEO_ASPECT_RATIO_PORTRAIT"
	}
	platformName := "Muse Chat"
	if provider == webai.PlatformGoogleFlow {
		platformName = "Google Flow"
	}

	account, err := s.WebAI.PickActiveAccount(provider)
	if err != nil {
		back("err", err.Error()+" — kiểm tra phiên trong Cài đặt → Tài khoản AI.")
		return
	}
	if strings.TrimSpace(account.Cookie) == "" {
		back("err", platformName+" chưa có cookie phiên hợp lệ; kết nối lại tài khoản trong Cài đặt → Tài khoản AI.")
		return
	}
	params := map[string]any{"provider": provider, "kind": kind, "prompt": prompt, "aspect": aspect, "account_id": account.ID}
	id, err := s.Studio.CreateExternalJob("ai_content", platformName+": "+title, params,
		func(ctx context.Context, progress func(int, string)) (string, string, error) {
			progress(5, platformName+": bắt đầu tác vụ")
			if provider == webai.PlatformGoogleFlow {
				progress(15, "FLOW_PROJECT: tạo project trong Flow")
				result, err := webai.GoogleFlowVideo(ctx, account.Cookie, prompt, aspect, func(line string) { progress(45, line) })
				if webai.CanFallbackToGoogleFlowHeadless(err) && ctx.Err() == nil {
					progress(20, "FLOW_FALLBACK: direct API unavailable before submit; starting headless browser")
					result, err = webai.GoogleFlowVideoHeadless(ctx, account.Cookie, prompt, aspect, result.ProjectID, func(line string) { progress(55, line) })
				}
				if err != nil {
					return "", "", err
				}
				filename := fmt.Sprintf("flow_%s.mp4", safeAIFileID(result.ProjectID))
				outPath := s.Studio.OutputPath(filename)
				progress(80, "FLOW_DOWNLOAD: tải MP4 vào kho Studio")
				if err := webai.DownloadFlowVideo(ctx, result.MediaURL, outPath); err != nil {
					return "", "", err
				}
				return filename, "", nil
			}

			promptText := museProductionPrompt(kind, prompt)
			progress(10, "MUSE_CHAT: gửi prompt qua gateway Muse")
			result, err := webai.MuseChat(ctx, account.Cookie, account.ID, promptText, func(line string) { progress(45, line) })
			var gatewayUnavailable *webai.MuseGatewayUnavailableError
			if errors.As(err, &gatewayUnavailable) {
				progress(15, "MUSE_FALLBACK: direct gateway unavailable; starting headless browser")
				result, err = webai.MuseChatHeadless(ctx, account.Cookie, promptText, func(line string) { progress(45, line) })
			}
			if err != nil {
				return "", "", err
			}
			if kind == "script" {
				filename := fmt.Sprintf("muse_script_%s.md", safeAIFileID(account.ID+title))
				if err := os.MkdirAll(filepath.Dir(s.Studio.OutputPath(filename)), 0o755); err != nil {
					return "", "", err
				}
				if err := os.WriteFile(s.Studio.OutputPath(filename), []byte(result.Text+"\n"), 0o644); err != nil {
					return "", "", err
				}
				return filename, "", nil
			}
			wantExt := ".mp4"
			if kind == "image" {
				wantExt = ".png"
			}
			mediaURL := ""
			for _, candidate := range result.MediaURLs {
				parsed, parseErr := url.Parse(candidate)
				if parseErr != nil {
					continue
				}
				candidateExt := strings.ToLower(filepath.Ext(parsed.Path))
				if candidateExt == wantExt || (kind == "image" && (candidateExt == ".jpg" || candidateExt == ".jpeg" || candidateExt == ".webp")) {
					mediaURL = candidate
					break
				}
			}
			if mediaURL == "" {
				return "", "", fmt.Errorf("Muse Chat replied but included no downloadable %s link", strings.TrimPrefix(wantExt, "."))
			}
			ext := filepath.Ext(strings.SplitN(mediaURL, "?", 2)[0])
			filename := fmt.Sprintf("muse_%s%s", safeAIFileID(account.ID+title), ext)
			outPath := s.Studio.OutputPath(filename)
			progress(80, "MUSE_DOWNLOAD: tải media vào kho Studio")
			if err := downloadMuseMedia(ctx, account.Cookie, mediaURL, outPath); err != nil {
				return "", "", err
			}
			return filename, "", nil
		})
	if err != nil {
		back("err", "Không tạo được job: "+err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?tab=running&job_id="+url.QueryEscape(id))
}

func museProductionPrompt(kind, prompt string) string {
	switch kind {
	case "script":
		return "Viết kịch bản video bằng tiếng Việt theo yêu cầu dưới đây. Trả lời trực tiếp bằng kịch bản hoàn chỉnh, có hook, lời thoại, phân cảnh và CTA; không chỉ đưa hướng dẫn hoặc prompt.\n\n" + prompt
	case "image":
		return "Tạo một hình ảnh theo yêu cầu dưới đây bằng công cụ tạo ảnh của Muse. Ưu tiên ảnh dọc 9:16, không tự thêm chữ hoặc watermark. Sau khi tạo, trả link chia sẻ tải trực tiếp tới file ảnh.\n\n" + prompt
	default:
		return "Tạo một video dọc 9:16 theo yêu cầu dưới đây bằng công cụ tạo video của Muse. Hãy tạo media thực tế, không chỉ viết kịch bản hay prompt. Sau khi tạo, trả link chia sẻ tải trực tiếp tới file MP4.\n\n" + prompt
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
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "muse.ai" && u.Hostname() != "www.muse.ai") || !strings.HasPrefix(u.Path, "/files/") {
		return fmt.Errorf("Muse media link is not a trusted muse.ai file URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("invalid Muse media URL")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AICOS/1.0)")
	req.Header.Set("Cookie", cookie)
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
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
