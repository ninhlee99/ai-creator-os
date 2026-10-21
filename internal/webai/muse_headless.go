package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
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

// MuseChatHeadless is a browser fallback for environments where Muse's
// direct TLS/WebSocket gateway cannot be reached. It uses installed
// Chrome/Edge in headless mode and never opens a visible window.
func MuseChatHeadless(ctx context.Context, cookie, prompt string, progress func(string)) (MuseReply, error) {
	var out MuseReply
	if strings.TrimSpace(cookie) == "" || strings.TrimSpace(prompt) == "" {
		return out, errors.New("Muse headless fallback needs a session cookie and prompt")
	}
	if progress == nil {
		progress = func(string) {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	bin, err := findChromiumBinary()
	if err != nil {
		return out, errors.New("Muse direct gateway failed and no Chrome/Edge binary is installed for headless fallback")
	}
	profile, err := os.MkdirTemp("", "aicos-muse-headless-*")
	if err != nil {
		return out, errors.New("cannot create temporary Muse headless profile")
	}
	defer os.RemoveAll(profile)
	clearChromiumSingletonLocks(profile)
	port, listener, err := pickFreePort()
	if err != nil {
		return out, errors.New("cannot allocate Muse headless debugging port")
	}
	_ = listener.Close()
	args := []string{
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-first-run",
		"--no-default-browser-check", "--disable-sync", "--disable-extensions",
		"--disable-background-networking", "--hide-scrollbars", "--window-size=1280,1000",
		"--remote-debugging-address=127.0.0.1", fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profile, "about:blank",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		return out, errors.New("cannot start Muse headless browser")
	}
	defer terminateBrowser(cmd)

	browserURL, err := waitDebuggerURL(ctx, port)
	if err != nil {
		return out, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := dialer.DialContext(ctx, browserURL, nil)
	if err != nil {
		return out, errors.New("cannot connect to Muse headless browser")
	}
	defer conn.Close()
	client := &museCDP{conn: conn}
	created, err := client.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "")
	if err != nil {
		return out, err
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(created, &target); err != nil || target.TargetID == "" {
		return out, errors.New("Muse headless browser did not create a page")
	}
	attached, err := client.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, "")
	if err != nil {
		return out, err
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attached, &session); err != nil || session.SessionID == "" {
		return out, errors.New("Muse headless browser did not attach to page")
	}
	client.sessionID = session.SessionID
	cookies := museBrowserCookies(cookie)
	if len(cookies) == 0 {
		return out, errors.New("saved Muse cookie header is empty")
	}
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable"} {
		if _, err := client.call(ctx, method, map[string]any{}, client.sessionID); err != nil {
			return out, err
		}
	}
	if _, err := client.call(ctx, "Network.setCookies", map[string]any{"cookies": cookies}, client.sessionID); err != nil {
		return out, errors.New("Muse headless browser rejected saved session cookies")
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]any{"url": museHTTPBase + "/"}, client.sessionID); err != nil {
		return out, errors.New("Muse headless page navigation failed")
	}
	progress("MUSE_HEADLESS: khởi chạy Chrome/Edge ẩn để dùng phiên Muse đã lưu")
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		value, err := client.evaluate(ctx, `!!document.querySelector('[data-hatch-composer-root] textarea, [data-lexical-editor="true"]')`)
		if err == nil && value == true {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	ready, err := client.evaluate(ctx, `!!document.querySelector('[data-hatch-composer-root] textarea, [data-lexical-editor="true"]')`)
	if err != nil || ready != true {
		return out, errors.New("Muse composer did not load in headless browser; check saved session and access")
	}
	baselineValue, err := client.evaluate(ctx, `document.querySelectorAll('[data-message-item][data-message-role="assistant"]').length`)
	if err != nil {
		return out, errors.New("cannot read Muse chat before sending prompt")
	}
	baseline, _ := baselineValue.(float64)
	promptJSON, _ := json.Marshal(prompt)
	insert := `(() => { const e=document.querySelector('[data-hatch-composer-root] textarea, [data-lexical-editor="true"]'); if(!e)return false; e.focus(); const p=` + string(promptJSON) + `; if(e instanceof HTMLTextAreaElement){const s=Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set;s.call(e,p);e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));e.dispatchEvent(new Event('change',{bubbles:true}));}else{e.textContent=p;e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));} return true;})()`
	inserted, err := client.evaluate(ctx, insert)
	if err != nil || inserted != true {
		return out, errors.New("could not enter prompt in Muse composer")
	}
	if _, err := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID); err != nil {
		return out, errors.New("could not send prompt to Muse composer")
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID)
	progress("MUSE_HEADLESS: chờ Muse phản hồi")
	deadline = time.Now().Add(11 * time.Minute)
	lastText := ""
	stableAt := time.Time{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		value, err := client.evaluate(ctx, `(() => {const a=[...document.querySelectorAll('[data-message-item][data-message-role="assistant"]')];const n=a[a.length-1];return {count:a.length,text:n?(n.innerText||n.textContent||''):'',links:n?[...n.querySelectorAll('a[href]')].map(x=>x.href):[],busy:!!document.querySelector('[data-testid="hatch-composer-stop-button"]')}})()`)
		if err == nil {
			var snapshot struct {
				Count int      `json:"count"`
				Text  string   `json:"text"`
				Links []string `json:"links"`
				Busy  bool     `json:"busy"`
			}
			if raw, marshalErr := json.Marshal(value); marshalErr == nil && json.Unmarshal(raw, &snapshot) == nil && float64(snapshot.Count) > baseline && strings.TrimSpace(snapshot.Text) != "" && !snapshot.Busy {
				if snapshot.Text == lastText {
					if stableAt.IsZero() {
						stableAt = time.Now()
					}
					if time.Since(stableAt) >= 1800*time.Millisecond {
						out.Text = snapshot.Text
						out.MediaURLs = extractMuseMediaURLs(map[string]any{"links": snapshot.Links}, snapshot.Text)
						return out, nil
					}
				} else {
					lastText = snapshot.Text
					stableAt = time.Time{}
				}
			}
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return out, errors.New("Muse headless browser timed out waiting for reply")
}

type museCDP struct {
	conn      *websocket.Conn
	sessionID string
	nextID    atomic.Int64
}

func (c *museCDP) call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	request := map[string]any{"id": id, "method": method, "params": params}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second)); err != nil {
		return nil, err
	}
	if err := c.conn.WriteJSON(request); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return nil, errors.New("Muse headless CDP connection failed")
		}
		var response struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &response) != nil || response.ID != id {
			continue
		}
		if len(response.Error) > 0 {
			return nil, fmt.Errorf("Muse headless CDP %s failed", method)
		}
		return response.Result, nil
	}
}

func (c *museCDP) evaluate(ctx context.Context, expression string) (any, error) {
	response, err := c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, c.sessionID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, err
	}
	return result.Result.Value, nil
}

func waitDebuggerURL(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if err := ctx.Err(); err != nil {
			return "", errors.New("Muse headless browser did not start")
		}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
		if err == nil {
			var meta struct {
				WS string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&meta)
			_ = resp.Body.Close()
			if decodeErr == nil && meta.WS != "" {
				return meta.WS, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", errors.New("Muse headless browser did not start")
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func museBrowserCookies(header string) []map[string]any {
	var out []map[string]any
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		out = append(out, map[string]any{"name": name, "value": value, "domain": ".muse.ai", "path": "/", "secure": true, "httpOnly": true})
	}
	return out
}
