package webai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func cdpHasLandingHost(port int, host string) bool {
	if host == "" {
		return true
	}
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false
	}
	for _, t := range tabs {
		if t.Type != "page" {
			continue
		}

		isMatch := strings.Contains(t.URL, host)
		if !isMatch && (host == "flow.google.com" || host == "labs.google" || host == "flow.google") {
			if strings.Contains(t.URL, "flow.google") || strings.Contains(t.URL, "labs.google") || strings.Contains(t.URL, "aitestkitchen.withgoogle.com") {
				isMatch = true
			}
		}
		if !isMatch {
			continue
		}

		// Đang ở màn hình chọn tài khoản Google hoặc trang login thì CHƯA được coi là xong!
		if (host == "gemini.google.com" || host == "flow.google.com" || host == "labs.google" || host == "flow.google") && (strings.Contains(t.URL, "accounts.google.com") || strings.Contains(t.URL, "AccountChooser")) {
			continue
		}
		if host == "chatgpt.com" && (strings.Contains(t.URL, "auth/login") || strings.Contains(t.URL, "auth0") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "claude.ai" && (strings.Contains(t.URL, "claude.ai/login") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "grok.com" && (strings.Contains(t.URL, "accounts.x.ai") || strings.Contains(t.URL, "twitter.com") || strings.Contains(t.URL, "x.com") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "muse.ai" {
			u := strings.ToLower(t.URL)
			if strings.Contains(u, "/signin") || strings.Contains(u, "/signup") || strings.Contains(u, "/login") || strings.Contains(u, "/aymh") || strings.Contains(u, "auth.muse.ai") || strings.Contains(u, "accounts.google.com") || strings.Contains(u, "two_factor") || strings.Contains(u, "two-factor") || strings.Contains(u, "checkpoint") || strings.Contains(u, "challenge") || strings.Contains(u, "2fa") || strings.Contains(u, "mfa") {
				continue
			}
			return true
		}
		return true
	}
	return false
}

// cdpHasMuseLandingPage only accepts a first-party Muse app page. The
// generic landing-host check is intentionally substring-based for legacy
// providers; for Muse that also matches auth subdomains and redirect URLs.
func cdpHasMuseLandingPage(port int) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false
	}
	for _, tab := range tabs {
		if tab.Type != "page" {
			continue
		}
		u, err := url.Parse(tab.URL)
		if err != nil || u.Scheme != "https" {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host != "muse.ai" && host != "www.muse.ai" {
			continue
		}
		path := strings.ToLower(strings.TrimRight(u.EscapedPath(), "/"))
		isAuthPath := false
		for _, prefix := range []string{
			"/signin", "/sign-in", "/signup", "/sign-up", "/login", "/logout",
			"/auth", "/aymh", "/checkpoint", "/challenge", "/two_factor",
			"/two-factor", "/2fa", "/mfa",
		} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				isAuthPath = true
				break
			}
		}
		if !isAuthPath {
			return true
		}
	}
	return false
}
