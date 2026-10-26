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

		u, err := url.Parse(t.URL)
		if err != nil || u.Scheme != "https" {
			continue
		}
		actualHost := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		isMatch := actualHost == host
		if host == "flow.google.com" || host == "labs.google" {
			isMatch = actualHost == "flow.google.com" || actualHost == "labs.google" || actualHost == "aitestkitchen.withgoogle.com"
		}
		if host == "grok.com" {
			isMatch = actualHost == "grok.com" || actualHost == "x.ai" || strings.HasSuffix(actualHost, ".grok.com") || strings.HasSuffix(actualHost, ".x.ai")
		}
		if !isMatch {
			continue
		}

		// Đang ở màn hình chọn tài khoản Google hoặc trang login thì CHƯA được coi là xong!
		if actualHost == "accounts.google.com" || strings.Contains(t.URL, "AccountChooser") || strings.Contains(t.URL, "/login") || strings.Contains(t.URL, "/signin") {
			continue
		}
		return true
	}
	return false
}

// cdpHasMuseLandingPage only accepts a first-party Muse app page.
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
