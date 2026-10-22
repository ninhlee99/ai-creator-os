package webai

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func aicosProfileDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".aicos", "browser-profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
func pickFreePort() (int, net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	return ln.Addr().(*net.TCPAddr).Port, ln, nil
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

type cdpCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
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
