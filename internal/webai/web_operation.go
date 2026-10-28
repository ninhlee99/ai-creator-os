package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebAICapabilities reports UI features documented for each provider. Plan,
// region, and account settings can still remove provider-side access.
type WebAICapabilities struct {
	ImageInput  bool `json:"image_input"`
	VideoInput  bool `json:"video_input"`
	ImageOutput bool `json:"image_output"`
	VideoOutput bool `json:"video_output"`
}

var webAICapabilities = map[string]WebAICapabilities{
	PlatformChatGPT: {ImageInput: true, VideoInput: true, ImageOutput: true},
	PlatformClaude:  {ImageInput: true},
	PlatformGemini:  {ImageInput: true, VideoInput: true, ImageOutput: true, VideoOutput: true},
	PlatformGrok:    {ImageInput: true, VideoInput: true, ImageOutput: true, VideoOutput: true},
}

func WebAIOperationCapabilities(platform string) (WebAICapabilities, bool) {
	capabilities, ok := webAICapabilities[strings.ToLower(strings.TrimSpace(platform))]
	return capabilities, ok
}

type WebAIOperationRequest struct {
	Prompt              string
	Files               []string
	OutputType          string // text | image | video
	OutputPath          string // required for image/video; extension is selected from downloaded media
	ConversationURL     string // optional provider conversation URL to continue in
	requireRadarMarkers bool
}

type WebAIOperationResult struct {
	Text            string
	MediaPath       string
	ConversationURL string
}

// UseWebAIInWebUI submits prompt and optional local image/video files through
// the signed-in provider website. It does not call provider chat APIs.
func UseWebAIInWebUI(ctx context.Context, platform, cookie string, savedCookies []BrowserCookie, request WebAIOperationRequest, updater CookieSnapshotUpdater) (result WebAIOperationResult, retErr error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	page, ok := imageAnalysisPages[platform]
	if !ok {
		return result, fmt.Errorf("Web UI operation is unsupported for provider %q", platform)
	}
	capabilities, ok := WebAIOperationCapabilities(platform)
	if !ok {
		return result, fmt.Errorf("Web UI capabilities are missing for provider %q", platform)
	}
	request.OutputType = strings.ToLower(strings.TrimSpace(request.OutputType))
	if request.OutputType == "" {
		request.OutputType = "text"
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return result, errors.New("Web AI needs a prompt")
	}
	if len(request.Files) > 10 {
		return result, errors.New("Web AI accepts at most 10 input files per operation")
	}
	switch request.OutputType {
	case "text":
	case "image":
		if !capabilities.ImageOutput {
			return result, fmt.Errorf("%s web UI does not provide image generation", platform)
		}
	case "video":
		if !capabilities.VideoOutput {
			return result, fmt.Errorf("%s web UI does not provide video generation", platform)
		}
	default:
		return result, fmt.Errorf("unsupported Web AI output type %q", request.OutputType)
	}
	if request.OutputType != "text" && strings.TrimSpace(request.OutputPath) == "" {
		return result, errors.New("Web AI media generation needs an output path")
	}
	files, err := validateWebAIInputFiles(platform, request.Files, capabilities)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(cookie) == "" && len(savedCookies) == 0 {
		return result, errors.New("Web AI operation needs a saved session")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := 3 * time.Minute
	if request.OutputType == "image" {
		timeout = 6 * time.Minute
	} else if request.OutputType == "video" {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	bin, err := findChromiumBinary()
	if err != nil {
		return result, fmt.Errorf("Web AI operation needs Chrome, Edge, Brave, or Chromium: %w", err)
	}
	profile, releaseProfile, err := acquireAccountBrowserProfile(platform, cookie, savedCookies)
	if err != nil {
		return result, errors.New("cannot create temporary Web AI browser profile")
	}
	defer releaseProfile()
	port, listener, err := pickFreePort()
	if err != nil {
		return result, errors.New("cannot allocate Web AI browser debugging port")
	}
	_ = listener.Close()
	args := optimizedHeadlessBrowserArgs(port, profile, "1440,1100")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	configureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		return result, errors.New("cannot start Web AI headless browser")
	}
	defer terminateBrowser(cmd)
	browserURL, err := waitDebuggerURL(ctx, port, profile)
	if err != nil {
		return result, err
	}
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 8 * time.Second}).DialContext(ctx, browserURL, nil)
	if err != nil {
		return result, errors.New("cannot connect to Web AI headless browser")
	}
	defer conn.Close()
	client := &browserCDPClient{conn: conn}
	created, err := client.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
	if err != nil {
		return result, err
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(created, &target) != nil || target.TargetID == "" {
		return result, errors.New("Web AI browser did not create a page")
	}
	attached, err := client.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, "")
	if err != nil {
		return result, err
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(attached, &session) != nil || session.SessionID == "" {
		return result, errors.New("Web AI browser did not attach to page")
	}
	client.sessionID = session.SessionID
	cookies := normalizeCookieJar(savedCookies)
	if len(cookies) == 0 {
		cookies = mergeCookieJarHeader(nil, cookie, platform)
	}
	if len(cookies) == 0 {
		return result, errors.New("saved Web AI cookie snapshot is empty")
	}
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable", "DOM.enable"} {
		if _, err := client.call(ctx, method, map[string]any{}, client.sessionID); err != nil {
			return result, err
		}
	}
	client.enableStealth(ctx)
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": browserCookieSetParams(platform, cookies)}, client.sessionID); err != nil {
		return result, fmt.Errorf("Web AI browser rejected saved session cookies: %w", err)
	}
	startURL := webAIOperationStartURL(page, request.OutputType, request.ConversationURL)
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": startURL}, client.sessionID); err != nil {
		return result, errors.New("Web AI page navigation failed")
	}
	if err := waitForWebAIComposer(ctx, client, page.composer, 45*time.Second); err != nil {
		return result, fmt.Errorf("%s composer did not load; check saved session and web access: %w", platform, err)
	}
	if err := selectWebAIOperationMode(ctx, client, request.OutputType, page); err != nil {
		return result, fmt.Errorf("select %s mode in %s: %w", request.OutputType, platform, err)
	}
	prompt := webAIOperationPrompt(request.OutputType, request.Prompt)
	baseline, err := webAIResponseCount(ctx, client, page.responses, prompt, request.requireRadarMarkers)
	if err != nil {
		return result, fmt.Errorf("cannot read %s conversation before prompt: %w", platform, err)
	}
	if len(files) > 0 {
		if err := attachWebAIFiles(ctx, client, files); err != nil {
			return result, fmt.Errorf("attach files in %s: %w", platform, err)
		}
	}
	if err := setWebAIPrompt(ctx, client, page.composer, prompt); err != nil {
		return result, fmt.Errorf("enter prompt in %s: %w", platform, err)
	}
	if err := submitWebAIPrompt(ctx, client, platform); err != nil {
		return result, fmt.Errorf("submit prompt in %s: %w", platform, err)
	}
	operationResult, err := waitWebAIResult(ctx, client, page, prompt, baseline, request.OutputType, timeout, request.requireRadarMarkers)
	if err != nil {
		return result, fmt.Errorf("wait for %s response: %w", platform, err)
	}
	result.Text = operationResult.Text
	if location, locationErr := client.evaluate(ctx, `location.href`); locationErr == nil {
		if currentURL, ok := location.(string); ok {
			result.ConversationURL = webAIConversationURL(page, currentURL)
		}
	}
	if request.OutputType != "text" {
		mediaURL := ""
		if request.OutputType == "image" && len(operationResult.Images) > 0 {
			mediaURL = operationResult.Images[len(operationResult.Images)-1]
		}
		if request.OutputType == "video" && len(operationResult.Videos) > 0 {
			mediaURL = operationResult.Videos[len(operationResult.Videos)-1]
		}
		if mediaURL == "" {
			return result, fmt.Errorf("%s replied without a downloadable %s; account access or provider UI may not support this request", platform, request.OutputType)
		}
		result.MediaPath, err = downloadWebAIMediaInBrowser(ctx, client, mediaURL, request.OutputPath, request.OutputType)
		if err != nil {
			return result, fmt.Errorf("download %s output from %s: %w", request.OutputType, platform, err)
		}
	}
	if updater != nil {
		refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer refreshCancel()
		if refreshed, readErr := browserCookieSnapshotFromCDP(refreshCtx, client); readErr == nil && len(refreshed) > 0 {
			if updateErr := updater(imageAnalysisCookieHeader(platform, refreshed), refreshed); updateErr != nil {
				log.Printf("webai operation: could not persist refreshed %s cookies: %v", platform, updateErr)
			}
		}
	}
	return result, nil
}

func webAIOperationPrompt(outputType, prompt string) string {
	switch outputType {
	case "image":
		return "Create one image from this brief. Follow reference files and preserve requested details.\n\n" + prompt
	case "video":
		return "Create one video from this brief. Follow reference files and preserve requested details.\n\n" + prompt
	default:
		return prompt
	}
}

func webAIOperationURL(page imageAnalysisPage, outputType string) string {
	switch outputType {
	case "image":
		if page.imageURL != "" {
			return page.imageURL
		}
	case "video":
		if page.videoURL != "" {
			return page.videoURL
		}
	}
	return page.url
}

func webAIOperationStartURL(page imageAnalysisPage, outputType, conversationURL string) string {
	if candidate := webAIConversationURL(page, conversationURL); candidate != "" {
		return candidate
	}
	return webAIOperationURL(page, outputType)
}

func webAIConversationURL(page imageAnalysisPage, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	parsed, err := url.Parse(candidate)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	allowed, err := url.Parse(page.url)
	if err != nil || !strings.EqualFold(parsed.Hostname(), allowed.Hostname()) {
		return ""
	}
	return parsed.String()
}

func validateWebAIInputFiles(platform string, paths []string, capabilities WebAICapabilities) ([]string, error) {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve Web AI file path: %w", err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return nil, fmt.Errorf("Web AI file is missing, empty, or not a regular file: %s", path)
		}
		ext := strings.ToLower(filepath.Ext(absolute))
		isImage := strings.Contains(" .png .jpg .jpeg .webp ", " "+ext+" ")
		isVideo := strings.Contains(" .mp4 .mov .webm .m4v ", " "+ext+" ")
		if isImage {
			if !capabilities.ImageInput {
				return nil, fmt.Errorf("%s web UI does not accept image uploads", platform)
			}
			if info.Size() > 20<<20 {
				return nil, fmt.Errorf("Web AI image exceeds 20 MB: %s", path)
			}
		} else if isVideo {
			if !capabilities.VideoInput {
				return nil, fmt.Errorf("%s web UI does not accept video uploads", platform)
			}
			if info.Size() > 100<<20 {
				return nil, fmt.Errorf("Web AI video exceeds 100 MB: %s", path)
			}
		} else {
			return nil, fmt.Errorf("unsupported Web AI file type: %s", path)
		}
		resolved = append(resolved, absolute)
	}
	return resolved, nil
}

type webAIObservedResult struct {
	Text   string
	Images []string
	Videos []string
}

func waitWebAIResult(ctx context.Context, client *browserCDPClient, page imageAnalysisPage, prompt string, baseline int, outputType string, timeout time.Duration, requireRadarMarkers bool) (webAIObservedResult, error) {
	expression := webAIResponseExpression(page.responses, prompt, mustJSON(page.busy), requireRadarMarkers)
	deadline := time.Now().Add(timeout)
	lastText := ""
	stableAt := time.Time{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return webAIObservedResult{}, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil {
			var state struct {
				Count  int      `json:"count"`
				Text   string   `json:"text"`
				Busy   bool     `json:"busy"`
				Images []string `json:"images"`
				Videos []string `json:"videos"`
			}
			if raw, marshalErr := json.Marshal(value); marshalErr == nil && json.Unmarshal(raw, &state) == nil && state.Count > baseline {
				cleanText := strings.TrimSpace(state.Text)
				foundMedia := outputType == "image" && len(state.Images) > 0 || outputType == "video" && len(state.Videos) > 0
				if !state.Busy && (foundMedia || outputType == "text" && cleanText != "") {
					if foundMedia {
						return webAIObservedResult{Text: cleanText, Images: state.Images, Videos: state.Videos}, nil
					}
					if cleanText == lastText {
						if stableAt.IsZero() {
							stableAt = time.Now()
						}
						if time.Since(stableAt) >= 1400*time.Millisecond {
							return webAIObservedResult{Text: cleanText, Images: state.Images, Videos: state.Videos}, nil
						}
					} else {
						lastText = cleanText
						stableAt = time.Time{}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return webAIObservedResult{}, ctx.Err()
		case <-time.After(650 * time.Millisecond):
		}
	}
	return webAIObservedResult{}, errors.New("response wait timed out")
}

func selectWebAIOperationMode(ctx context.Context, client *browserCDPClient, outputType string, page imageAnalysisPage) error {
	var labels []string
	switch outputType {
	case "image":
		labels = page.imageModeLabels
	case "video":
		labels = page.videoModeLabels
	default:
		return nil
	}
	if len(labels) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const norm=s=>(s||'').replace(/\s+/g,' ').trim().toLowerCase();const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};const xs=[...document.querySelectorAll('button,[role="button"],[role="menuitem"],a')].filter(visible);const e=xs.find(x=>{const t=norm((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('title')||''));return labels.some(s=>t===s||t.startsWith(s+' '))&&!x.disabled&&x.getAttribute('aria-disabled')!=='true'});if(!e)return false;e.click();return true})()`
	value, err := client.evaluate(ctx, expression)
	if err != nil {
		return err
	}
	if value != true && outputType == "video" {
		return errors.New("video creation mode was not found")
	}
	if value == true {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
	return nil
}

func downloadWebAIMediaInBrowser(ctx context.Context, client *browserCDPClient, mediaURL, outputPath, outputType string) (string, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return "", errors.New("provider output has no media URL")
	}
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	downloadDir, err := os.MkdirTemp(outputDir, ".webai-download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(downloadDir)
	if _, err := client.call(ctx, "Page.setDownloadBehavior", map[string]any{"behavior": "allow", "downloadPath": downloadDir}, client.sessionID); err != nil {
		return "", fmt.Errorf("enable browser downloads: %w", err)
	}
	providerDownload := `(() => {const url=` + mustJSON(mediaURL) + `;const artifact=[...document.querySelectorAll('img,video,video source')].find(e=>(e.currentSrc||e.src||'')===url);if(!artifact)return false;const labels=['download','save','tải xuống','tải về'];for(let n=artifact;n&&n!==document.body;n=n.parentElement){const button=[...n.querySelectorAll('button,[role="button"],a')].find(e=>{const r=e.getBoundingClientRect();const t=((e.innerText||'')+' '+(e.getAttribute('aria-label')||'')+' '+(e.getAttribute('title')||'')).toLowerCase();return r.width>0&&r.height>0&&!e.disabled&&labels.some(s=>t.includes(s))});if(button){button.click();return true}}return false})()`
	_, _ = client.evaluate(ctx, providerDownload)
	if path, ok, err := waitForDownloadedWebAIMedia(ctx, downloadDir, outputPath, outputType, 2*time.Second); err != nil || ok {
		return path, err
	}
	filename := "aicos-generated-image.png"
	if outputType == "video" {
		filename = "aicos-generated-video.mp4"
	}
	expression := `(() => {const url=` + mustJSON(mediaURL) + `;const a=document.createElement('a');a.href=url;a.download=` + mustJSON(filename) + `;a.style.display='none';document.body.appendChild(a);a.click();a.remove();return true})()`
	if value, evalErr := client.evaluate(ctx, expression); evalErr != nil || value != true {
		if evalErr != nil {
			return "", evalErr
		}
		return "", errors.New("browser did not start media download")
	}
	path, _, err := waitForDownloadedWebAIMedia(ctx, downloadDir, outputPath, outputType, 3*time.Minute)
	return path, err
}

func waitForDownloadedWebAIMedia(ctx context.Context, downloadDir, outputPath, outputType string, timeout time.Duration) (string, bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, readErr := os.ReadDir(downloadDir)
		if readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() || strings.HasSuffix(entry.Name(), ".crdownload") || strings.HasSuffix(entry.Name(), ".tmp") {
					continue
				}
				candidate := filepath.Join(downloadDir, entry.Name())
				info, statErr := os.Stat(candidate)
				if statErr != nil || info.Size() <= 0 {
					continue
				}
				limit := int64(400 << 20)
				if outputType == "image" {
					limit = 50 << 20
				}
				if info.Size() > limit {
					return "", true, fmt.Errorf("generated %s exceeds %d MB", outputType, limit>>20)
				}
				ext := detectedWebAIMediaExtension(candidate, outputType)
				if ext == "" {
					return "", true, fmt.Errorf("provider download is not a valid %s file", outputType)
				}
				finalPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ext
				if err := os.Rename(candidate, finalPath); err != nil {
					return "", true, err
				}
				return finalPath, true, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return "", false, nil
}

func detectedWebAIMediaExtension(path, outputType string) string {
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		var signature [16]byte
		n, _ := file.Read(signature[:])
		data := signature[:n]
		if outputType == "image" {
			switch {
			case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
				return ".png"
			case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
				return ".jpg"
			case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
				return ".webp"
			}
		} else {
			if len(data) >= 8 && string(data[4:8]) == "ftyp" {
				return ".mp4"
			}
			if len(data) >= 4 && string(data[:4]) == "\x1aE\xdf\xa3" {
				return ".webm"
			}
		}
	}
	return ""
}
