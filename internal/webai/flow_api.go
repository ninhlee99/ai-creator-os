package webai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	flowLabsBase = "https://labs.google/fx/api/trpc"
	flowVideoAPI = "https://aisandbox-pa.googleapis.com/v1"
	// Public key embedded in Google Flow's web client. User supplies Google
	// session cookies only; this is not a Google AI Studio developer key.
	flowWebClientKey = "AIzaSyBtrm0o5ab1c-Ec8ZuLcGt3oJAA5VWt3pY"
)

type FlowVideoResult struct {
	MediaURL     string
	ProjectID    string
	MediaID      string
	CookieHeader string
	CookieJar    []BrowserCookie
}

// FlowAPIUnavailableError marks a failed preflight or an explicit rejected
// generation request. The browser fallback is safe only when Flow did not
// accept a generation request.
type FlowAPIUnavailableError struct{ cause error }

func (e *FlowAPIUnavailableError) Error() string {
	return "Google Flow direct API unavailable: " + e.cause.Error()
}
func (e *FlowAPIUnavailableError) Unwrap() error { return e.cause }

// flowUIRateLimitError marks an explicit quota rejection shown by Flow after
// the UI submit action. The visible rejection confirms generation was not accepted.
type flowUIRateLimitError struct{ message string }

func (e *flowUIRateLimitError) Error() string {
	return "Google Flow rejected generation before acceptance: " + e.message
}

// CanFallbackToGoogleFlowHeadless is true only for errors known to happen
// before a generation was accepted; post-submit failures must never retry.
func CanFallbackToGoogleFlowHeadless(err error) bool {
	var unavailable *FlowAPIUnavailableError
	return errors.As(err, &unavailable)
}

// FlowPreSubmitRateLimitCooldown returns provider Retry-After for a rate limit
// known to occur before Flow accepted a generation request.
func FlowPreSubmitRateLimitCooldown(err error) (time.Duration, bool) {
	var uiLimit *flowUIRateLimitError
	if errors.As(err, &uiLimit) {
		return 15 * time.Minute, true
	}
	var unavailable *FlowAPIUnavailableError
	var responseErr *flowHTTPError
	if !errors.As(err, &unavailable) || !errors.As(err, &responseErr) || responseErr.status != http.StatusTooManyRequests {
		return 0, false
	}
	if responseErr.retryAfter > 0 {
		return responseErr.retryAfter, true
	}
	return 15 * time.Minute, true
}

func IsWebSessionAuthError(err error) bool {
	var sessionErr *WebSessionAuthError
	if errors.As(err, &sessionErr) {
		// HTTP 403 can be a Cloudflare or access-policy block. Only 401 proves
		// that provider rejected the saved session itself.
		return sessionErr.StatusCode == http.StatusUnauthorized
	}
	var flowErr *flowHTTPError
	return errors.As(err, &flowErr) && flowErr.status == http.StatusUnauthorized
}

type flowHTTPError struct {
	status     int
	retryAfter time.Duration
}

func (e *flowHTTPError) Error() string { return fmt.Sprintf("Flow HTTP %d", e.status) }

type flowClient struct {
	http           *http.Client
	cookie         string
	csrf           string
	sessionID      string
	onCookieUpdate func(string) error
	snapshotJar    *cookieSnapshotJar
}

// ProbeGoogleFlowSession checks access to the authenticated Flow service.
func ProbeGoogleFlowSession(ctx context.Context, cookie string) error {
	return ProbeGoogleFlowSessionWithCookieUpdate(ctx, cookie, nil)
}

// ProbeGoogleFlowSessionWithCookieUpdate persists cookies rotated by history.
func ProbeGoogleFlowSessionWithCookieUpdate(ctx context.Context, cookie string, onCookieUpdate func(string) error) error {
	updater := func(header string, _ []BrowserCookie) error {
		if onCookieUpdate != nil {
			return onCookieUpdate(header)
		}
		return nil
	}
	return ProbeGoogleFlowSessionWithCookieSnapshot(ctx, cookie, nil, updater)
}

// ProbeGoogleFlowSessionWithCookieSnapshot probes Flow using scoped cookies and
// persists every response cookie together with its domain and path metadata.
func ProbeGoogleFlowSessionWithCookieSnapshot(ctx context.Context, cookie string, cookies []BrowserCookie, updater CookieSnapshotUpdater) error {
	if strings.TrimSpace(cookie) == "" && len(cookies) == 0 {
		return errors.New("Google Flow chưa có cookie phiên")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newFlowClientWithCookieSnapshot(cookie, cookies, updater)
	if err != nil {
		return fmt.Errorf("không khởi tạo được cookie jar của Flow: %w", err)
	}

	// 1. Kiểm tra trực tiếp dịch vụ web Google Flow (https://flow.google.com/)
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, flowAppURL, nil)
	if reqErr == nil {
		req.Header.Set("User-Agent", museUserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		resp, doErr := client.http.Do(req)
		if doErr == nil {
			defer resp.Body.Close()
			finalURL := ""
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.String()
			}
			isLoginRedirect := strings.Contains(finalURL, "accounts.google.com") ||
				strings.Contains(finalURL, "/signin") ||
				strings.Contains(finalURL, "/login") ||
				strings.Contains(finalURL, "/servicelogin")
			if resp.StatusCode == http.StatusOK && !isLoginRedirect {
				return nil
			}
			if resp.StatusCode == http.StatusUnauthorized || isLoginRedirect {
				return &flowHTTPError{status: http.StatusUnauthorized}
			}
		}
	}

	// 2. Dự phòng kiểm tra qua Flow history API nếu labs.google khả dụng
	_, err = client.history(ctx, "PINHOLE", "", 1)
	return err
}

// GoogleFlowVideo calls Flow's web-client endpoints with a saved Google
// session, then correlates its generation against the authenticated history.
func GoogleFlowVideo(ctx context.Context, cookie, prompt, aspect string, progress func(string)) (result FlowVideoResult, retErr error) {
	return GoogleFlowVideoWithCookieUpdate(ctx, cookie, prompt, aspect, progress, nil)
}

// GoogleFlowVideoWithCookieUpdate reports each changed Set-Cookie snapshot
// before the next Flow request uses the updated session.
func GoogleFlowVideoWithCookieUpdate(ctx context.Context, cookie, prompt, aspect string, progress func(string), onCookieUpdate func(string) error) (result FlowVideoResult, retErr error) {
	client := newFlowClientWithCookieUpdate(cookie, onCookieUpdate)
	return googleFlowVideoWithClient(ctx, cookie, prompt, aspect, progress, client)
}

// GoogleFlowVideoWithCookieSnapshot sends imported cookies with their original
// browser domain/path scope and persists each rotated snapshot before reuse.
func GoogleFlowVideoWithCookieSnapshot(ctx context.Context, cookie string, cookies []BrowserCookie, prompt, aspect string, progress func(string), updater CookieSnapshotUpdater) (result FlowVideoResult, retErr error) {
	client, err := newFlowClientWithCookieSnapshot(cookie, cookies, updater)
	if err != nil {
		return result, fmt.Errorf("không khởi tạo được cookie jar của Flow: %w", err)
	}
	return googleFlowVideoWithClient(ctx, cookie, prompt, aspect, progress, client)
}

func googleFlowVideoWithClient(ctx context.Context, cookie, prompt, aspect string, progress func(string), client *flowClient) (result FlowVideoResult, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cookie) == "" && client.snapshotJar == nil {
		return result, errors.New("Google Flow chưa có cookie phiên; kết nối tài khoản Flow trước")
	}
	if strings.TrimSpace(prompt) == "" {
		return result, errors.New("Google Flow yêu cầu prompt không rỗng")
	}
	if progress == nil {
		progress = func(string) {}
	}
	defer func() {
		if client.cookie != "" {
			result.CookieHeader = client.cookie
		}
		if client.snapshotJar != nil {
			result.CookieJar = client.snapshotJar.Snapshot()
			result.CookieHeader = client.snapshotJar.CookieHeader()
		}
	}()
	before, err := client.history(ctx, "PINHOLE", "", 25)
	if err != nil {
		return result, &FlowAPIUnavailableError{cause: fmt.Errorf("không đọc được lịch sử Flow: %w", err)}
	}
	baseline := map[string]bool{}
	for _, workflow := range before {
		if id := flowMediaID(workflow); id != "" {
			baseline[id] = true
		}
	}
	projectID, err := client.createProject(ctx, "AICOS · "+time.Now().Format("2006-01-02 15:04"))
	if err != nil {
		return result, &FlowAPIUnavailableError{cause: fmt.Errorf("Google Flow không tạo được project: %w", err)}
	}
	result.ProjectID = projectID
	if aspect != "VIDEO_ASPECT_RATIO_LANDSCAPE" && aspect != "VIDEO_ASPECT_RATIO_PORTRAIT" && aspect != "VIDEO_ASPECT_RATIO_SQUARE" {
		aspect = "VIDEO_ASPECT_RATIO_PORTRAIT"
	}
	seed := time.Now().UnixMilli() % 2147483647
	sceneID := flowUUID()
	payload := map[string]any{
		"clientContext": map[string]any{
			"sessionId":       client.sessionID,
			"projectId":       projectID,
			"tool":            "PINHOLE",
			"userPaygateTier": "PAYGATE_TIER_ONE",
		},
		"requests": []any{map[string]any{
			"aspectRatio":   aspect,
			"seed":          seed,
			"textInput":     map[string]string{"prompt": prompt},
			"videoModelKey": "veo_3_1_t2v_fast",
			"metadata":      map[string]string{"sceneId": sceneID},
		}},
	}
	var generated map[string]any
	if err := client.requestJSON(ctx, http.MethodPost, flowVideoAPI+"/video:batchAsyncGenerateVideoText?key="+url.QueryEscape(flowWebClientKey), payload, &generated); err != nil {
		var responseErr *flowHTTPError
		if errors.As(err, &responseErr) && responseErr.status >= 400 && responseErr.status < 500 {
			return result, &FlowAPIUnavailableError{cause: fmt.Errorf("Flow rejected the request before starting video: %w", err)}
		}
		return result, fmt.Errorf("Google Flow video request failed: %w", err)
	}
	if ops, ok := generated["operations"].([]any); !ok || len(ops) == 0 {
		return result, errors.New("Google Flow accepted no video operation")
	}
	progress("FLOW_GENERATE: Flow đã nhận yêu cầu; chờ media trong lịch sử")

	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		workflows, err := client.history(ctx, "PINHOLE", projectID, 25)
		if err == nil {
			for _, workflow := range workflows {
				mediaID := flowMediaID(workflow)
				if mediaID == "" || baseline[mediaID] {
					continue
				}
				if flowProjectID(workflow) != "" && flowProjectID(workflow) != projectID {
					continue
				}
				mediaURL, err := client.mediaURL(ctx, mediaID)
				if err != nil {
					continue
				}
				if !isGoogleMediaURL(mediaURL) {
					continue
				}
				result = FlowVideoResult{MediaURL: mediaURL, ProjectID: projectID, MediaID: mediaID, CookieHeader: client.cookie}
				if client.snapshotJar != nil {
					result.CookieJar = client.snapshotJar.Snapshot()
					result.CookieHeader = client.snapshotJar.CookieHeader()
				}
				return result, nil
			}
		}
		progress("FLOW_WAIT: đang đợi Flow hoàn tất video")
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return result, errors.New("Google Flow chưa đưa video mới vào lịch sử sau 12 phút")
}
