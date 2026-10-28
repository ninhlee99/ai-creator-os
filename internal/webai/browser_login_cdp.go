package webai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func debuggerPortFromProfile(profile string) int {
	contents, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(contents))
	if len(fields) == 0 {
		return 0
	}
	port, _ := strconv.Atoi(fields[0])
	return port
}

func debuggerWSURLFromProfile(profile string) (string, error) {
	port := debuggerPortFromProfile(profile)
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("Chromium did not publish DevToolsActivePort")
	}
	return debuggerWSURL(port)
}

func debuggerWSURL(port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var meta struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	if meta.WS == "" {
		return "", fmt.Errorf("webSocketDebuggerUrl rỗng")
	}
	return meta.WS, nil
}

var cdpReqID atomic.Int64

func closeLoginBrowser(wsURL string) {
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	_ = conn.WriteJSON(map[string]any{"id": cdpReqID.Add(1), "method": "Browser.close"})
}

func cdpListCookies(wsURL string) ([]cdpCookie, error) {
	cookies, err := cdpCallCookies(wsURL, "Storage.getCookies")
	if err == nil && len(cookies) > 0 {
		return cookies, nil
	}
	alt, err2 := cdpCallCookies(wsURL, "Network.getAllCookies")
	if err2 == nil && len(alt) > 0 {
		return alt, nil
	}
	if err != nil {
		return nil, err
	}
	return cookies, err2
}
func cdpCallCookies(wsURL, method string) ([]cdpCookie, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))

	id := cdpReqID.Add(1)
	req := map[string]any{"id": id, "method": method}
	if err := conn.WriteJSON(req); err != nil {
		return nil, err
	}

	for {
		var msg struct {
			ID     int64 `json:"id"`
			Result struct {
				Cookies []cdpCookie `json:"cookies"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, err
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("cdp %s: %s", method, msg.Error.Message)
		}
		return msg.Result.Cookies, nil
	}
}
