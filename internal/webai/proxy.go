package webai

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyRelay forwards requests to target Web AI portals while stripping
// anti-embedding headers (X-Frame-Options, frame-ancestors CSP) to enable
// the in-app interactive web experience requested by the user.
type ProxyRelay struct {
	store      *Store
	httpClient *http.Client
}

// NewProxyRelay creates a proxy handler for Web AI portals.
func NewProxyRelay(store *Store) *ProxyRelay {
	return &ProxyRelay{
		store: store,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
				MaxIdleConns:        50,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression: false,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// ServeHTTP handles proxying for /web-ai/proxy/{platform}/...
func (p *ProxyRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path format: /web-ai/proxy/{platform}/{rest...}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/web-ai/proxy/"), "/", 2)
	platform := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	var targetBase string
	switch platform {
	case PlatformChatGPT:
		targetBase = "https://chatgpt.com"
	case PlatformClaude:
		targetBase = "https://claude.ai"
	case PlatformGemini:
		targetBase = "https://gemini.google.com"
	case PlatformGrok:
		targetBase = "https://grok.com"
	case PlatformMuseAI:
		targetBase = "https://muse.ai"
	default:
		http.Error(w, "Unsupported web AI platform", http.StatusBadRequest)
		return
	}

	targetURL := targetBase
	if subPath != "" {
		targetURL = targetBase + "/" + subPath
	}
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy standard headers
	for k, vv := range r.Header {
		lower := strings.ToLower(k)
		if lower == "host" || lower == "x-forwarded-for" || lower == "origin" || lower == "referer" {
			continue
		}
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}
	proxyReq.Header.Set("Host", u.Host)
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	proxyReq.Header.Set("Referer", targetBase+"/")
	proxyReq.Header.Set("Origin", targetBase)

	// Inject saved session credentials / cookies if present in store
	if p.store != nil {
		if acc, err := p.store.GetAccount(platform); err == nil && acc != nil {
			var cookies []string
			if existing := proxyReq.Header.Get("Cookie"); existing != "" {
				cookies = append(cookies, existing)
			}
			if acc.Cookie != "" {
				cookies = append(cookies, acc.Cookie)
			}
			if acc.SessionKey != "" {
				switch platform {
				case PlatformClaude:
					cookies = append(cookies, "sessionKey="+acc.SessionKey)
				case PlatformChatGPT:
					proxyReq.Header.Set("Authorization", "Bearer "+acc.SessionKey)
					cookies = append(cookies, "__Secure-next-auth.session-token="+acc.SessionKey)
				case PlatformGemini:
					cookies = append(cookies, "__Secure-1PSID="+acc.SessionKey)
				case PlatformGrok:
					proxyReq.Header.Set("Authorization", "Bearer "+acc.SessionKey)
					cookies = append(cookies, "sso="+acc.SessionKey)
				case PlatformMuseAI:
					proxyReq.Header.Set("Authorization", "Bearer "+acc.SessionKey)
					cookies = append(cookies, "auth_token="+acc.SessionKey)
				}
			}
			if len(cookies) > 0 {
				proxyReq.Header.Set("Cookie", strings.Join(cookies, "; "))
			}
		}
	}

	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("webai: proxy error to %s: %v", targetURL, err)
		// Return friendly embedded fallback html with direct link & sync launcher
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderFallbackHTML(platform, targetBase)))
		return
	}
	defer resp.Body.Close()

	// Intercept Set-Cookie to automatically harvest session tokens and save to store
	if p.store != nil {
		setCookies := resp.Header.Values("Set-Cookie")
		if len(setCookies) > 0 {
			p.harvestCookies(platform, setCookies)
		}
	}

	// Copy response headers, stripping anti-embedding headers
	for k, vv := range resp.Header {
		lower := strings.ToLower(k)
		if lower == "x-frame-options" || lower == "content-security-policy" || lower == "cross-origin-opener-policy" {
			continue // Strip frame-busting headers!
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// Allow embedding from current host
	w.Header().Set("X-Frame-Options", "ALLOWALL")

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *ProxyRelay) harvestCookies(platform string, setCookies []string) {
	fullCookie := strings.Join(setCookies, "; ")
	var sessionKey string
	for _, sc := range setCookies {
		parts := strings.SplitN(sc, ";", 2)
		cookiePair := strings.TrimSpace(parts[0])
		kv := strings.SplitN(cookiePair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if val == "" {
			continue
		}
		switch platform {
		case PlatformChatGPT:
			if name == "__Secure-next-auth.session-token" || name == "accessToken" {
				sessionKey = val
			}
		case PlatformClaude:
			if name == "sessionKey" {
				sessionKey = val
			}
		case PlatformGemini:
			if name == "__Secure-1PSID" || name == "SID" {
				sessionKey = val
			}
		case PlatformGrok:
			if name == "sso" || name == "sso-rw" || name == "auth_token" {
				sessionKey = val
			}
		case PlatformMuseAI:
			if name == "session" || name == "auth_token" {
				sessionKey = val
			}
		}
	}
	if sessionKey != "" || len(setCookies) >= 2 {
		_, _ = p.store.UpsertAccountSession(platform, sessionKey, fullCookie, fmt.Sprintf("Web Login (%s)", time.Now().Format("15:04")), "", "free")
	}
}

func renderFallbackHTML(platform, targetURL string) string {
	name := platform
	switch platform {
	case PlatformChatGPT:
		name = "ChatGPT"
	case PlatformClaude:
		name = "Claude"
	case PlatformGemini:
		name = "Gemini"
	case PlatformGrok:
		name = "Grok (xAI)"
	case PlatformMuseAI:
		name = "Muse AI"
	}

	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; padding: 20px; box-sizing: border-box; text-align: center; }
.card { background: #1e293b; border: 1px solid #334155; border-radius: 16px; padding: 32px; max-width: 520px; box-shadow: 0 10px 25px -5px rgba(0,0,0,0.5); }
h2 { margin-top: 0; font-size: 22px; color: #60a5fa; }
p { color: #94a3b8; font-size: 14px; line-height: 1.6; margin-bottom: 24px; }
.btn { display: inline-block; background: #2563eb; color: #fff; text-decoration: none; padding: 12px 24px; border-radius: 8px; font-weight: 600; font-size: 14px; transition: background 0.2s; }
.btn:hover { background: #1d4ed8; }
.hint { font-size: 12px; color: #64748b; margin-top: 16px; }
</style>
</head>
<body>
<div class="card">
  <h2>Kết Nối ` + name + `</h2>
  <p>Một số nền tảng kích hoạt chính sách bảo mật Cloudflare / Anti-bot trên mạng của bạn. Bạn có thể mở trực tiếp cửa sổ làm việc đồng bộ để sử dụng toàn bộ tính năng như trên web thật.</p>
  <a href="` + targetURL + `" target="_blank" class="btn" rel="noreferrer">🚀 Mở ` + name + ` Trong Cửa Sổ Đồng Bộ</a>
  <div class="hint">Cửa sổ sẽ tự động đồng bộ kịch bản và prompt sản phẩm từ AI Creator OS.</div>
</div>
</body>
</html>`
}
