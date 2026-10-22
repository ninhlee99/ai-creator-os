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
