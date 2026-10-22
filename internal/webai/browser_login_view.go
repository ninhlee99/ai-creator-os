package webai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// LoginBrowserInput contains one bounded interaction from the remote viewer.
// Pointer coordinates use normalized viewport fractions in the range [0, 1].
type LoginBrowserInput struct {
	Type        string  `json:"type"`
	Action      string  `json:"action,omitempty"`
	PointerType string  `json:"pointer_type,omitempty"`
	X           float64 `json:"x,omitempty"`
	Y           float64 `json:"y,omitempty"`
	Button      string  `json:"button,omitempty"`
	ClickCount  int     `json:"click_count,omitempty"`
	DeltaX      float64 `json:"delta_x,omitempty"`
	DeltaY      float64 `json:"delta_y,omitempty"`
	Text        string  `json:"text,omitempty"`
	Key         string  `json:"key,omitempty"`
}

type loginPageTarget struct {
	Type                 string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type loginCDPError struct {
	Message string `json:"message"`
}

type loginCDPResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *loginCDPError  `json:"error"`
}

type loginViewport struct {
	CSSVisualViewport struct {
		ClientWidth  float64 `json:"clientWidth"`
		ClientHeight float64 `json:"clientHeight"`
	} `json:"cssVisualViewport"`
	CSSLayoutViewport struct {
		ClientWidth  float64 `json:"clientWidth"`
		ClientHeight float64 `json:"clientHeight"`
	} `json:"cssLayoutViewport"`
}

func captureLoginBrowserScreenshot(ctx context.Context, port int) ([]byte, int, int, error) {
	conn, err := dialLoginPage(ctx, port)
	if err != nil {
		return nil, 0, 0, err
	}
	defer conn.Close()

	metricsRaw, err := callLoginPageCDP(conn, "Page.getLayoutMetrics", nil)
	if err != nil {
		return nil, 0, 0, err
	}
	var metrics loginViewport
	if err := json.Unmarshal(metricsRaw, &metrics); err != nil {
		return nil, 0, 0, err
	}
	width := metrics.CSSVisualViewport.ClientWidth
	height := metrics.CSSVisualViewport.ClientHeight
	if width <= 0 || height <= 0 {
		width = metrics.CSSLayoutViewport.ClientWidth
		height = metrics.CSSLayoutViewport.ClientHeight
	}
	if width <= 0 || height <= 0 {
		return nil, 0, 0, errors.New("không đọc được kích thước cửa sổ đăng nhập")
	}

	screenshotRaw, err := callLoginPageCDP(conn, "Page.captureScreenshot", map[string]any{
		"format":                "png",
		"fromSurface":           true,
		"captureBeyondViewport": false,
	})
	if err != nil {
		return nil, 0, 0, err
	}
	var screenshot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(screenshotRaw, &screenshot); err != nil {
		return nil, 0, 0, err
	}
	png, err := base64.StdEncoding.DecodeString(screenshot.Data)
	if err != nil {
		return nil, 0, 0, err
	}
	return png, int(width), int(height), nil
}

func dispatchLoginBrowserInput(ctx context.Context, port int, input LoginBrowserInput) error {
	if !finiteFraction(input.X) || !finiteFraction(input.Y) {
		return errors.New("tọa độ không hợp lệ")
	}
	conn, err := dialLoginPage(ctx, port)
	if err != nil {
		return err
	}
	defer conn.Close()

	switch input.Type {
	case "pointer", "wheel":
		metricsRaw, err := callLoginPageCDP(conn, "Page.getLayoutMetrics", nil)
		if err != nil {
			return err
		}
		var metrics loginViewport
		if err := json.Unmarshal(metricsRaw, &metrics); err != nil {
			return err
		}
		width := metrics.CSSVisualViewport.ClientWidth
		height := metrics.CSSVisualViewport.ClientHeight
		if width <= 0 || height <= 0 {
			width = metrics.CSSLayoutViewport.ClientWidth
			height = metrics.CSSLayoutViewport.ClientHeight
		}
		if width <= 0 || height <= 0 {
			return errors.New("không đọc được kích thước cửa sổ đăng nhập")
		}
		x := input.X * width
		y := input.Y * height
		if x >= width {
			x = width - 1
		}
		if y >= height {
			y = height - 1
		}
		if input.Type == "wheel" {
			if !finiteDelta(input.DeltaX) || !finiteDelta(input.DeltaY) {
				return errors.New("hướng cuộn không hợp lệ")
			}
			_, err = callLoginPageCDP(conn, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseWheel", "x": x, "y": y,
				"deltaX": input.DeltaX, "deltaY": input.DeltaY,
			})
			return err
		}

		if input.PointerType == "touch" {
			if input.Action == "down" {
				if _, err := callLoginPageCDP(conn, "Emulation.setTouchEmulationEnabled", map[string]any{
					"enabled": true, "maxTouchPoints": 1,
				}); err != nil {
					return err
				}
			}
			touchType := map[string]string{"down": "touchStart", "move": "touchMove", "up": "touchEnd"}[input.Action]
			if touchType == "" {
				return errors.New("thao tác chạm không hợp lệ")
			}
			touchPoints := []map[string]any{{"x": x, "y": y, "id": 1, "radiusX": 1, "radiusY": 1, "force": 1}}
			if input.Action == "up" {
				touchPoints = []map[string]any{}
			}
			_, err := callLoginPageCDP(conn, "Input.dispatchTouchEvent", map[string]any{
				"type": touchType, "touchPoints": touchPoints,
			})
			return err
		}
		if input.PointerType != "" && input.PointerType != "mouse" {
			return errors.New("loại con trỏ không hợp lệ")
		}
		button, err := loginMouseButton(input.Button)
		if err != nil {
			return err
		}
		kind := map[string]string{"move": "mouseMoved", "down": "mousePressed", "up": "mouseReleased"}[input.Action]
		if kind == "" {
			return errors.New("thao tác chuột không hợp lệ")
		}
		params := map[string]any{"type": kind, "x": x, "y": y}
		if input.Action == "down" || input.Action == "up" {
			count := input.ClickCount
			if count < 1 || count > 2 {
				count = 1
			}
			params["button"] = button
			params["clickCount"] = count
		}
		if input.Action == "move" && input.Button != "" {
			params["buttons"] = 1
		}
		_, err = callLoginPageCDP(conn, "Input.dispatchMouseEvent", params)
		return err

	case "text":
		if len(input.Text) == 0 || len(input.Text) > 4096 {
			return errors.New("độ dài văn bản không hợp lệ")
		}
		_, err = callLoginPageCDP(conn, "Input.insertText", map[string]any{"text": input.Text})
		return err

	case "key":
		key, ok := loginSpecialKey(input.Key)
		if !ok {
			return errors.New("phím không được hỗ trợ")
		}
		if _, err := callLoginPageCDP(conn, "Input.dispatchKeyEvent", map[string]any{
			"type": "keyDown", "key": key.key, "code": key.code,
			"windowsVirtualKeyCode": key.virtualKey, "nativeVirtualKeyCode": key.virtualKey,
		}); err != nil {
			return err
		}
		_, err = callLoginPageCDP(conn, "Input.dispatchKeyEvent", map[string]any{
			"type": "keyUp", "key": key.key, "code": key.code,
			"windowsVirtualKeyCode": key.virtualKey, "nativeVirtualKeyCode": key.virtualKey,
		})
		return err
	default:
		return errors.New("loại thao tác không hợp lệ")
	}
}

func dialLoginPage(ctx context.Context, port int) (*websocket.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/list", port), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trình duyệt trả trạng thái %d", response.StatusCode)
	}
	var targets []loginPageTarget
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return nil, err
	}
	var pageURL string
	for i := len(targets) - 1; i >= 0; i-- {
		if targets[i].Type == "page" && validLoopbackWebSocketURL(targets[i].WebSocketDebuggerURL) {
			pageURL = targets[i].WebSocketDebuggerURL
			break
		}
	}
	if pageURL == "" {
		return nil, errors.New("chưa có trang đăng nhập để hiển thị")
	}
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).DialContext(ctx, pageURL, nil)
	if err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)
	_ = conn.SetWriteDeadline(deadline)
	return conn, nil
}

func validLoopbackWebSocketURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "ws" || u.User != nil || u.Port() == "" {
		return false
	}
	host := strings.Trim(u.Hostname(), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func callLoginPageCDP(conn *websocket.Conn, method string, params map[string]any) (json.RawMessage, error) {
	id := cdpReqID.Add(1)
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if err := conn.WriteJSON(request); err != nil {
		return nil, err
	}
	for {
		var response loginCDPResponse
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

func finiteFraction(v float64) bool { return v >= 0 && v <= 1 }
func finiteDelta(v float64) bool    { return v >= -1200 && v <= 1200 }

func loginMouseButton(button string) (string, error) {
	switch button {
	case "", "left":
		return "left", nil
	case "right", "middle":
		return button, nil
	default:
		return "", errors.New("nút chuột không hợp lệ")
	}
}

type loginKey struct {
	key        string
	code       string
	virtualKey int
}

func loginSpecialKey(name string) (loginKey, bool) {
	keys := map[string]loginKey{
		"Backspace":  {"Backspace", "Backspace", 8},
		"Delete":     {"Delete", "Delete", 46},
		"Enter":      {"Enter", "Enter", 13},
		"Tab":        {"Tab", "Tab", 9},
		"Escape":     {"Escape", "Escape", 27},
		"ArrowLeft":  {"ArrowLeft", "ArrowLeft", 37},
		"ArrowUp":    {"ArrowUp", "ArrowUp", 38},
		"ArrowRight": {"ArrowRight", "ArrowRight", 39},
		"ArrowDown":  {"ArrowDown", "ArrowDown", 40},
		"Home":       {"Home", "Home", 36},
		"End":        {"End", "End", 35},
	}
	k, ok := keys[name]
	return k, ok
}
