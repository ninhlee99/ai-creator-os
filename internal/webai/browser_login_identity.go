package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func cdpLookupAccountEmail(ctx context.Context, browserWSURL string, port int, lookupURL string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	created, err := cdpBrowserCommand(lookupCtx, browserWSURL, "Target.createTarget", map[string]any{
		"url":        lookupURL,
		"background": true,
	})
	if err != nil {
		return ""
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(created, &target) != nil || target.TargetID == "" {
		return ""
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		_, _ = cdpBrowserCommand(closeCtx, browserWSURL, "Target.closeTarget", map[string]any{"targetId": target.TargetID})
	}()

	script := accountIdentityScript()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err := lookupCtx.Err(); err != nil {
			return ""
		}
		if targetWS := cdpPageWebSocket(port, target.TargetID); targetWS != "" {
			if email := cdpEvaluateIdentity(targetWS, script); email != "" {
				return email
			}
		}
		select {
		case <-lookupCtx.Done():
			return ""
		case <-time.After(350 * time.Millisecond):
		}
	}
	return ""
}

func cdpBrowserCommand(ctx context.Context, browserWSURL, method string, params map[string]any) (json.RawMessage, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.DialContext(ctx, browserWSURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	id := cdpReqID.Add(1)
	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	if err := conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var response struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("cdp %s: %s", method, response.Error.Message)
		}
		return response.Result, nil
	}
}

func cdpPageWebSocket(port int, targetID string) string {
	client := &http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var targets []struct {
		ID                   string `json:"id"`
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if json.NewDecoder(resp.Body).Decode(&targets) != nil {
		return ""
	}
	for _, target := range targets {
		if target.ID == targetID && target.Type == "page" {
			return target.WebSocketDebuggerURL
		}
	}
	return ""
}

func cdpEvaluateIdentity(targetWS, script string) string {
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(targetWS, nil)
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	id := cdpReqID.Add(1)
	if err := conn.WriteJSON(map[string]any{
		"id":     id,
		"method": "Runtime.evaluate",
		"params": map[string]any{"expression": script, "returnByValue": true, "awaitPromise": true},
	}); err != nil {
		return ""
	}
	for {
		var response struct {
			ID     int64 `json:"id"`
			Result struct {
				Result struct {
					Value any `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if conn.ReadJSON(&response) != nil {
			return ""
		}
		if response.ID != id {
			continue
		}
		if value, ok := response.Result.Result.Value.(string); ok {
			return cleanExtractedEmail(value)
		}
		return ""
	}
}

func accountIdentityScript() string {
	return `(function () {
		const emailFromText = value => {
			if (typeof value !== 'string') return '';
			const matches = value.match(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g) || [];
			for (const candidate of matches) {
				const email = candidate.trim();
				const low = email.toLowerCase();
				if (low.includes('sentry') || low.includes('example.') || low.includes('w3.org') || low.includes('schema.org') || low.includes('facebook.com') || low.includes('meta.com') || low.startsWith('support@')) continue;
				return email;
			}
			return '';
		};
		const host = location.hostname.toLowerCase();
		const path = location.pathname.toLowerCase();
		if ((host === 'accounts.google.com' || host === 'facebook.com' || host.endsWith('.facebook.com')) && (path.includes('login') || path.includes('signin') || document.querySelector('input[type="password"]'))) return '';
		if (document.querySelector('input[type="password"]')) return '';
		const fromObject = (value, depth = 0) => {
			if (!value || typeof value !== 'object' || depth > 7) return '';
			for (const [key, child] of Object.entries(value)) {
				if (/^email(address)?$/i.test(key)) {
					const email = emailFromText(child);
					if (email) return email;
				}
				if (child && typeof child === 'object') {
					const email = fromObject(child, depth + 1);
					if (email) return email;
				}
			}
			return '';
		};
		try {
			if (window.WIZ_global_data && typeof window.WIZ_global_data.oPEP7c === 'string') {
				const email = emailFromText(window.WIZ_global_data.oPEP7c);
				if (email) return email;
			}
			for (const key of ['user', '__INITIAL_STATE__', '__NEXT_DATA__', '__NUXT__', '__APOLLO_STATE__']) {
				const email = fromObject(window[key]);
				if (email) return email;
			}
		} catch(e) {}
		try {
			for (const el of document.querySelectorAll('[data-email], [aria-label*="@"], [title*="@"], a[href^="mailto:"], [data-testid*="email" i], [data-testid*="profile" i]')) {
				for (const value of [el.getAttribute('data-email'), el.getAttribute('aria-label'), el.getAttribute('title'), el.getAttribute('href'), el.innerText, el.textContent]) {
					const email = emailFromText(value || '');
					if (email) return email;
				}
			}
		} catch(e) {}
		try {
			for (const storage of [localStorage, sessionStorage]) {
				for (let i = 0; i < storage.length; i++) {
					const key = storage.key(i) || '';
					if (!/(auth|user|account|profile|session)/i.test(key)) continue;
					const value = storage.getItem(key) || '';
					if (!value.includes('@')) continue;
					try {
						const email = fromObject(JSON.parse(value));
						if (email) return email;
					} catch(e) {}
				}
			}
		} catch(e) {}
		return emailFromText(document.body ? document.body.innerText || '' : '');
	})()`
}

func cdpExtractPageIdentity(port int, target WebLoginTarget) string {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type                 string `json:"type"`
		URL                  string `json:"url"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return ""
	}
	if target.Platform == PlatformMuseAI {
		var museTargets []string
		var identityProviderTargets []string
		for _, t := range tabs {
			if t.Type != "page" || t.WebSocketDebuggerURL == "" {
				continue
			}
			u, err := url.Parse(t.URL)
			if err != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())
			if host == "muse.ai" || host == "www.muse.ai" || strings.HasSuffix(host, ".muse.ai") {
				museTargets = append(museTargets, t.WebSocketDebuggerURL)
			} else if isMuseIdentityProviderHost(host) {
				identityProviderTargets = append(identityProviderTargets, t.WebSocketDebuggerURL)
			}
		}
		return cdpExtractMuseEmailFromTargets(append(museTargets, identityProviderTargets...))
	}

	adapter, ok := webLoginAdapterFor(target.Platform)
	if !ok {
		return ""
	}
	var targetWS string
	for _, tab := range tabs {
		if tab.Type == "page" && tab.WebSocketDebuggerURL != "" && adapter.matchesIdentityURL(tab.URL) {
			targetWS = tab.WebSocketDebuggerURL
			break
		}
	}
	if targetWS == "" {
		return ""
	}

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(targetWS, nil)
	if err != nil {
		return ""
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	script := adapter.identityScript()
	if script == "" {
		return ""
	}
	reqID := cdpReqID.Add(1)
	req := map[string]any{
		"id":     reqID,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    script,
			"returnByValue": true,
			"awaitPromise":  true,
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		return ""
	}

	for {
		var msg struct {
			ID     int64 `json:"id"`
			Result struct {
				Result struct {
					Type  string `json:"type"`
					Value any    `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return ""
		}
		if msg.ID != reqID {
			continue
		}
		if str, ok := msg.Result.Result.Value.(string); ok {
			cand := strings.TrimSpace(str)
			if clean := cleanExtractedEmail(cand); clean != "" {
				return clean
			}
			if !isSystemOrStaticEmail(cand) && cand != "" {
				if strings.Contains(cand, "@") {
					return cand
				}
				if target.Platform == PlatformMuseAI {
					return ""
				}
				return cand + "@" + target.Platform
			}
		}
		break
	}
	return ""
}
