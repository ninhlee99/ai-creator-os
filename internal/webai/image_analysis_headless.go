package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type imageAnalysisPage struct {
	url             string
	composer        string
	responses       string
	busy            string
	fileInputAccept string
}

var imageAnalysisPages = map[string]imageAnalysisPage{
	PlatformChatGPT: {
		url: "https://chatgpt.com/", composer: "#prompt-textarea, [data-testid='prompt-textarea'], textarea, [contenteditable='true']",
		responses: "[data-message-author-role='assistant']", busy: "[data-testid*='stop'], button[aria-label*='Stop generating' i]",
		fileInputAccept: "image/",
	},
	PlatformClaude: {
		url: "https://claude.ai/new", composer: "[contenteditable='true'], [data-testid*='chat-input'] textarea, textarea",
		responses: ".font-claude-response", busy: "button[aria-label*='Stop' i], [data-testid*='stop']",
		fileInputAccept: "image/",
	},
	PlatformGemini: {
		url: "https://gemini.google.com/app", composer: "rich-textarea [contenteditable='true'], .ql-editor, textarea, [contenteditable='true']",
		responses: "model-response, .response-container", busy: "button[aria-label*='Stop' i], mat-spinner",
		fileInputAccept: "image/",
	},
	PlatformGrok: {
		url: "https://grok.com/", composer: "textarea, [contenteditable='true']",
		responses: "main [data-message-author-role='assistant'], main [data-role='assistant'], main [data-testid*='assistant' i], [data-testid*='conversation-turn'], main article", busy: "button[aria-label*='Stop' i], [data-testid*='stop']",
		fileInputAccept: "image/",
	},
}

// AnalyzeImagesInWebUI submits local image files and prompt through the
// provider's signed-in website. It uses no provider chat API or undocumented
// web endpoint.
func AnalyzeImagesInWebUI(ctx context.Context, platform, cookie string, savedCookies []BrowserCookie, prompt string, imagePaths []string, updater CookieSnapshotUpdater) (answer string, retErr error) {
	page, ok := imageAnalysisPages[strings.ToLower(strings.TrimSpace(platform))]
	if !ok {
		return "", fmt.Errorf("image analysis is unsupported for Web AI platform %q", platform)
	}
	if strings.TrimSpace(prompt) == "" || len(imagePaths) == 0 || len(imagePaths) > 10 {
		return "", errors.New("Web AI image analysis needs prompt and 1-10 images")
	}
	resolvedImagePaths := make([]string, 0, len(imagePaths))
	for _, path := range imagePaths {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", fmt.Errorf("resolve Web AI image path: %w", absErr)
		}
		info, err := os.Stat(absolute)
		if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > 20<<20 {
			return "", fmt.Errorf("Web AI image is missing, empty, or exceeds 20 MB: %s", path)
		}
		resolvedImagePaths = append(resolvedImagePaths, absolute)
	}
	if strings.TrimSpace(cookie) == "" && len(savedCookies) == 0 {
		return "", errors.New("Web AI image analysis needs a saved session")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	bin, err := findChromiumBinary()
	if err != nil {
		return "", fmt.Errorf("Web AI image analysis needs Chrome, Edge, Brave, or Chromium: %w", err)
	}
	profile, err := os.MkdirTemp("", "aicos-webai-image-*")
	if err != nil {
		return "", errors.New("cannot create temporary Web AI browser profile")
	}
	defer os.RemoveAll(profile)
	clearChromiumSingletonLocks(profile)
	port, listener, err := pickFreePort()
	if err != nil {
		return "", errors.New("cannot allocate Web AI browser debugging port")
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
		return "", errors.New("cannot start Web AI headless browser")
	}
	defer terminateBrowser(cmd)
	browserURL, err := waitDebuggerURL(ctx, port)
	if err != nil {
		return "", err
	}
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 8 * time.Second}).DialContext(ctx, browserURL, nil)
	if err != nil {
		return "", errors.New("cannot connect to Web AI headless browser")
	}
	defer conn.Close()
	client := &browserCDPClient{conn: conn}
	created, err := client.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
	if err != nil {
		return "", err
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(created, &target) != nil || target.TargetID == "" {
		return "", errors.New("Web AI browser did not create a page")
	}
	attached, err := client.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, "")
	if err != nil {
		return "", err
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(attached, &session) != nil || session.SessionID == "" {
		return "", errors.New("Web AI browser did not attach to page")
	}
	client.sessionID = session.SessionID
	cookies := normalizeCookieJar(savedCookies)
	if len(cookies) == 0 {
		cookies = mergeCookieJarHeader(nil, cookie, platform)
	}
	if len(cookies) == 0 {
		return "", errors.New("saved Web AI cookie snapshot is empty")
	}
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable", "DOM.enable"} {
		if _, err := client.call(ctx, method, map[string]any{}, client.sessionID); err != nil {
			return "", err
		}
	}
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": browserCookieSetParams(platform, cookies)}, client.sessionID); err != nil {
		return "", fmt.Errorf("Web AI browser rejected saved session cookies: %w", err)
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": page.url}, client.sessionID); err != nil {
		return "", errors.New("Web AI page navigation failed")
	}
	if err := waitForWebAIComposer(ctx, client, page.composer, 45*time.Second); err != nil {
		return "", fmt.Errorf("%s composer did not load; check saved session and web access: %w", platform, err)
	}
	baseline, err := webAIResponseCount(ctx, client, page.responses, prompt)
	if err != nil {
		return "", fmt.Errorf("cannot read %s conversation before prompt: %w", platform, err)
	}
	if err := attachWebAIImages(ctx, client, page.fileInputAccept, resolvedImagePaths); err != nil {
		return "", fmt.Errorf("attach images in %s: %w", platform, err)
	}
	if err := setWebAIPrompt(ctx, client, page.composer, prompt); err != nil {
		return "", fmt.Errorf("enter prompt in %s: %w", platform, err)
	}
	if err := submitWebAIPrompt(ctx, client, platform); err != nil {
		return "", fmt.Errorf("submit prompt in %s: %w", platform, err)
	}
	answer, err = waitWebAIAnswer(ctx, client, page.responses, page.busy, prompt, baseline, 65*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait for %s response: %w", platform, err)
	}
	if updater != nil {
		refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer refreshCancel()
		if refreshed, readErr := browserCookieSnapshotFromCDP(refreshCtx, client); readErr == nil && len(refreshed) > 0 {
			if updateErr := updater(imageAnalysisCookieHeader(platform, refreshed), refreshed); updateErr != nil {
				log.Printf("webai image analysis: could not persist refreshed %s cookies: %v", platform, updateErr)
			}
		}
	}
	return answer, nil
}

func imageAnalysisCookieHeader(platform string, cookies []BrowserCookie) string {
	hosts := map[string]string{
		PlatformChatGPT: "chatgpt.com", PlatformClaude: "claude.ai",
		PlatformGemini: "gemini.google.com", PlatformGrok: "grok.com",
	}
	return browserCookieHeaderAllPaths(cookies, BrowserCookieScope{Platform: platform, CookieHost: hosts[platform]})
}

func waitForWebAIComposer(ctx context.Context, client *browserCDPClient, selector string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		value, err := client.evaluate(ctx, `(() => [...document.querySelectorAll(`+mustJSON(selector)+`)].some(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&!e.disabled&&getComputedStyle(e).visibility!=='hidden'}))()`)
		if err == nil && value == true {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return errors.New("composer wait timed out")
}

func webAIResponseExpression(selector, prompt string, includeBusy string) string {
	return `(() => {
		const normalize=s=>(s||'').replace(/\s+/g,' ').trim();
		const promptText=normalize(` + mustJSON(prompt) + `);
		const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};
		const assistant=e=>e.querySelector("[data-message-author-role='assistant'],[data-role='assistant'],[data-testid*='assistant' i]")||e;
		const xs=[...new Set([...document.querySelectorAll(` + mustJSON(selector) + `)].map(assistant))].filter(e=>{
			if(!visible(e))return false;
			for(let n=e;n&&n!==document.body;n=n.parentElement){
				const role=((n.getAttribute('data-message-author-role')||n.getAttribute('data-role')||'').toLowerCase());
				const id=(n.getAttribute('data-testid')||'').toLowerCase();
				if(role==='user'||role==='human'||/(^|[-_ ])(?:user|human)[-_ ]?(?:message|turn)?(?:$|[-_ ])/i.test(id))return false;
			}
			const raw=normalize(e.innerText||e.textContent||'');
			const text=promptText?normalize(raw.split(promptText).join(' ')):raw;
			return /dance_detected|video_index/i.test(text);
		});
		const busy=[...document.querySelectorAll(` + includeBusy + `)].some(visible);
		const raw=xs.length?(xs[xs.length-1].innerText||xs[xs.length-1].textContent||''):'';
		const text=promptText?normalize(raw).split(promptText).join(' ').trim():raw;
		return {count:xs.length,text,busy};
	})()`
}

func webAIResponseCount(ctx context.Context, client *browserCDPClient, selector, prompt string) (int, error) {
	value, err := client.evaluate(ctx, webAIResponseExpression(selector, prompt, mustJSON(":root:disabled")))
	if err != nil {
		return 0, err
	}
	var state struct {
		Count int `json:"count"`
	}
	raw, marshalErr := json.Marshal(value)
	if marshalErr != nil || json.Unmarshal(raw, &state) != nil {
		return 0, errors.New("response count is unreadable")
	}
	return state.Count, nil
}

func attachWebAIImages(ctx context.Context, client *browserCDPClient, accept string, paths []string) error {
	inputs, err := flowFileInputs(ctx, client)
	if err != nil {
		return err
	}
	selected := selectWebAIImageInput(inputs, accept, len(paths))
	if selected == nil {
		if err := openWebAIAttachmentMenu(ctx, client); err != nil {
			return errors.New("image file input not found")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		inputs, err = flowFileInputs(ctx, client)
		if err != nil {
			return err
		}
		selected = selectWebAIImageInput(inputs, accept, len(paths))
	}
	if selected == nil {
		return errors.New("provider has no multi-image input")
	}
	if _, err := client.call(ctx, "DOM.setFileInputFiles", map[string]any{"files": paths, "nodeId": selected.NodeID}, client.sessionID); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1500 * time.Millisecond):
	}
	return nil
}

func selectWebAIImageInput(inputs []flowFileInput, accept string, count int) *flowFileInput {
	var fallback *flowFileInput
	for i := range inputs {
		input := &inputs[i]
		if !flowInputMatches(*input, "image") || (accept != "" && !strings.Contains(input.Accept, accept)) {
			continue
		}
		if fallback == nil {
			fallback = input
		}
		if input.Multiple || count == 1 {
			return input
		}
	}
	if fallback != nil && count == 1 {
		return fallback
	}
	return nil
}

func openWebAIAttachmentMenu(ctx context.Context, client *browserCDPClient) error {
	labels := []string{"attach", "add file", "add photo", "upload", "tải tệp", "đính kèm", "thêm ảnh"}
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const e=[...document.querySelectorAll('button,[role="button"]')].find(x=>{const r=x.getBoundingClientRect();const t=((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('title')||'')).toLowerCase();return r.width>0&&r.height>0&&!x.disabled&&labels.some(s=>t.includes(s))});if(!e)return false;e.click();return true})()`
	if value, err := client.evaluate(ctx, expression); err == nil && value == true {
		return nil
	}
	return errors.New("attachment menu not found")
}

func setWebAIPrompt(ctx context.Context, client *browserCDPClient, selector, prompt string) error {
	expression := `(() => {const e=[...document.querySelectorAll(` + mustJSON(selector) + `)].find(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&!x.disabled&&getComputedStyle(x).visibility!=='hidden'});if(!e)return false;const p=` + mustJSON(prompt) + `;e.focus();if(e instanceof HTMLTextAreaElement||e instanceof HTMLInputElement){const proto=e instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;Object.getOwnPropertyDescriptor(proto,'value').set.call(e,p);e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));e.dispatchEvent(new Event('change',{bubbles:true}));}else{e.textContent='';document.execCommand('insertText',false,p);e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));}return true})()`
	value, err := client.evaluate(ctx, expression)
	if err != nil || value != true {
		return errors.New("composer rejected prompt")
	}
	return nil
}

func submitWebAIPrompt(ctx context.Context, client *browserCDPClient, platform string) error {
	labels := []string{"send", "submit", "send message", "submit prompt", "gửi"}
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const e=[...document.querySelectorAll('button,[role="button"]')].find(x=>{const r=x.getBoundingClientRect();const t=((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('data-testid')||'')).toLowerCase();return r.width>0&&r.height>0&&!x.disabled&&x.getAttribute('aria-disabled')!=='true'&&labels.some(s=>t.includes(s))});if(!e)return false;e.click();return true})()`
	if value, err := client.evaluate(ctx, expression); err == nil && value == true {
		return nil
	}
	if _, err := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID); err != nil {
		return fmt.Errorf("%s composer did not accept submit", platform)
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID)
	return nil
}

func waitWebAIAnswer(ctx context.Context, client *browserCDPClient, responses, busy, prompt string, baseline int, timeout time.Duration) (string, error) {
	expression := webAIResponseExpression(responses, prompt, mustJSON(busy))
	deadline := time.Now().Add(timeout)
	lastText := ""
	stableAt := time.Time{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil {
			var state struct {
				Count int    `json:"count"`
				Text  string `json:"text"`
				Busy  bool   `json:"busy"`
			}
			if raw, marshalErr := json.Marshal(value); marshalErr == nil && json.Unmarshal(raw, &state) == nil {
				cleanText := strings.TrimSpace(state.Text)
				if state.Count > baseline && cleanText != "" && !state.Busy {
					if state.Text == lastText {
						if stableAt.IsZero() {
							stableAt = time.Now()
						}
						if time.Since(stableAt) >= 1400*time.Millisecond {
							return cleanText, nil
						}
					} else {
						lastText = state.Text
						stableAt = time.Time{}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", errors.New("response wait timed out")
}

func mustJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
