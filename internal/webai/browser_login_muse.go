package webai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// cdpProbeMuseAI evaluates the Muse AI page state via CDP to distinguish between OTP step, 2FA step, and logged-in state.
func cdpProbeMuseAI(port int) (isLoggedIn bool, isWaitingOTP bool, isWaiting2FA bool, detectedEmail string, err error) {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false, false, false, "", err
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type                 string `json:"type"`
		URL                  string `json:"url"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false, false, false, "", err
	}

	var targetWS, providerWS string
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
			targetWS = t.WebSocketDebuggerURL
			break
		}
		if providerWS == "" && isMuseIdentityProviderHost(host) {
			providerWS = t.WebSocketDebuggerURL
		}
	}
	if targetWS == "" {
		targetWS = providerWS
	}
	if targetWS == "" {
		return false, false, false, "", fmt.Errorf("không tìm thấy tab muse.ai")
	}

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(targetWS, nil)
	if err != nil {
		return false, false, false, "", err
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	script := `(async () => {
		try {
			const bodyText = (document.body && document.body.innerText) ? document.body.innerText : '';
			const bodyLower = bodyText.toLowerCase();
			const path = (location.pathname || '').toLowerCase();
			const host = (location.host || '').toLowerCase();
			const isAuthPage = path.includes('/signin') || path.includes('/signup') || path.includes('/login') || path.includes('/aymh') || host.includes('auth.muse.ai') || host.includes('facebook.com') || path.includes('two_factor') || path.includes('two-factor') || path.includes('2fa') || path.includes('checkpoint') || path.includes('challenge') || path.includes('mfa');

			// Check for 2FA specific fields and keywords
			const twoFAInput = document.querySelector(
				'input[name*="2fa" i], input[placeholder*="2fa" i], input[placeholder*="authenticator" i], input[placeholder*="security" i], input[id*="approvals"], input[name*="approvals"], input[name*="totp" i]'
			);
			const has2FAKeyword = bodyLower.includes('two-factor') || bodyLower.includes('two factor') || bodyLower.includes('2-factor') || bodyLower.includes('authenticator') || bodyLower.includes('authentication app') || bodyLower.includes('security code') || bodyLower.includes('login approval') || bodyLower.includes('check your other device') || bodyLower.includes('xác thực 2 yếu tố') || bodyLower.includes('hai bước') || bodyLower.includes('mã 2fa');

			// Check for general OTP code fields and keywords
			const otpInput = document.querySelector(
				'input[autocomplete*="one-time-code"], input[name*="code" i], input[placeholder*="code" i], input[maxlength="6"], input[maxlength="8"], input[inputmode="numeric"]'
			);
			const hasCodeKeyword = bodyLower.includes('verification code') || bodyLower.includes('enter the code') || bodyLower.includes('enter code') || bodyLower.includes('we sent a code') || bodyLower.includes('sent a 6-digit') || bodyLower.includes('check your email') || bodyLower.includes('mã xác nhận');

			// Only authenticated account state may supply identity; arbitrary email
			// text in chat history or OTP pages must not label the saved session.
			let candEmail = '';

			// 1. Detect 2FA waiting state first
			if (twoFAInput || (has2FAKeyword && (twoFAInput || otpInput || bodyLower.includes('submit') || bodyLower.includes('continue') || bodyLower.includes('verify') || bodyLower.includes('enter') || bodyLower.includes('confirm')))) {
				return JSON.stringify({ state: "waiting_2fa", email: candEmail });
			}

			// 2. Detect OTP waiting state
			if (otpInput || (hasCodeKeyword && (bodyLower.includes('resend') || bodyLower.includes('verify') || bodyLower.includes('submit') || bodyLower.includes('continue')))) {
				return JSON.stringify({ state: "waiting_otp", email: candEmail });
			}

			// Auth/checkpoint URLs are never successful landing pages, even if a
			// SPA fallback or public endpoint responds with HTTP 200.
				if (isAuthPage || path.includes('/checkpoint') || path.includes('/two-factor') || path.includes('/2fa')) {
				return JSON.stringify({ state: "not_logged_in", email: candEmail });
			}

			// Muse serves its public SPA HTML at /api/account, so HTTP 200 there
			// cannot establish identity. Use authenticated app UI markers instead.
			// 3. Check for explicit sign-out / log-out links
			const signoutLink = document.querySelector('a[href*="/signout"], a[href*="/logout"], button[data-testid*="logout"], [data-action*="logout"], button[aria-label*="log out" i], button[aria-label*="sign out" i]');
			if (signoutLink && !otpInput && !twoFAInput && !has2FAKeyword) {
				return JSON.stringify({ state: "logged_in", email: candEmail });
			}

			// 4. Check user avatar/profile or dashboard navigation outside auth pages
			const userProfileElement = document.querySelector('[class*="user-menu"], [class*="account-menu"], [data-testid*="user-menu"], [data-testid*="profile"], [data-testid*="account"], a[href*="/account"], a[href*="/profile"], button[aria-label*="account" i], button[aria-label*="profile" i], [role="button"][aria-label*="user" i], button[class*="avatar"], a[class*="avatar"], [role="button"][class*="avatar"]');
			const appNavigation = document.querySelector('a[href*="/videos"], a[href*="/upload"], [data-testid*="upload"], button[class*="upload"]');
			const loginControl = Array.from(document.querySelectorAll('button, a')).some(el => /^(log in|sign in)$/i.test((el.innerText || '').trim()));
			if (loginControl && !signoutLink) {
				return JSON.stringify({ state: "not_logged_in", email: candEmail });
			}
			if (!otpInput && !twoFAInput && !has2FAKeyword && (userProfileElement || appNavigation || path.includes('/dashboard'))) {
				return JSON.stringify({ state: "logged_in", email: candEmail });
			}

			return JSON.stringify({ state: "not_logged_in", email: candEmail });
		} catch(e) {
			return JSON.stringify({ state: "error", error: e.toString() });
		}
	})()`

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
		return false, false, false, "", err
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
			return false, false, false, "", err
		}
		if msg.ID != reqID {
			continue
		}
		if str, ok := msg.Result.Result.Value.(string); ok {
			var res struct {
				State string `json:"state"`
				Email string `json:"email"`
			}
			if err := json.Unmarshal([]byte(str), &res); err == nil {
				cleanEm := cleanExtractedEmail(res.Email)
				if res.State == "waiting_2fa" {
					return false, false, true, cleanEm, nil
				}
				if res.State == "waiting_otp" {
					return false, true, false, cleanEm, nil
				}
				if res.State == "logged_in" {
					return true, false, false, cleanEm, nil
				}
				return false, false, false, cleanEm, nil
			}
		}
		break
	}
	return false, false, false, "", nil
}
