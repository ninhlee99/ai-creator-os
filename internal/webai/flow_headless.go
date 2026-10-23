package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

const flowAppURL = "https://flow.google.com/"

type flowHeadlessRequest struct {
	Mode       string
	Prompt     string
	Aspect     string
	ProjectID  string
	ImagePaths []string
	VideoPath  string
	ModelLabel string
}

// GoogleFlowVideoHeadless uses the signed-in Flow web UI only after the direct
// API has failed before accepting a generation request.
func GoogleFlowVideoHeadless(ctx context.Context, cookie, prompt, aspect, projectID string, progress func(string)) (FlowVideoResult, error) {
	return GoogleFlowVideoHeadlessWithCookieUpdate(ctx, cookie, prompt, aspect, projectID, progress, nil)
}

func GoogleFlowVideoHeadlessWithCookieUpdate(ctx context.Context, cookie, prompt, aspect, projectID string, progress func(string), onCookieUpdate func(string) error) (FlowVideoResult, error) {
	updater := func(header string, _ []BrowserCookie) error {
		if onCookieUpdate != nil {
			return onCookieUpdate(header)
		}
		return nil
	}
	return GoogleFlowVideoHeadlessWithCookieSnapshot(ctx, cookie, nil, prompt, aspect, projectID, progress, updater)
}

func GoogleFlowVideoHeadlessWithCookieSnapshot(ctx context.Context, cookie string, cookies []BrowserCookie, prompt, aspect, projectID string, progress func(string), updater CookieSnapshotUpdater) (FlowVideoResult, error) {
	return googleFlowVideoHeadless(ctx, cookie, flowHeadlessRequest{
		Mode: "text", Prompt: prompt, Aspect: aspect, ProjectID: projectID,
	}, cookies, progress, updater)
}

// GoogleFlowVideoIngredientsHeadless creates an 8-second clip from the exact
// character/product reference images. The text-only reverse endpoint cannot
// carry these inputs, so this path uses Flow's real Ingredients UI mode.
func GoogleFlowVideoIngredientsHeadless(ctx context.Context, cookie, prompt, aspect string, imagePaths []string, projectID string, progress func(string)) (FlowVideoResult, error) {
	return GoogleFlowVideoIngredientsHeadlessWithCookieUpdate(ctx, cookie, prompt, aspect, imagePaths, projectID, progress, nil)
}

func GoogleFlowVideoIngredientsHeadlessWithCookieUpdate(ctx context.Context, cookie, prompt, aspect string, imagePaths []string, projectID string, progress func(string), onCookieUpdate func(string) error) (FlowVideoResult, error) {
	updater := func(header string, _ []BrowserCookie) error {
		if onCookieUpdate != nil {
			return onCookieUpdate(header)
		}
		return nil
	}
	return GoogleFlowVideoIngredientsHeadlessWithCookieSnapshot(ctx, cookie, nil, prompt, aspect, imagePaths, projectID, progress, updater)
}

func GoogleFlowVideoIngredientsHeadlessWithCookieSnapshot(ctx context.Context, cookie string, cookies []BrowserCookie, prompt, aspect string, imagePaths []string, projectID string, progress func(string), updater CookieSnapshotUpdater) (FlowVideoResult, error) {
	return googleFlowVideoHeadless(ctx, cookie, flowHeadlessRequest{
		Mode: "ingredients", Prompt: prompt, Aspect: aspect, ProjectID: projectID,
		ImagePaths: imagePaths, ModelLabel: "Veo 3.1 Fast",
	}, cookies, progress, updater)
}

// GoogleFlowVideoEditHeadless edits one source segment while using channel
// character images as ingredients. Flow's video-edit model accepts at most
// 10 seconds per edit.
func GoogleFlowVideoEditHeadless(ctx context.Context, cookie, prompt, aspect, videoPath string, imagePaths []string, projectID string, progress func(string)) (FlowVideoResult, error) {
	return GoogleFlowVideoEditHeadlessWithCookieUpdate(ctx, cookie, prompt, aspect, videoPath, imagePaths, projectID, progress, nil)
}

func GoogleFlowVideoEditHeadlessWithCookieUpdate(ctx context.Context, cookie, prompt, aspect, videoPath string, imagePaths []string, projectID string, progress func(string), onCookieUpdate func(string) error) (FlowVideoResult, error) {
	updater := func(header string, _ []BrowserCookie) error {
		if onCookieUpdate != nil {
			return onCookieUpdate(header)
		}
		return nil
	}
	return GoogleFlowVideoEditHeadlessWithCookieSnapshot(ctx, cookie, nil, prompt, aspect, videoPath, imagePaths, projectID, progress, updater)
}

func GoogleFlowVideoEditHeadlessWithCookieSnapshot(ctx context.Context, cookie string, cookies []BrowserCookie, prompt, aspect, videoPath string, imagePaths []string, projectID string, progress func(string), updater CookieSnapshotUpdater) (FlowVideoResult, error) {
	return googleFlowVideoHeadless(ctx, cookie, flowHeadlessRequest{
		Mode: "video_edit", Prompt: prompt, Aspect: aspect, ProjectID: projectID,
		ImagePaths: imagePaths, VideoPath: videoPath, ModelLabel: "Gemini Omni Flash 1.1",
	}, cookies, progress, updater)
}

func validateFlowHeadlessRequest(request flowHeadlessRequest) error {
	if strings.TrimSpace(request.Prompt) == "" {
		return errors.New("Flow headless request needs a prompt")
	}
	if request.Mode != "text" && request.Mode != "ingredients" && request.Mode != "video_edit" {
		return fmt.Errorf("unsupported Flow headless mode %q", request.Mode)
	}
	if request.Mode == "text" {
		if len(request.ImagePaths) != 0 || request.VideoPath != "" {
			return errors.New("text-to-video request cannot contain reference inputs")
		}
		return nil
	}
	if len(request.ImagePaths) < 1 || len(request.ImagePaths) > 3 {
		return fmt.Errorf("Flow %s requires 1-3 channel/product reference images", request.Mode)
	}
	for _, path := range request.ImagePaths {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() || st.Size() <= 0 || st.Size() > 50<<20 {
			return fmt.Errorf("Flow reference image is missing, empty, or exceeds 50 MB: %s", path)
		}
	}
	if request.Mode == "video_edit" {
		st, err := os.Stat(request.VideoPath)
		if err != nil || st.IsDir() || st.Size() <= 0 || st.Size() > 250<<20 {
			return errors.New("Flow video edit source is missing, empty, or exceeds 250 MB")
		}
	} else if request.VideoPath != "" {
		return errors.New("Ingredients-to-Video request cannot contain source video")
	}
	return nil
}

func googleFlowVideoHeadless(ctx context.Context, cookie string, request flowHeadlessRequest, cookies []BrowserCookie, progress func(string), onCookieUpdate CookieSnapshotUpdater) (result FlowVideoResult, retErr error) {
	if strings.TrimSpace(cookie) == "" && len(cookies) == 0 {
		return result, errors.New("Google Flow headless fallback needs a session cookie")
	}
	if err := validateFlowHeadlessRequest(request); err != nil {
		return result, err
	}
	if progress == nil {
		progress = func(string) {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 13*time.Minute)
	defer cancel()
	bin, err := findChromiumBinary()
	if err != nil {
		return result, errors.New("Google Flow direct API failed and no Chrome/Edge binary is installed for headless fallback")
	}
	profile, err := os.MkdirTemp("", "aicos-flow-headless-*")
	if err != nil {
		return result, errors.New("cannot create temporary Flow headless profile")
	}
	defer os.RemoveAll(profile)
	clearChromiumSingletonLocks(profile)
	port, listener, err := pickFreePort()
	if err != nil {
		return result, errors.New("cannot allocate Flow headless debugging port")
	}
	_ = listener.Close()
	args := []string{
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-first-run",
		"--no-default-browser-check", "--disable-sync", "--disable-extensions",
		"--disable-background-networking", "--hide-scrollbars", "--window-size=1440,1100",
		"--remote-debugging-address=127.0.0.1", fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profile, "about:blank",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		return result, errors.New("cannot start Flow headless browser")
	}
	defer terminateBrowser(cmd)

	browserURL, err := waitDebuggerURL(ctx, port)
	if err != nil {
		return result, errors.New("Flow headless browser did not start")
	}
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 8 * time.Second}).DialContext(ctx, browserURL, nil)
	if err != nil {
		return result, errors.New("cannot connect to Flow headless browser")
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
		return result, errors.New("Flow headless browser did not create a page")
	}
	attached, err := client.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, "")
	if err != nil {
		return result, err
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(attached, &session) != nil || session.SessionID == "" {
		return result, errors.New("Flow headless browser did not attach to page")
	}
	client.sessionID = session.SessionID
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable"} {
		if _, err := client.call(ctx, method, map[string]any{}, client.sessionID); err != nil {
			return result, err
		}
	}
	if len(cookies) == 0 {
		cookies = mergeCookieJarHeader(nil, cookie, PlatformGoogleFlow)
	}
	if len(cookies) == 0 {
		return result, errors.New("saved Google Flow cookie header is empty")
	}
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": browserCookieSetParams(PlatformGoogleFlow, cookies)}, client.sessionID); err != nil {
		return result, errors.New("Flow headless browser rejected saved session cookies")
	}
	initialSnapshot, err := browserCookieSnapshotFromCDP(ctx, client)
	if err != nil || len(initialSnapshot) == 0 {
		initialSnapshot = normalizeCookieJar(cookies)
	}
	monitorCtx, monitor := startBrowserCookieMonitor(ctx, client, initialSnapshot, browserCookieSnapshotFromCDP, PlatformGoogleFlow, onCookieUpdate)
	ctx = monitorCtx
	defer func() {
		if monitorErr := monitor.stop(); monitorErr != nil {
			retErr = fmt.Errorf("persist refreshed Flow cookies: %w", monitorErr)
		}
		refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if refreshed, readErr := browserCookieSnapshotFromCDP(refreshCtx, client); readErr == nil && len(refreshed) > 0 {
			result.CookieJar = refreshed
			result.CookieHeader = browserCookieHeaderForPlatform(PlatformGoogleFlow, refreshed)
			if !reflect.DeepEqual(refreshed, monitor.current()) && onCookieUpdate != nil {
				if err := onCookieUpdate(result.CookieHeader, refreshed); err != nil {
					retErr = fmt.Errorf("persist refreshed Flow cookies: %w", err)
				}
			}
		}
	}()
	startURL := flowAppURL
	if strings.TrimSpace(request.ProjectID) != "" {
		startURL = flowAppURL + "project/" + url.PathEscape(request.ProjectID)
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": startURL}, client.sessionID); err != nil {
		return result, errors.New("Flow headless page navigation failed")
	}
	progress("FLOW_HEADLESS: mở Chromium ẩn với phiên Google đã lưu")
	if request.ProjectID == "" {
		created, err := flowWaitAndClick(ctx, client, `(() => {const xs=[...document.querySelectorAll('button.new-project-button,button[aria-label*="new project" i],button')];const e=xs.find(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&!x.disabled&&/new project|create project|tạo dự án|dự án mới/i.test((x.innerText||'')+' '+(x.getAttribute('aria-label')||''))});if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`, 60*time.Second)
		if err != nil {
			return result, fmt.Errorf("Flow project controls did not load: %w", err)
		}
		if !created {
			return result, errors.New("Flow new-project button was not found; check saved session and Flow access")
		}
		request.ProjectID, err = flowWaitForProjectID(ctx, client, 45*time.Second)
		if err != nil {
			return result, err
		}
	}
	result.ProjectID = request.ProjectID
	composerReady, err := flowWaitForValue(ctx, client, `!!document.querySelector('.ProseMirror,div[contenteditable="true"],textarea')`, 60*time.Second)
	if err != nil || composerReady != true {
		return result, errors.New("Flow prompt editor did not load; check saved Google session and access")
	}
	baselineValue, err := client.evaluate(ctx, `(() => [...document.querySelectorAll('flow-video-tile video,video,a[href*="flow-content.google/video/"]')].map(e=>e.currentSrc||e.src||e.href).filter(Boolean))()`)
	if err != nil {
		return result, errors.New("cannot read Flow project before generation")
	}
	baseline := stringSet(baselineValue)
	if request.Mode != "text" {
		if err := flowSelectModelAndMode(ctx, client, request); err != nil {
			return result, err
		}
		if err := flowAttachRequestFiles(ctx, client, request); err != nil {
			return result, err
		}
	}
	if err := flowSetAspect(ctx, client, request.Aspect); err != nil {
		return result, fmt.Errorf("Flow cannot apply requested aspect ratio: %w", err)
	}
	if _, err := client.evaluate(ctx, `(() => {const e=document.querySelector('.ProseMirror,div[contenteditable="true"],textarea');if(!e)return false;e.focus();document.execCommand('selectAll',false,null);document.execCommand('delete',false,null);return true})()`); err != nil {
		return result, errors.New("could not focus Flow prompt editor")
	}
	if _, err := client.call(ctx, "Input.insertText", map[string]any{"text": request.Prompt}, client.sessionID); err != nil {
		return result, errors.New("could not enter prompt in Flow editor")
	}
	clicked, err := flowWaitAndClick(ctx, client, `(() => {const e=document.querySelector('button.generate-icon-button,button[aria-label="Start generation"]');if(!e||e.disabled||e.getAttribute('aria-disabled')==='true')return null;const r=e.getBoundingClientRect();return r.width>0&&r.height>0?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`, 30*time.Second)
	if err != nil || !clicked {
		return result, errors.New("Flow generation button did not become ready")
	}
	progress("FLOW_HEADLESS: Flow đã nhận prompt; đang đợi video hoàn tất")
	deadline := time.Now().Add(12 * time.Minute)
	nextProgress := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		mediaValue, err := client.evaluate(ctx, `(() => {const all=[...document.querySelectorAll('flow-video-tile video,video,a[href*="flow-content.google/video/"]')];return all.map(e=>e.currentSrc||e.src||e.href).filter(Boolean)})()`)
		if err == nil {
			for _, raw := range valueStrings(mediaValue) {
				if baseline[raw] || !isGoogleMediaURL(raw) {
					continue
				}
				result.MediaURL = raw
				if _, path, ok := strings.Cut(strings.SplitN(raw, "?", 2)[0], "/video/"); ok {
					result.MediaID = strings.TrimSuffix(path, "/")
				}
				result.CookieHeader = flowUpdatedCookieHeader(ctx, client)
				return result, nil
			}
		}
		if message := flowHeadlessRateLimitMessage(ctx, client); message != "" {
			return result, &flowUIRateLimitError{message: message}
		}
		// Flow mounts video elements lazily when the newest result tile is hovered.
		bounds, _ := client.evaluate(ctx, `(() => {const xs=[...document.querySelectorAll('flow-video-tile')];const e=xs[xs.length-1];if(!e)return null;const r=e.getBoundingClientRect();return r.width>0&&r.height>0?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`)
		if point, ok := bounds.(map[string]any); ok {
			_, _ = client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point["x"], "y": point["y"]}, client.sessionID)
		}
		if time.Now().After(nextProgress) {
			progress("FLOW_HEADLESS_WAIT: đang chờ Flow render video")
			nextProgress = time.Now().Add(10 * time.Second)
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return result, errors.New("Flow headless UI timed out waiting for a downloadable video")
}

func flowHeadlessRateLimitMessage(ctx context.Context, client *browserCDPClient) string {
	value, err := client.evaluate(ctx, `(() => {
	  const visible = e => { const r=e.getBoundingClientRect(), s=getComputedStyle(e); return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none' };
	  return [...document.querySelectorAll('[role="alert"],[role="status"],[aria-live="assertive"],[aria-live="polite"],[class*="toast"],[class*="snackbar"],[data-state="open"]')]
	    .filter(visible).map(e=>(e.innerText||e.textContent||'').trim()).filter(Boolean).join('\n').slice(0,1200);
	})()`)
	if err != nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	flat := strings.Join(strings.Fields(text), " ")
	lower := strings.ToLower(flat)
	phrases := []string{
		"rate limit", "too many requests", "quota exceeded", "quota exhausted", "limit reached",
		"reached your limit", "exceeded your limit", "limit has been reached", "hit your daily limit",
		"reached your daily limit", "daily limit reached", "daily limit exceeded", "weekly limit reached",
		"monthly limit reached", "generation limit reached", "generation limit exceeded", "out of generations",
		"no more generations", "out of credits", "run out of credits", "ran out of credits",
		"no credits remaining", "not enough credits", "insufficient credits", "try again tomorrow",
		"đã hết lượt", "hết lượt", "đã đạt giới hạn", "vượt giới hạn", "đã sử dụng hết",
		"không đủ tín dụng", "hết tín dụng",
	}
	for _, phrase := range phrases {
		if strings.Contains(lower, phrase) {
			if len(flat) > 300 {
				flat = flat[:300]
			}
			return flat
		}
	}
	return ""
}
