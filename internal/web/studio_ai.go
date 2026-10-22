package web

import (
	"context"
	"errors"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleStudioAICreate(w http.ResponseWriter, r *http.Request) {
	s.render(w, "studio_ai_create", s.ctx(
		"Title", "Tạo nội dung AI",
		"Path", "/studio/create",
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
	))
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
	if provider == webai.PlatformGoogleFlow && r.PostFormValue("confirm_flow_credits") != "1" {
		back("err", "Xác nhận Google Flow có thể dùng credits trước khi tạo video.")
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
	outputAspect := ""
	outputResolution := ""
	if kind == "video" {
		var aspectErr error
		outputAspect, aspectErr = normalizedStudioVideoAspect(aspect)
		if aspectErr != nil {
			back("err", aspectErr.Error())
			return
		}
		var resolutionErr error
		outputResolution, resolutionErr = studio.NormalizeVideoOutputResolution(r.PostFormValue("output_resolution"))
		if resolutionErr != nil {
			back("err", resolutionErr.Error())
			return
		}
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
	if kind == "video" {
		params["output_aspect"] = outputAspect
		params["output_resolution"] = outputResolution
	}
	id, err := s.Studio.CreateExternalJob("ai_content", platformName+": "+title, params,
		func(ctx context.Context, progress func(int, string)) (string, string, error) {
			progress(5, platformName+": bắt đầu tác vụ")
			if provider == webai.PlatformGoogleFlow {
				progress(15, "FLOW_PROJECT: tạo project trong Flow")
				result, usedAccount, initialErr, err := s.generateStudioFlowVideo(ctx, account, prompt, aspect, func(line string) {
					progress(45, line)
				})
				if err != nil {
					authRejected := webai.IsWebSessionAuthError(err) || webai.IsWebSessionAuthError(initialErr)
					if authRejected {
						_ = s.WebAI.MarkSessionExpired(provider, usedAccount.Email)
					} else if result.CookieHeader != "" {
						if saveErr := s.persistWebAICookieRefresh(ctx, provider, usedAccount.Email, result.CookieHeader, true); saveErr != nil {
							progress(60, "FLOW_COOKIE_REFRESH: chưa xác minh/lưu được cookie mới")
						}
					}
					return "", "", err
				}
				if result.CookieHeader != "" && strings.TrimSpace(usedAccount.Email) != "" {
					if err := s.persistWebAICookieRefresh(ctx, provider, usedAccount.Email, result.CookieHeader, false); err != nil {
						progress(60, "FLOW_COOKIE_REFRESH: chưa lưu được phiên mới")
					}
				}
				filename := fmt.Sprintf("flow_%s.mp4", safeAIFileID(result.ProjectID))
				outPath := s.Studio.OutputPath(filename)
				rawPath := outPath + ".source.mp4"
				defer os.Remove(rawPath)
				progress(78, "FLOW_DOWNLOAD: tải video gốc")
				if err := webai.DownloadFlowVideo(ctx, result.MediaURL, rawPath); err != nil {
					return "", "", err
				}
				progress(86, "VIDEO_UPSCALE: xuất master "+strings.ToUpper(outputResolution))
				if err := studio.UpscaleVideo(ctx, rawPath, outPath, outputAspect, outputResolution); err != nil {
					return "", "", err
				}
				return filename, "", nil
			}

			promptText := museProductionPrompt(kind, prompt, outputAspect)
			progress(10, "MUSE_CHAT: gửi prompt qua gateway Muse")
			usedMuseAccount := account
			attemptedMuse := map[string]struct{}{account.ID: {}}
			var result webai.MuseReply
			var err, initialErr error
			for {
				result, err = webai.MuseChat(ctx, usedMuseAccount.Cookie, usedMuseAccount.ID, promptText, func(line string) { progress(45, line) })
				initialErr = err
				if cooldown, limited := webai.MusePreSubmitRateLimitCooldown(err); limited {
					if markErr := s.WebAI.MarkRateLimited(usedMuseAccount.ID, cooldown); markErr != nil {
						err = fmt.Errorf("Muse Chat báo hết quota; không lưu được cooldown tài khoản miễn phí: %w", markErr)
						break
					}
					next, pickErr := s.WebAI.PickActiveAccountExcluding(webai.PlatformMuseAI, attemptedMuse)
					if pickErr != nil {
						err = fmt.Errorf("Muse Chat hết quota; dừng vì không còn tài khoản miễn phí khả dụng: %w", pickErr)
						break
					}
					attemptedMuse[next.ID] = struct{}{}
					usedMuseAccount = next
					progress(20, "MUSE_ACCOUNT_ROTATION: tài khoản miễn phí trước bị giới hạn; chuyển sang tài khoản miễn phí tiếp theo")
					continue
				}
				var gatewayUnavailable *webai.MuseGatewayUnavailableError
				if errors.As(err, &gatewayUnavailable) {
					progress(15, "MUSE_FALLBACK: direct gateway unavailable; starting headless browser")
					refreshedCookie := result.CookieHeader
					fallbackCookie := usedMuseAccount.Cookie
					if refreshedCookie != "" {
						fallbackCookie = refreshedCookie
					}
					result, err = webai.MuseChatHeadless(ctx, fallbackCookie, promptText, func(line string) { progress(45, line) })
					if result.CookieHeader == "" {
						result.CookieHeader = refreshedCookie
					}
				}
				break
			}
			if err != nil {
				authRejected := webai.IsWebSessionAuthError(err) || webai.IsWebSessionAuthError(initialErr)
				if authRejected {
					_ = s.WebAI.MarkSessionExpired(provider, usedMuseAccount.Email)
				} else if result.CookieHeader != "" {
					if saveErr := s.persistWebAICookieRefresh(ctx, provider, usedMuseAccount.Email, result.CookieHeader, true); saveErr != nil {
						progress(60, "MUSE_COOKIE_REFRESH: chưa xác minh/lưu được cookie mới")
					}
				}
				return "", "", err
			}
			refreshMuseSession := func() {
				if result.CookieHeader != "" && strings.TrimSpace(usedMuseAccount.Email) != "" {
					if err := s.persistWebAICookieRefresh(ctx, provider, usedMuseAccount.Email, result.CookieHeader, false); err != nil {
						progress(96, "MUSE_COOKIE_REFRESH: chưa lưu được phiên mới")
					}
				}
			}
			refreshMuseSession()
			if kind == "script" {
				filename := fmt.Sprintf("muse_script_%s.md", safeAIFileID(usedMuseAccount.ID+title))
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
			parsedMediaURL, parseErr := url.Parse(mediaURL)
			if parseErr != nil {
				return "", "", fmt.Errorf("Muse media link is invalid")
			}
			ext := strings.ToLower(filepath.Ext(parsedMediaURL.Path))
			filename := fmt.Sprintf("muse_%s%s", safeAIFileID(usedMuseAccount.ID+title), ext)
			outPath := s.Studio.OutputPath(filename)
			rawPath := outPath
			if kind == "video" {
				filename = fmt.Sprintf("muse_%s.mp4", safeAIFileID(usedMuseAccount.ID+title))
				outPath = s.Studio.OutputPath(filename)
				rawPath = outPath + ".source" + ext
				defer os.Remove(rawPath)
			}
			progress(80, "MUSE_DOWNLOAD: tải media gốc")
			mediaCookie := usedMuseAccount.Cookie
			if result.CookieHeader != "" {
				mediaCookie = result.CookieHeader
			}
			if err := downloadMuseMedia(ctx, mediaCookie, mediaURL, rawPath); err != nil {
				return "", "", err
			}
			if kind == "video" {
				progress(86, "VIDEO_UPSCALE: xuất master "+strings.ToUpper(outputResolution))
				if err := studio.UpscaleVideo(ctx, rawPath, outPath, outputAspect, outputResolution); err != nil {
					return "", "", err
				}
			}
			return filename, "", nil
		})
	if err != nil {
		back("err", "Không tạo được job: "+err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?tab=running&job_id="+url.QueryEscape(id))
}

func (s *Server) generateStudioFlowVideo(ctx context.Context, initial *webai.Account, prompt, aspect string, progress func(string)) (webai.FlowVideoResult, *webai.Account, error, error) {
	if progress == nil {
		progress = func(string) {}
	}
	current := initial
	attempted := map[string]struct{}{initial.ID: {}}
	for {
		result, err := webai.GoogleFlowVideo(ctx, current.Cookie, prompt, aspect, progress)
		initialErr := err
		if cooldown, limited := webai.FlowPreSubmitRateLimitCooldown(err); limited {
			if markErr := s.WebAI.MarkRateLimited(current.ID, cooldown); markErr != nil {
				return result, current, initialErr, fmt.Errorf("Google Flow báo hết quota; không lưu được cooldown tài khoản miễn phí: %w", markErr)
			}
			next, pickErr := s.WebAI.PickActiveAccountExcluding(webai.PlatformGoogleFlow, attempted)
			if pickErr != nil {
				return result, current, initialErr, fmt.Errorf("Google Flow hết quota; dừng vì không còn tài khoản miễn phí khả dụng: %w", pickErr)
			}
			attempted[next.ID] = struct{}{}
			current = next
			progress("FLOW_ACCOUNT_ROTATION: tài khoản miễn phí trước bị giới hạn; chuyển sang tài khoản miễn phí tiếp theo")
			continue
		}
		if webai.CanFallbackToGoogleFlowHeadless(err) && ctx.Err() == nil {
			progress("FLOW_FALLBACK: direct API unavailable before submit; starting headless browser")
			refreshedCookie := result.CookieHeader
			fallbackCookie := current.Cookie
			if refreshedCookie != "" {
				fallbackCookie = refreshedCookie
			}
			result, err = webai.GoogleFlowVideoHeadless(ctx, fallbackCookie, prompt, aspect, result.ProjectID, func(line string) {
				progress(line)
			})
			if result.CookieHeader == "" {
				result.CookieHeader = refreshedCookie
			}
		}
		return result, current, initialErr, err
	}
}
