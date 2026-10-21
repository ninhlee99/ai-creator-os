package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const flowAppURL = "https://flow.google.com/"

// GoogleFlowVideoHeadless uses the signed-in Flow web UI only after the direct
// API has failed before accepting a generation request.
func GoogleFlowVideoHeadless(ctx context.Context, cookie, prompt, aspect, projectID string, progress func(string)) (FlowVideoResult, error) {
	var result FlowVideoResult
	if strings.TrimSpace(cookie) == "" || strings.TrimSpace(prompt) == "" {
		return result, errors.New("Google Flow headless fallback needs a session cookie and prompt")
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
	client := &museCDP{conn: conn}
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
	cookies := flowBrowserCookies(cookie)
	if len(cookies) == 0 {
		return result, errors.New("saved Google Flow cookie header is empty")
	}
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": cookies}, client.sessionID); err != nil {
		return result, errors.New("Flow headless browser rejected saved session cookies")
	}
	startURL := flowAppURL
	if strings.TrimSpace(projectID) != "" {
		startURL = flowAppURL + "project/" + url.PathEscape(projectID)
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": startURL}, client.sessionID); err != nil {
		return result, errors.New("Flow headless page navigation failed")
	}
	progress("FLOW_HEADLESS: mở Chromium ẩn với phiên Google đã lưu")
	if projectID == "" {
		created, err := flowWaitAndClick(ctx, client, `(() => {const xs=[...document.querySelectorAll('button.new-project-button,button[aria-label*="new project" i],button')];const e=xs.find(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&!x.disabled&&/new project|create project|tạo dự án|dự án mới/i.test((x.innerText||'')+' '+(x.getAttribute('aria-label')||''))});if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`, 60*time.Second)
		if err != nil {
			return result, fmt.Errorf("Flow project controls did not load: %w", err)
		}
		if !created {
			return result, errors.New("Flow new-project button was not found; check saved session and Flow access")
		}
		projectID, err = flowWaitForProjectID(ctx, client, 45*time.Second)
		if err != nil {
			return result, err
		}
	}
	result.ProjectID = projectID
	composerReady, err := flowWaitForValue(ctx, client, `!!document.querySelector('.ProseMirror,div[contenteditable="true"],textarea')`, 60*time.Second)
	if err != nil || composerReady != true {
		return result, errors.New("Flow prompt editor did not load; check saved Google session and access")
	}
	baselineValue, err := client.evaluate(ctx, `(() => [...document.querySelectorAll('flow-video-tile video,video,a[href*="flow-content.google/video/"]')].map(e=>e.currentSrc||e.src||e.href).filter(Boolean))()`)
	if err != nil {
		return result, errors.New("cannot read Flow project before generation")
	}
	baseline := stringSet(baselineValue)
	if err := flowSetAspect(ctx, client, aspect); err != nil {
		return result, fmt.Errorf("Flow cannot apply requested aspect ratio: %w", err)
	}
	if _, err := client.evaluate(ctx, `(() => {const e=document.querySelector('.ProseMirror,div[contenteditable="true"],textarea');if(!e)return false;e.focus();document.execCommand('selectAll',false,null);document.execCommand('delete',false,null);return true})()`); err != nil {
		return result, errors.New("could not focus Flow prompt editor")
	}
	if _, err := client.call(ctx, "Input.insertText", map[string]any{"text": prompt}, client.sessionID); err != nil {
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
				return result, nil
			}
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

func flowBrowserCookies(header string) []map[string]any {
	var out []map[string]any
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		for _, cookieURL := range []string{flowAppURL, "https://labs.google/"} {
			out = append(out, map[string]any{"name": name, "value": value, "url": cookieURL, "path": "/", "secure": true, "httpOnly": true})
		}
	}
	return out
}

func flowWaitForValue(ctx context.Context, client *museCDP, expression string, timeout time.Duration) (any, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil && value == true {
			return value, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, errors.New("Flow page did not reach the expected state")
}

func flowWaitAndClick(ctx context.Context, client *museCDP, expression string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil {
			if point, ok := value.(map[string]any); ok {
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": point["x"], "y": point["y"]}, client.sessionID); err != nil {
					return false, err
				}
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": point["x"], "y": point["y"], "button": "left", "clickCount": 1}, client.sessionID); err != nil {
					return false, err
				}
				if _, err := client.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": point["x"], "y": point["y"], "button": "left", "clickCount": 1}, client.sessionID); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return false, nil
}

func flowWaitForProjectID(ctx context.Context, client *museCDP, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		value, err := client.evaluate(ctx, `location.pathname.match(/\/project\/([^/?]+)/)?.[1]||''`)
		if err == nil {
			if id, ok := value.(string); ok && id != "" {
				return id, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", errors.New("Flow did not open a new project")
}

func flowSetAspect(ctx context.Context, client *museCDP, aspect string) error {
	ratio := "9:16"
	if aspect == "VIDEO_ASPECT_RATIO_LANDSCAPE" {
		ratio = "16:9"
	} else if aspect == "VIDEO_ASPECT_RATIO_SQUARE" {
		ratio = "1:1"
	}
	settings := `(() => {const e=document.querySelector('button[aria-label="Settings trigger"],.settings-trigger-button');return e?(e.innerText||e.getAttribute('aria-label')||''):''})()`
	if value, err := client.evaluate(ctx, settings); err == nil {
		if text, ok := value.(string); ok && (strings.Contains(text, ratio) || strings.Contains(text, "crop_"+strings.ReplaceAll(ratio, ":", "_"))) {
			return nil
		}
	}
	opened, err := flowWaitAndClick(ctx, client, `(() => {const e=document.querySelector('button[aria-label="Settings trigger"],.settings-trigger-button');if(!e)return null;const r=e.getBoundingClientRect();return r.width>0&&r.height>0?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`, 5*time.Second)
	if err != nil || !opened {
		return errors.New("Flow settings control was not found")
	}
	videoScript := `(() => {const e=[...document.querySelectorAll('.cdk-overlay-pane mat-button-toggle')].find(x=>/video/i.test(x.innerText||''));if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`
	_, _ = flowWaitAndClick(ctx, client, videoScript, 5*time.Second)
	ratioJSON, _ := json.Marshal(ratio)
	ratioScript := `(() => {const t=` + string(ratioJSON) + `;const e=[...document.querySelectorAll('.cdk-overlay-pane mat-button-toggle')].find(x=>(x.innerText||'').includes(t));if(!e)return null;const b=e.querySelector('button')||e;const r=b.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`
	clicked, err := flowWaitAndClick(ctx, client, ratioScript, 5*time.Second)
	if _, closeErr := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Escape", "code": "Escape", "windowsVirtualKeyCode": 27}, client.sessionID); closeErr != nil {
		return closeErr
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Escape", "code": "Escape", "windowsVirtualKeyCode": 27}, client.sessionID)
	if err != nil {
		return err
	}
	if !clicked {
		return errors.New("Flow aspect ratio toggle was not found")
	}
	return nil
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range valueStrings(value) {
		result[item] = true
	}
	return result
}

func valueStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok && value != "" {
			result = append(result, value)
		}
	}
	return result
}
