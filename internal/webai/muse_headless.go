package webai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// MuseGatewayUnavailableError marks failures before chat.stream was sent.
// Callers may safely fall back to headless browser because no prompt was queued.
type MuseGatewayUnavailableError struct{ cause error }

func (e *MuseGatewayUnavailableError) Error() string {
	return "Muse direct gateway unavailable: " + e.cause.Error()
}
func (e *MuseGatewayUnavailableError) Unwrap() error { return e.cause }

// MuseChatHeadless uses an installed Chrome/Edge browser in headless mode
// with the supplied saved Muse session. It never opens a visible window.
func MuseChatHeadless(ctx context.Context, cookie, prompt string, progress func(string)) (out MuseReply, retErr error) {
	return MuseChatHeadlessWithCookieUpdate(ctx, cookie, prompt, progress, nil)
}

func MuseChatHeadlessWithCookieUpdate(ctx context.Context, cookie, prompt string, progress func(string), onCookieUpdate func(string) error) (out MuseReply, retErr error) {
	updater := func(header string, _ []BrowserCookie) error {
		if onCookieUpdate != nil {
			return onCookieUpdate(header)
		}
		return nil
	}
	return MuseChatHeadlessWithCookieSnapshot(ctx, cookie, nil, prompt, progress, updater)
}

func MuseChatHeadlessWithCookieSnapshot(ctx context.Context, cookie string, savedCookies []BrowserCookie, prompt string, progress func(string), updater CookieSnapshotUpdater) (out MuseReply, retErr error) {
	return MuseChatHeadlessWithCookieSnapshotForKind(ctx, cookie, savedCookies, prompt, "", progress, updater)
}

// MuseChatHeadlessWithCookieSnapshotForKind waits for image/video attachments
// when requested; text chats return with the assistant reply.
func MuseChatHeadlessWithCookieSnapshotForKind(ctx context.Context, cookie string, savedCookies []BrowserCookie, prompt, kind string, progress func(string), updater CookieSnapshotUpdater) (out MuseReply, retErr error) {
	return museChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(
		ctx, cookie, savedCookies, prompt, nil, "", false, "", museAttachmentKind(kind), progress, updater,
	)
}

// MuseChatHeadlessWithFilesAndThreadCookieSnapshot runs a chat in a temporary
// headless browser profile using the selected account's saved session.
func MuseChatHeadlessWithFilesAndThreadCookieSnapshot(ctx context.Context, cookie string, savedCookies []BrowserCookie, prompt string, files []string, threadURL string, newThread bool, progress func(string), updater CookieSnapshotUpdater) (out MuseReply, retErr error) {
	return MuseChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(ctx, cookie, savedCookies, prompt, files, threadURL, newThread, "", progress, updater)
}

// MuseChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName handles the
// first-use name question without modifying the workflow prompt.
func MuseChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(ctx context.Context, cookie string, savedCookies []BrowserCookie, prompt string, files []string, threadURL string, newThread bool, accountName string, progress func(string), updater CookieSnapshotUpdater) (out MuseReply, retErr error) {
	return museChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(ctx, cookie, savedCookies, prompt, files, threadURL, newThread, accountName, "video", progress, updater)
}

func museChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(ctx context.Context, cookie string, savedCookies []BrowserCookie, prompt string, files []string, threadURL string, newThread bool, accountName, mediaKind string, progress func(string), updater CookieSnapshotUpdater) (out MuseReply, retErr error) {
	if strings.TrimSpace(cookie) == "" && len(savedCookies) == 0 || strings.TrimSpace(prompt) == "" {
		return out, errors.New("Muse headless renderer needs a saved session and nonempty prompt")
	}
	for _, file := range files {
		if strings.TrimSpace(file) == "" {
			return out, errors.New("Muse reference file does not exist")
		}
		if _, err := os.Stat(file); err != nil {
			return out, errors.New("Muse reference file does not exist")
		}
	}
	if threadURL != "" {
		threadURL = museConversationURL(threadURL)
		if threadURL == "" {
			return out, errors.New("Muse thread URL is invalid or has no conversation ID")
		}
	}
	if progress == nil {
		progress = func(string) {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	releaseBrowserSlot, err := acquireHeadlessBrowserSlot(ctx)
	if err != nil {
		return out, fmt.Errorf("waiting for available headless browser: %w", err)
	}
	defer releaseBrowserSlot()
	bin, err := findChromiumBinary()
	if err != nil {
		return out, errors.New("Muse headless rendering requires an installed Chrome, Edge, or Chromium browser")
	}
	profile, releaseProfile, err := acquireAccountBrowserProfile(PlatformMuseAI, cookie, savedCookies)
	if err != nil {
		return out, errors.New("cannot create temporary Muse headless profile")
	}
	defer releaseAccountBrowserProfile(profile, releaseProfile)
	args := optimizedHeadlessBrowserArgs(0, profile, "1280,1000")
	cmd := exec.CommandContext(ctx, bin, args...)
	closeBrowserOutput, err := ConfigureHeadlessBrowserOutput(cmd)
	if err != nil {
		return out, errors.New("cannot configure Muse headless browser output")
	}
	defer closeBrowserOutput()
	configureHeadlessBrowserCommand(cmd)
	if err := cmd.Start(); err != nil {
		return out, errors.New("cannot start Muse headless browser")
	}
	defer terminateBrowser(cmd)

	browserURL, err := waitDebuggerURL(ctx, 0, profile)
	if err != nil {
		return out, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := dialer.DialContext(ctx, browserURL, nil)
	if err != nil {
		return out, errors.New("cannot connect to Muse headless browser")
	}
	defer conn.Close()
	client := &browserCDPClient{conn: conn}
	targetID, _, err := client.attachOrReusePageTarget(ctx)
	if err != nil {
		return out, err
	}
	defer client.closePage(context.Background(), targetID)
	cookies := normalizeCookieJar(savedCookies)
	if len(cookies) == 0 {
		cookies = mergeCookieJarHeader(nil, cookie, PlatformMuseAI)
	}
	if len(cookies) == 0 {
		return out, errors.New("saved Muse cookie header is empty")
	}
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable", "DOM.enable"} {
		if _, err := client.call(ctx, method, map[string]any{}, client.sessionID); err != nil {
			return out, err
		}
	}
	client.enableStealth(ctx)
	client.setupResourceThrottling(ctx)
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": browserCookieSetParams(PlatformMuseAI, cookies)}, client.sessionID); err != nil {
		return out, errors.New("Muse headless browser rejected saved session cookies")
	}
	initialSnapshot, err := browserCookieSnapshotFromCDP(ctx, client)
	if err != nil || len(initialSnapshot) == 0 {
		initialSnapshot = cookies
	}
	monitorCtx, monitor := startBrowserCookieMonitor(ctx, client, initialSnapshot, browserCookieSnapshotFromCDP, PlatformMuseAI, updater)
	ctx = monitorCtx
	defer func() {
		if monitorErr := monitor.stop(); monitorErr != nil {
			retErr = fmt.Errorf("persist refreshed Muse cookies: %w", monitorErr)
		}
		refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if refreshed, readErr := browserCookieSnapshotFromCDP(refreshCtx, client); readErr == nil && len(refreshed) > 0 {
			out.CookieJar = refreshed
			out.CookieHeader = browserCookieHeaderForPlatform(PlatformMuseAI, refreshed)
			if !reflect.DeepEqual(refreshed, monitor.current()) && updater != nil {
				if err := updater(out.CookieHeader, refreshed); err != nil {
					retErr = fmt.Errorf("persist refreshed Muse cookies: %w", err)
				}
			}
		}
	}()
	startURL := museHTTPBase + "/"
	if !newThread && threadURL != "" {
		startURL = threadURL
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": startURL}, client.sessionID); err != nil {
		return out, errors.New("Muse headless page navigation failed")
	}
	progress("MUSE_HEADLESS: khởi chạy Chrome/Edge ẩn để dùng phiên Muse đã lưu")
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		value, err := client.evaluate(ctx, "Boolean("+museComposerTargetJS()+")")
		if err == nil && value == true {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	ready, err := client.evaluate(ctx, "Boolean("+museComposerTargetJS()+")")
	if err != nil || ready != true {
		return out, errors.New("Muse composer did not load in headless browser; check saved session and access")
	}
	if newThread {
		if err := ensureMuseNewThread(ctx, client); err != nil {
			return out, err
		}
	}
	baseline, err := museAssistantCount(ctx, client)
	if err != nil {
		return out, fmt.Errorf("cannot read Muse chat before sending prompt: %w", err)
	}
	if len(files) > 0 {
		if err := attachMuseFiles(ctx, client, files); err != nil {
			return out, fmt.Errorf("Muse could not attach reference files: %w", err)
		}
	}
	reply, err := sendMusePromptFromBaseline(ctx, client, prompt, baseline, progress)
	if err != nil {
		return out, err
	}
	if isMuseNameQuestion(reply.Text) {
		if strings.TrimSpace(accountName) == "" {
			return out, errors.New("Muse onboarding requires a nonempty account display name")
		}
		progress("MUSE_HEADLESS: trả lời câu hỏi onboarding của Muse")
		if _, err := sendMusePromptAndWait(ctx, client, strings.TrimSpace(accountName), progress); err != nil {
			return out, fmt.Errorf("Muse onboarding reply failed: %w", err)
		}
		baseline, err = museAssistantCount(ctx, client)
		if err != nil {
			return out, fmt.Errorf("cannot read Muse chat after onboarding: %w", err)
		}
		if len(files) > 0 {
			if err := attachMuseFiles(ctx, client, files); err != nil {
				return out, fmt.Errorf("Muse could not reattach reference files after onboarding: %w", err)
			}
		}
		reply, err = sendMusePromptFromBaseline(ctx, client, prompt, baseline, progress)
		if err != nil {
			return out, err
		}
	}
	if mediaKind != "" {
		reply, err = waitMuseMediaAttachment(ctx, client, reply, mediaKind, progress)
		if err != nil {
			return out, err
		}
	}
	out.Text = reply.Text
	out.MediaURLs = extractMuseMediaURLs(map[string]any{"links": reply.Links}, reply.Text)
	out.ThreadURL = museConversationURL(reply.Thread)
	blobLinks := reply.VideoLinks
	if mediaKind == "image" {
		blobLinks = reply.ImageLinks
	}
	for _, mediaURL := range blobLinks {
		if !strings.HasPrefix(mediaURL, "blob:") {
			continue
		}
		data, mime, mediaErr := fetchMuseBlobMedia(ctx, client, mediaURL, mediaKind)
		if mediaErr == nil && len(data) > 0 {
			out.MediaData, out.MediaMime = data, mime
			break
		}
	}
	out.CookieHeader = museUpdatedCookieHeader(ctx, client)
	return out, nil
}

func museAttachmentKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "video", "image":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return ""
	}
}

func museConversationURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || (parsed.Hostname() != "muse.ai" && parsed.Hostname() != "www.muse.ai") {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "thread" || strings.TrimSpace(parts[1]) == "" {
		return ""
	}
	return parsed.String()
}

type museAssistantReply struct {
	Count      int      `json:"count"`
	Text       string   `json:"text"`
	Links      []string `json:"links"`
	VideoLinks []string `json:"videoLinks"`
	ImageLinks []string `json:"imageLinks"`
	Thread     string   `json:"thread"`
}

func sendMusePromptAndWait(ctx context.Context, client *browserCDPClient, prompt string, progress func(string)) (museAssistantReply, error) {
	baseline, err := museAssistantCount(ctx, client)
	if err != nil {
		return museAssistantReply{}, fmt.Errorf("cannot read Muse chat before sending prompt: %w", err)
	}
	return sendMusePromptFromBaseline(ctx, client, prompt, baseline, progress)
}

func museAssistantCount(ctx context.Context, client *browserCDPClient) (float64, error) {
	baselineValue, err := client.evaluate(ctx, `document.querySelectorAll('[data-message-item][data-message-role="assistant"]').length`)
	if err != nil {
		return 0, err
	}
	baseline, ok := baselineValue.(float64)
	if !ok {
		return 0, errors.New("Muse returned invalid assistant message count")
	}
	return baseline, nil
}

func sendMusePromptFromBaseline(ctx context.Context, client *browserCDPClient, prompt string, baseline float64, progress func(string)) (museAssistantReply, error) {
	inserted, err := client.evaluate(ctx, `(() => {const e=`+museComposerTargetJS()+`;if(!e)return false;e.click();e.focus();return true})()`)
	if err != nil || inserted != true {
		return museAssistantReply{}, errors.New("could not focus Muse composer")
	}
	cleared, err := client.evaluate(ctx, museClearComposerJS())
	if err != nil || cleared != true {
		return museAssistantReply{}, errors.New("could not clear Muse composer")
	}
	if _, err := client.call(ctx, "Input.insertText", map[string]any{"text": prompt}, client.sessionID); err != nil {
		return museAssistantReply{}, errors.New("could not enter prompt in Muse composer")
	}
	promptJSON, _ := json.Marshal(prompt)
	verify := `(async()=>{const p=` + string(promptJSON) + `;const read=()=>{const e=` + museComposerTargetJS() + `;return e?(e.value??e.innerText??e.textContent??''):''};for(let i=0;i<20;i++){if(read().trim()===p.trim())return true;await new Promise(r=>setTimeout(r,100))}return false})()`
	inserted, err = client.evaluate(ctx, verify)
	if err != nil || inserted != true {
		return museAssistantReply{}, errors.New("Muse composer did not retain the requested prompt")
	}
	if _, err := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID); err != nil {
		return museAssistantReply{}, errors.New("could not send prompt to Muse composer")
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID)
	if err := waitMuseUserMessage(ctx, client, prompt, 15*time.Second); err != nil {
		return museAssistantReply{}, err
	}
	progress("MUSE_HEADLESS: chờ Muse phản hồi")
	deadline := time.Now().Add(11 * time.Minute)
	lastText := ""
	stableAt := time.Time{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return museAssistantReply{}, err
		}
		value, err := client.evaluate(ctx, `(() => {
			const side = document.querySelector('[data-side-chat], [aria-label*="side chat" i], [class*="side-chat" i], [class*="sidechat" i], aside');
			const root = side || document;
			let a = [...root.querySelectorAll('[data-message-item][data-message-role="assistant"]')];
			if (!a.length) {
				a = [...document.querySelectorAll('[data-message-item][data-message-role="assistant"]')];
			}
			const n = a[a.length - 1];
			const busy = !!(root.querySelector('[data-testid="hatch-composer-stop-button"]') || document.querySelector('[data-testid="hatch-composer-stop-button"]'));
			return {
				count: a.length,
				text: n ? (n.innerText || n.textContent || '') : '',
				links: n ? [...n.querySelectorAll('a[href],video[src],source[src],img[src]')].map(x => x.href || x.src) : [],
				videoLinks: n ? [...n.querySelectorAll('video[src],source[src],a[download*=".mp4"],a[href*=".mp4"]')].map(x => x.src || x.href) : [],
				imageLinks: n ? [...n.querySelectorAll('img[src],a[download*=".png"],a[href*=".png"]')].map(x => x.src || x.href) : [],
				busy: busy,
				thread: location.href
			};
		})()`)
		if err == nil {
			var snapshot struct {
				Count      int      `json:"count"`
				Text       string   `json:"text"`
				Links      []string `json:"links"`
				VideoLinks []string `json:"videoLinks"`
				ImageLinks []string `json:"imageLinks"`
				Busy       bool     `json:"busy"`
				Thread     string   `json:"thread"`
			}
			if raw, marshalErr := json.Marshal(value); marshalErr == nil && json.Unmarshal(raw, &snapshot) == nil && float64(snapshot.Count) > baseline && strings.TrimSpace(snapshot.Text) != "" && !snapshot.Busy {
				if snapshot.Text == lastText {
					if stableAt.IsZero() {
						stableAt = time.Now()
					}
					if time.Since(stableAt) >= 1800*time.Millisecond {
						return museAssistantReply{Count: snapshot.Count, Text: snapshot.Text, Links: snapshot.Links, VideoLinks: snapshot.VideoLinks, ImageLinks: snapshot.ImageLinks, Thread: snapshot.Thread}, nil
					}
				} else {
					lastText = snapshot.Text
					stableAt = time.Time{}
				}
			}
		}
		select {
		case <-ctx.Done():
			return museAssistantReply{}, ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
	return museAssistantReply{}, errors.New("Muse headless browser timed out waiting for reply")
}

func waitMuseMediaAttachment(ctx context.Context, client *browserCDPClient, reply museAssistantReply, mediaKind string, progress func(string)) (museAssistantReply, error) {
	if museReplyHasMediaAttachment(reply, mediaKind) {
		return reply, nil
	}
	deadline := time.Now().Add(8 * time.Minute)
	lastProgress := time.Time{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return reply, err
		}
		value, err := client.evaluate(ctx, `(() => {
			const side = document.querySelector('[data-side-chat], [aria-label*="side chat" i], [class*="side-chat" i], [class*="sidechat" i], aside');
			const root = side || document;
			let a = [...root.querySelectorAll('[data-message-item][data-message-role="assistant"]')];
			if (!a.length) {
				a = [...document.querySelectorAll('[data-message-item][data-message-role="assistant"]')];
			}
			const n = a[a.length - 1];
			const busy = !!(root.querySelector('[data-testid="hatch-composer-stop-button"]') || document.querySelector('[data-testid="hatch-composer-stop-button"]'));
			return {
				count: a.length,
				text: n ? (n.innerText || n.textContent || '') : '',
				links: n ? [...n.querySelectorAll('a[href],video[src],source[src],img[src]')].map(x => x.href || x.src) : [],
				videoLinks: n ? [...n.querySelectorAll('video[src],source[src],a[download*=".mp4"],a[href*=".mp4"]')].map(x => x.src || x.href) : [],
				imageLinks: n ? [...n.querySelectorAll('img[src],a[download*=".png"],a[href*=".png"]')].map(x => x.src || x.href) : [],
				busy: busy,
				thread: location.href
			};
		})()`)
		if err == nil {
			var snapshot museAssistantReply
			var state struct {
				Count      int      `json:"count"`
				Text       string   `json:"text"`
				Links      []string `json:"links"`
				VideoLinks []string `json:"videoLinks"`
				ImageLinks []string `json:"imageLinks"`
				Busy       bool     `json:"busy"`
				Thread     string   `json:"thread"`
			}
			if raw, marshalErr := json.Marshal(value); marshalErr == nil && json.Unmarshal(raw, &state) == nil {
				snapshot = museAssistantReply{Count: state.Count, Text: state.Text, Links: state.Links, VideoLinks: state.VideoLinks, ImageLinks: state.ImageLinks, Thread: state.Thread}
				if snapshot.Count >= reply.Count && !state.Busy && museReplyHasMediaAttachment(snapshot, mediaKind) {
					return snapshot, nil
				}
			}
		}
		if progress != nil && time.Since(lastProgress) >= 15*time.Second {
			progress("MUSE_HEADLESS: chờ tệp media được đính kèm")
			lastProgress = time.Now()
		}
		select {
		case <-ctx.Done():
			return reply, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return reply, fmt.Errorf("Muse reply completed but its %s attachment did not appear within 8 minutes", mediaKind)
}

func museReplyHasMediaAttachment(reply museAssistantReply, mediaKind string) bool {
	links := reply.VideoLinks
	validExtensions := map[string]bool{".mp4": true, ".webm": true}
	if mediaKind == "image" {
		links = reply.ImageLinks
		validExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
	}
	for _, link := range links {
		if strings.HasPrefix(link, "blob:") {
			return true
		}
	}
	for _, link := range reply.Links {
		parsed, err := url.Parse(link)
		if err == nil && validExtensions[strings.ToLower(filepath.Ext(parsed.Path))] {
			return true
		}
	}
	return false
}

func waitMuseUserMessage(ctx context.Context, client *browserCDPClient, prompt string, timeout time.Duration) error {
	encoded, _ := json.Marshal(prompt)
	expression := `(() => {const p=` + string(encoded) + `;return [...document.querySelectorAll('[data-message-item][data-message-role="user"]')].some(e=>(e.innerText||e.textContent||'').includes(p))})()`
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil && value == true {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return errors.New("Muse did not submit the requested prompt")
}

func museComposerTargetJS() string {
	return `(() => {
		const selector = '[data-lexical-editor="true"], [data-hatch-composer-root] textarea, [data-hatch-composer-prehydration-input], [data-hatch-composer-root] [contenteditable="true"], textarea, [contenteditable="true"]';
		const visible = e => {
			const r = e.getBoundingClientRect(), s = getComputedStyle(e);
			return r.width > 0 && r.height > 0 && s.display !== 'none' && s.visibility !== 'hidden' && !e.disabled && !e.readOnly;
		};
		const all = [...document.querySelectorAll(selector)].filter(visible);
		if (!all.length) return null;
		// Ưu tiên composer nằm trong Side chat pane / drawer / aside
		const sideEl = all.find(e => {
			return e.closest('[data-side-chat], [aria-label*="side chat" i], [class*="side-chat" i], [class*="sidechat" i], [id*="side" i], [class*="drawer" i], [class*="panel" i], aside') !== null;
		});
		if (sideEl) return sideEl;
		// Nếu có nhiều composer, chọn composer xuất hiện sau cùng (panel side chat)
		return all[all.length - 1];
	})()`
}

func museClearComposerJS() string {
	return `(() => {const e=` + museComposerTargetJS() + `;if(!e)return false;e.focus();if(e instanceof HTMLTextAreaElement||e instanceof HTMLInputElement){const proto=e instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;const setter=Object.getOwnPropertyDescriptor(proto,'value')?.set;if(!setter)return false;setter.call(e,'');e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'deleteContentBackward'}));e.dispatchEvent(new Event('change',{bubbles:true}));}else{document.execCommand('selectAll',false,null);document.execCommand('delete',false,null)}return (e.value??e.innerText??e.textContent??'').trim()===''})()`
}

func isMuseNameQuestion(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, cue := range []string{
		"what’s your name", "what's your name", "what is your name",
		"what should i call you", "what would you like me to call you",
		"what name should i use", "may i have your name",
	} {
		if strings.Contains(text, cue) {
			return true
		}
	}
	return strings.Contains(text, "before we get started") && strings.Contains(text, "name")
}

func ensureMuseNewThread(ctx context.Context, client *browserCDPClient) error {
	clickNewChat := func() bool {
		clicked, err := client.evaluate(ctx, `(() => {
			const visible = e => {
				const r = e.getBoundingClientRect();
				return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== 'hidden';
			};
			const xs = [...document.querySelectorAll('button, [role="button"], a')].filter(visible);
			const label = x => ((x.innerText || '') + ' ' + (x.getAttribute('aria-label') || '') + ' ' + (x.getAttribute('title') || '')).replace(/\s+/g, ' ').trim();
			
			// 1. Ưu tiên tuyệt đối tạo New Side Chat theo yêu cầu
			const sideChatBtn = xs.find(x => /new side chat/i.test(label(x))) ||
			                    xs.find(x => /start (a )?side chat/i.test(label(x))) ||
			                    xs.find(x => /side chat/i.test(label(x)));
			if (sideChatBtn) {
				sideChatBtn.click();
				return true;
			}
			
			// 2. Fallback: new chat thông thường
			const e = xs.find(x => /^(new chat|start new chat|new conversation|start conversation)$/i.test(label(x))) ||
			          xs.find(x => /new chat|new conversation/i.test(label(x)));
			if (!e) return false;
			e.click();
			return true;
		})()`)
		return err == nil && clicked == true
	}

	checkComposerReady := func(timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if ctx.Err() != nil {
				return false
			}
			val, err := client.evaluate(ctx, "Boolean("+museComposerTargetJS()+")")
			if err == nil && val == true {
				return true
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(300 * time.Millisecond):
			}
		}
		return false
	}

	if clickNewChat() {
		if checkComposerReady(5 * time.Second) {
			return nil
		}
	}

	if checkComposerReady(1 * time.Second) {
		return nil
	}

	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": museHTTPBase + "/"}, client.sessionID); err != nil {
		return errors.New("Muse could not start a new chat")
	}
	if err := waitMuseComposer(ctx, client, 15*time.Second); err != nil {
		return errors.New("Muse new chat composer did not load")
	}
	clickNewChat()
	return nil
}

func waitMuseComposer(ctx context.Context, client *browserCDPClient, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := client.evaluate(ctx, "Boolean("+museComposerTargetJS()+")")
		if err == nil && value == true {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return errors.New("Muse composer wait timed out")
}

func attachMuseFiles(ctx context.Context, client *browserCDPClient, files []string) error {
	absoluteFiles := make([]string, 0, len(files))
	for _, file := range files {
		path, err := filepath.Abs(strings.TrimSpace(file))
		if err != nil {
			return errors.New("Muse reference file path is invalid")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("Muse reference file does not exist")
		}
		absoluteFiles = append(absoluteFiles, path)
	}
	files = absoluteFiles
	findInput := func() (int, error) {
		doc, err := client.call(ctx, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, client.sessionID)
		if err != nil {
			return 0, err
		}
		var document struct {
			Root struct {
				NodeID int `json:"nodeId"`
			} `json:"root"`
		}
		if json.Unmarshal(doc, &document) != nil || document.Root.NodeID == 0 {
			return 0, errors.New("Muse document root is unavailable")
		}
		query, err := client.call(ctx, "DOM.querySelector", map[string]any{
			"nodeId":   document.Root.NodeID,
			"selector": "[data-hatch-composer-root] input[type=file]",
		}, client.sessionID)
		if err != nil {
			return 0, err
		}
		var result struct {
			NodeID int `json:"nodeId"`
		}
		if json.Unmarshal(query, &result) == nil && result.NodeID > 0 {
			return result.NodeID, nil
		}
		// Fallback tìm input[type=file] bất kỳ (kể cả trong side chat)
		fallbackQuery, err := client.call(ctx, "DOM.querySelector", map[string]any{
			"nodeId":   document.Root.NodeID,
			"selector": "input[type=file]",
		}, client.sessionID)
		if err == nil && json.Unmarshal(fallbackQuery, &result) == nil && result.NodeID > 0 {
			return result.NodeID, nil
		}
		return 0, nil
	}

	nodeID, err := findInput()
	if err != nil {
		return err
	}
	if nodeID == 0 {
		if _, err := client.evaluate(ctx, `(() => {const e=document.querySelector('[data-hatch-composer-root] button[aria-label="Attach file"]');if(!e)return false;e.click();return true})()`); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		nodeID, err = findInput()
		if err != nil {
			return err
		}
	}
	if nodeID == 0 {
		return errors.New("Muse file input is unavailable")
	}
	if _, err := client.call(ctx, "DOM.setFileInputFiles", map[string]any{"files": files, "nodeId": nodeID}, client.sessionID); err != nil {
		return errors.New("Muse rejected reference files")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1800 * time.Millisecond):
	}
	return nil
}

// fetchMuseBlobMedia first attempts a direct browser-to-disk download via
// Page.setDownloadBehavior (avoiding V8 + JSON + Base64 RAM multiplication),
// and falls back to streaming decoded chunks into a temporary file on disk.
func fetchMuseBlobMedia(ctx context.Context, client *browserCDPClient, mediaURL string, mediaKind ...string) ([]byte, string, error) {
	kind := "video"
	if len(mediaKind) > 0 && mediaKind[0] == "image" {
		kind = "image"
	}
	if downloadDir, err := os.MkdirTemp("", "aicos-muse-blob-*"); err == nil {
		defer os.RemoveAll(downloadDir)
		targetOut := filepath.Join(downloadDir, "muse-output.mp4")
		if kind == "image" {
			targetOut = filepath.Join(downloadDir, "muse-output.png")
		}
		if downloadedPath, dlErr := downloadWebAIMediaInBrowser(ctx, client, mediaURL, targetOut, kind); dlErr == nil && downloadedPath != "" {
			if raw, readErr := os.ReadFile(downloadedPath); readErr == nil && len(raw) >= 1024 {
				mime := "video/mp4"
				switch strings.ToLower(filepath.Ext(downloadedPath)) {
				case ".webm":
					mime = "video/webm"
				case ".png":
					mime = "image/png"
				case ".jpg", ".jpeg":
					mime = "image/jpeg"
				case ".webp":
					mime = "image/webp"
				}
				return raw, mime, nil
			}
		}
	}

	encodedURL, _ := json.Marshal(mediaURL)
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = client.evaluate(cleanupCtx, `window.__aicosMuseBlob = null; true`)
	}
	defer cleanup()
	prepare := `(async()=>{try{const r=await fetch(` + string(encodedURL) + `);const b=await r.blob();if(!r.ok||b.size<1024||b.size>262144000)return {error:'blob size or response invalid'};window.__aicosMuseBlob=b;return {size:b.size,mime:b.type||r.headers.get('content-type')||''}}catch(e){return {error:String(e)}}})()`
	value, err := client.evaluate(ctx, prepare)
	if err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	var result struct {
		Size  int64  `json:"size"`
		MIME  string `json:"mime"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, "", err
	}
	if result.Error != "" || result.Size < 1024 || result.Size > 250<<20 {
		return nil, result.MIME, errors.New("Muse blob media could not be read")
	}

	tmpFile, err := os.CreateTemp("", "aicos-muse-stream-*.bin")
	if err != nil {
		return nil, result.MIME, err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	const chunkSize = 2 << 20
	var writtenTotal int64
	for start := 0; int64(start) < result.Size; start += chunkSize {
		end := start + chunkSize
		if int64(end) > result.Size {
			end = int(result.Size)
		}
		chunkExpression := fmt.Sprintf(`(async()=>{try{const b=window.__aicosMuseBlob;if(!b)return {error:'blob unavailable'};const a=new Uint8Array(await b.slice(%d,%d).arrayBuffer());let s='';const n=0x8000;for(let i=0;i<a.length;i+=n)s+=String.fromCharCode(...a.subarray(i,i+n));return {base64:btoa(s)}}catch(e){return {error:String(e)}}})()`, start, end)
		chunkValue, err := client.evaluate(ctx, chunkExpression)
		if err != nil {
			return nil, result.MIME, err
		}
		chunkRaw, err := json.Marshal(chunkValue)
		if err != nil {
			return nil, result.MIME, err
		}
		var chunk struct {
			Base64 string `json:"base64"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(chunkRaw, &chunk); err != nil || chunk.Error != "" || chunk.Base64 == "" {
			return nil, result.MIME, errors.New("Muse blob media chunk could not be read")
		}
		decoded, err := base64.StdEncoding.DecodeString(chunk.Base64)
		if err != nil || writtenTotal+int64(len(decoded)) > result.Size {
			return nil, result.MIME, errors.New("Muse blob media chunk is invalid")
		}
		n, writeErr := tmpFile.Write(decoded)
		if writeErr != nil {
			return nil, result.MIME, writeErr
		}
		writtenTotal += int64(n)
	}
	if writtenTotal != result.Size {
		return nil, result.MIME, errors.New("Muse blob media is incomplete")
	}
	if err := tmpFile.Close(); err != nil {
		return nil, result.MIME, err
	}
	data, err := os.ReadFile(tmpName)
	if err != nil {
		return nil, result.MIME, err
	}
	return data, result.MIME, nil
}

func museUpdatedCookieHeader(ctx context.Context, client *browserCDPClient) string {
	response, err := client.call(ctx, "Network.getAllCookies", map[string]any{}, client.sessionID)
	if err != nil {
		return ""
	}
	var cookies struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if json.Unmarshal(response, &cookies) != nil {
		return ""
	}
	scope := BrowserCookieScope{Platform: PlatformMuseAI, CookieHost: "muse.ai", CookiePath: "/api/auth/check"}
	header := browserCookieHeaderAllPaths(cookies.Cookies, scope)
	if len(museSessionCookieCandidates(cookies.Cookies, scope)) > 0 {
		return header
	}
	return ""
}
