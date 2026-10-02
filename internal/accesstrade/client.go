// Package accesstrade là client cho Accesstrade Vietnam Publisher API
// (https://developers.accesstrade.vn), trụ affiliate của app.
//
// Xác thực: mọi request mang header `Authorization: Token <access_key>`.
// Key KHÔNG bao giờ được log (mask ••••abcd khi cần hiển thị).
//
// Không gọi API thật trong test — dùng httptest.Server.
package accesstrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BaseURL mặc định của Accesstrade Publisher API.
const DefaultBaseURL = "https://api.accesstrade.vn"

// requestTimeout là timeout mỗi lần gọi API.
const requestTimeout = 20 * time.Second

// maxRetries là số lần thử lại cho lỗi mạng / 5xx. Không retry 4xx.
const maxRetries = 2

// Client gọi Accesstrade API. Zero value chưa dùng được — tạo qua NewClient.
type Client struct {
	// BaseURL cho phép trỏ sang server mock trong test.
	BaseURL string
	key     string
	http    *http.Client
	// orderLimit giới hạn nội bộ 10 req/phút cho /v1/order-list
	// (spec Accesstrade). Lazy-init ở ListOrders.
	orderLimit *rateLimiter
}

// NewClient tạo client với access_key của publisher.
func NewClient(accessKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		key:     accessKey,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// maskedKey trả về dạng ••••abcd để log/toast an toàn.
func (c *Client) maskedKey() string {
	if len(c.key) <= 4 {
		return "••••"
	}
	return "••••" + c.key[len(c.key)-4:]
}

// apiError là lỗi HTTP từ API (mang status code, không chứa key).
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("accesstrade: HTTP %d: %s", e.status, e.msg)
}

// do thực hiện request: gắn header auth, retry lỗi mạng/5xx tối đa
// maxRetries lần, không retry 4xx. Trả về body đã đọc.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if c.key == "" {
		return nil, fmt.Errorf("accesstrade: chưa có access_key — nhập key ở Cài đặt · Accesstrade")
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("accesstrade: encode request: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
		if err != nil {
			return nil, fmt.Errorf("accesstrade: tạo request: %w", err)
		}
		// Format auth theo đúng spec: "Token <access_key>".
		req.Header.Set("Authorization", "Token "+c.key)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			// Lỗi mạng → retry (key không nằm trong err của http.Client).
			lastErr = fmt.Errorf("accesstrade: lỗi mạng (lần %d, key %s): %w", attempt+1, c.maskedKey(), err)
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("accesstrade: đọc response: %w", readErr)
			continue
		}
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return data, nil
		case resp.StatusCode >= 500:
			// 5xx → retry.
			lastErr = &apiError{status: resp.StatusCode, msg: snippet(data)}
			continue
		default:
			// 4xx (kể cả 429) → không retry, báo lỗi rõ để UI toast.
			return nil, &apiError{status: resp.StatusCode, msg: snippet(data)}
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("accesstrade: hết số lần thử lại")
}

// snippet cắt body lỗi để hiển thị (tối đa 200 ký tự, không chứa key).
func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// decodeList tách mảng kết quả khỏi các dạng envelope API có thể trả:
// {"data":[...]}, {"campaigns":[...]}, {"results":[...]}, {"items":[...]}
// hoặc mảng trần [...]. Dạng nào không khớp → lỗi rõ ràng.
func decodeList(data []byte, keys ...string) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("accesstrade: response không phải JSON hợp lệ")
	}
	for _, k := range keys {
		if raw, ok := env[k]; ok {
			if err := json.Unmarshal(raw, &arr); err == nil {
				return arr, nil
			}
		}
	}
	return nil, fmt.Errorf("accesstrade: không tìm thấy danh sách trong response")
}

// firstString lấy field string đầu tiên tồn tại trong map (cho response
// có tên field khác nhau giữa các endpoint).
func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil && s != "" {
				return s
			}
			// Một số field có thể là số — ép về string.
			var f float64
			if err := json.Unmarshal(raw, &f); err == nil {
				return fmt.Sprintf("%v", f)
			}
		}
	}
	return ""
}
