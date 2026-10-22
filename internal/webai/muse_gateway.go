package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	museHTTPBase  = "https://muse.ai"
	museWSBase    = "wss://hatch.metaaivm.com/v1/noise"
	museUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"
)

// MuseReply contains the text returned by Muse and media links explicitly
// present in that reply. Muse's chat transport is an internal Noise gateway.
type MuseReply struct {
	Text         string
	MediaURLs    []string
	CookieHeader string
}

type MuseHTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *MuseHTTPError) Error() string { return fmt.Sprintf("Muse HTTP %d", e.StatusCode) }

// MusePreSubmitRateLimitCooldown is true only for 429 responses wrapped by
// MuseGatewayUnavailableError, which is emitted before chat.stream is sent.
func MusePreSubmitRateLimitCooldown(err error) (time.Duration, bool) {
	var gatewayErr *MuseGatewayUnavailableError
	var responseErr *MuseHTTPError
	if !errors.As(err, &gatewayErr) || !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if responseErr.RetryAfter > 0 {
		return responseErr.RetryAfter, true
	}
	return 15 * time.Minute, true
}

// ProbeMuseSession validates saved Muse credentials without opening a browser
// or sending a chat message.
func ProbeMuseSession(ctx context.Context, cookie string) error {
	if strings.TrimSpace(cookie) == "" {
		return errors.New("Muse Chat chưa có cookie phiên")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	g, err := newMuseGateway(ctx, cookie)
	if err != nil {
		return err
	}
	defer g.Close()
	_, err = g.latestAssistantSeq(ctx)
	return err
}

// MuseChat sends one prompt through Muse's authenticated Hatch gateway and
// polls chat.history until a genuine assistant reply arrives.
func MuseChat(ctx context.Context, cookie, deviceID, prompt string, progress func(string)) (out MuseReply, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cookie) == "" {
		return out, errors.New("Muse Chat chưa có cookie phiên; kết nối tài khoản Muse trước")
	}
	if strings.TrimSpace(prompt) == "" {
		return out, errors.New("Muse Chat yêu cầu prompt không rỗng")
	}
	if progress == nil {
		progress = func(string) {}
	}
	g, err := newMuseGateway(ctx, cookie)
	if err != nil {
		return out, &MuseGatewayUnavailableError{cause: err}
	}
	defer g.Close()
	defer func() { out.CookieHeader = g.cookieHeader() }()
	stopOnCancel := context.AfterFunc(ctx, g.Close)
	defer stopOnCancel()

	baseline, err := g.latestAssistantSeq(ctx)
	if err != nil {
		return out, &MuseGatewayUnavailableError{cause: fmt.Errorf("Muse Chat không đọc được lịch sử: %w", err)}
	}
	progress("MUSE_SEND: gửi prompt tới Muse Chat")
	if err := g.sendPrompt(ctx, museDeviceID(deviceID), prompt); err != nil {
		return out, fmt.Errorf("Muse Chat không nhận prompt: %w", err)
	}

	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		events, err := g.history(ctx, 20)
		if err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			progress("MUSE_WAIT: chờ phản hồi Muse Chat")
		} else {
			for _, event := range events {
				if isMuseReply(event, baseline) {
					text := museEventText(event)
					if strings.TrimSpace(text) == "" {
						continue
					}
					out.Text = text
					out.MediaURLs = extractMuseMediaURLs(event, text)
					out.CookieHeader = g.cookieHeader()
					return out, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
	return out, errors.New("Muse Chat đã nhận prompt nhưng chưa trả lời trong 12 phút")
}

type museGateway struct {
	conn      *websocket.Conn
	ctx       context.Context
	client    *http.Client
	streamID  uint64
	sendKey   []byte
	recvKey   []byte
	sendNonce uint64
	recvNonce uint64
}

func newMuseGateway(ctx context.Context, cookie string) (*museGateway, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 25 * time.Second, Jar: jar}
	if museURL, err := url.Parse(museHTTPBase); err == nil {
		jar.SetCookies(museURL, parseCookieHeader(cookie))
	}
	access, err := museFetchAccess(ctx, client, cookie)
	if err != nil {
		return nil, err
	}
	vmID, err := museFetchVM(ctx, client, cookie)
	if err != nil {
		return nil, err
	}
	hatchToken, err := museFetchHatchToken(ctx, client, cookie, access, vmID)
	if err != nil {
		return nil, err
	}

	endpoint, _ := url.Parse(museWSBase)
	q := endpoint.Query()
	q.Set("vm_id", vmID)
	q.Set("auth_token", hatchToken)
	endpoint.RawQuery = q.Encode()
	header := http.Header{}
	header.Set("Origin", museHTTPBase)
	header.Set("User-Agent", museUserAgent)
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 25 * time.Second}
	conn, response, err := dialer.DialContext(ctx, endpoint.String(), header)
	if err != nil {
		if response != nil {
			if response.StatusCode == http.StatusTooManyRequests {
				return nil, &MuseHTTPError{StatusCode: response.StatusCode, RetryAfter: retryAfterDuration(response)}
			}
			return nil, fmt.Errorf("Muse gateway websocket handshake failed (HTTP %d)", response.StatusCode)
		}
		return nil, errors.New("Muse gateway websocket connection failed")
	}
	g := &museGateway{conn: conn, ctx: ctx, client: client, streamID: 1}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	if err := g.handshake(); err != nil {
		stop()
		_ = conn.Close()
		return nil, err
	}
	stop()
	return g, nil
}

func (g *museGateway) Close() {
	if g != nil && g.conn != nil {
		_ = g.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = g.conn.Close()
	}
}

func museFetchAccess(ctx context.Context, client *http.Client, cookie string) (string, error) {
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodPost, museHTTPBase+"/api/auth/check", cookie, "", nil, &data); err != nil {
		return "", fmt.Errorf("Muse authentication check failed: %w", err)
	}
	token, _ := data["access_token"].(string)
	if token == "" {
		return "", errors.New("Muse session expired or auth response omitted access token")
	}
	return token, nil
}

func museFetchVM(ctx context.Context, client *http.Client, cookie string) (string, error) {
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodGet, museHTTPBase+"/api/session", cookie, "", nil, &data); err != nil {
		return "", fmt.Errorf("Muse session lookup failed: %w", err)
	}
	if status, _ := data["status"].(string); status == "unavailable" {
		return "", errors.New("Muse VM is temporarily unavailable; retry later")
	}
	vmID, _ := data["vm_id"].(string)
	if vmID == "" {
		return "", errors.New("Muse session response omitted VM id")
	}
	return vmID, nil
}

func museFetchHatchToken(ctx context.Context, client *http.Client, cookie, access, vmID string) (string, error) {
	body := map[string]string{"vmAddress": "wss://" + vmID + ".metaaivm.com/", "vmName": vmID}
	var data map[string]any
	if err := museHTTPJSON(ctx, client, http.MethodPost, museHTTPBase+"/api/hatch/token", cookie, access, body, &data); err != nil {
		return "", fmt.Errorf("Muse gateway token request failed: %w", err)
	}
	token, _ := data["token"].(string)
	if token == "" {
		return "", errors.New("Muse gateway response omitted access token")
	}
	return token, nil
}

func museHTTPJSON(ctx context.Context, client *http.Client, method, target, cookie, bearer string, payload any, dst any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", museUserAgent)
	req.Header.Set("Referer", museHTTPBase+"/")
	req.Header.Set("Origin", museHTTPBase)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if client.Jar == nil {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return &WebSessionAuthError{Platform: PlatformMuseAI, StatusCode: resp.StatusCode}
		}
		return &MuseHTTPError{StatusCode: resp.StatusCode, RetryAfter: retryAfterDuration(resp)}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(dst); err != nil {
		return errors.New("invalid JSON response")
	}
	return nil
}

func parseCookieHeader(header string) []*http.Cookie {
	var cookies []*http.Cookie
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" || value == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}
	return cookies
}

func (g *museGateway) cookieHeader() string {
	if g == nil || g.client == nil || g.client.Jar == nil {
		return ""
	}
	museURL, err := url.Parse(museHTTPBase)
	if err != nil {
		return ""
	}
	var pairs []string
	for _, cookie := range g.client.Jar.Cookies(museURL) {
		if cookie.Name != "" && cookie.Value != "" {
			pairs = append(pairs, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(pairs, "; ")
}
