package webai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	MediaURL  string
	ProjectID string
	MediaID   string
}

// FlowAPIUnavailableError marks a failed preflight or an explicit rejected
// generation request. The browser fallback is safe only when Flow did not
// accept a generation request.
type FlowAPIUnavailableError struct{ cause error }

func (e *FlowAPIUnavailableError) Error() string {
	return "Google Flow direct API unavailable: " + e.cause.Error()
}
func (e *FlowAPIUnavailableError) Unwrap() error { return e.cause }

// CanFallbackToGoogleFlowHeadless is true only for errors known to happen
// before a generation was accepted; post-submit failures must never retry.
func CanFallbackToGoogleFlowHeadless(err error) bool {
	var unavailable *FlowAPIUnavailableError
	return errors.As(err, &unavailable)
}

type flowHTTPError struct{ status int }

func (e *flowHTTPError) Error() string { return fmt.Sprintf("Flow HTTP %d", e.status) }

type flowClient struct {
	http      *http.Client
	cookie    string
	csrf      string
	sessionID string
}

// ProbeGoogleFlowSession checks access to the authenticated Flow history API.
func ProbeGoogleFlowSession(ctx context.Context, cookie string) error {
	if strings.TrimSpace(cookie) == "" {
		return errors.New("Google Flow chưa có cookie phiên")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := newFlowClient(cookie).history(ctx, "PINHOLE", "", 1)
	return err
}

// GoogleFlowVideo calls Flow's web-client endpoints with a saved Google
// session, then correlates its generation against the authenticated history.
func GoogleFlowVideo(ctx context.Context, cookie, prompt, aspect string, progress func(string)) (FlowVideoResult, error) {
	var result FlowVideoResult
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cookie) == "" {
		return result, errors.New("Google Flow chưa có cookie phiên; kết nối tài khoản Flow trước")
	}
	if strings.TrimSpace(prompt) == "" {
		return result, errors.New("Google Flow yêu cầu prompt không rỗng")
	}
	if progress == nil {
		progress = func(string) {}
	}
	client := newFlowClient(cookie)
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
				result = FlowVideoResult{MediaURL: mediaURL, ProjectID: projectID, MediaID: mediaID}
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

func DownloadFlowVideo(ctx context.Context, mediaURL, outPath string) error {
	if !isGoogleMediaURL(mediaURL) {
		return errors.New("Google Flow media URL không được cho phép")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return errors.New("invalid Google Flow media URL")
	}
	req.Header.Set("User-Agent", museUserAgent)
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Google Flow media download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Google Flow media HTTP %d", resp.StatusCode)
	}
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".aicos-flow-*.mp4")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	written, err := io.Copy(f, io.LimitReader(resp.Body, 250<<20+1))
	if err != nil {
		_ = f.Close()
		return err
	}
	if written < 12 || written > 250<<20 {
		_ = f.Close()
		return errors.New("Google Flow video size is invalid or exceeds 250 MB")
	}
	if _, err := f.Seek(4, io.SeekStart); err != nil {
		_ = f.Close()
		return err
	}
	var signature [4]byte
	if _, err := io.ReadFull(f, signature[:]); err != nil || string(signature[:]) != "ftyp" {
		_ = f.Close()
		return errors.New("Google Flow media is not an MP4 file")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return err
	}
	return nil
}

func newFlowClient(cookie string) *flowClient {
	csrf := ""
	for _, part := range strings.Split(cookie, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(name), "csrf-token") {
			csrf = strings.SplitN(value, "|", 2)[0]
			break
		}
	}
	return &flowClient{
		http:      &http.Client{Timeout: 45 * time.Second},
		cookie:    cookie,
		csrf:      csrf,
		sessionID: ";" + fmt.Sprint(time.Now().UnixMilli()),
	}
}

func (c *flowClient) createProject(ctx context.Context, title string) (string, error) {
	body := map[string]any{"json": map[string]any{"projectTitle": title, "toolName": "PINHOLE"}}
	var response map[string]any
	if err := c.requestJSON(ctx, http.MethodPost, flowLabsBase+"/project.createProject", body, &response); err != nil {
		return "", err
	}
	project := findJSONKey(response, "projectId")
	if project == "" {
		return "", errors.New("Flow project response omitted project ID")
	}
	return project, nil
}

func (c *flowClient) history(ctx context.Context, kind, projectID string, limit int) ([]map[string]any, error) {
	input := map[string]any{"json": map[string]any{
		"type":          kind,
		"pageSize":      limit,
		"responseScope": "RESPONSE_SCOPE_UNSPECIFIED",
		"cursor":        nil,
	}, "meta": map[string]any{"values": map[string]any{"cursor": []string{"undefined"}}}}
	if projectID != "" {
		input["json"].(map[string]any)["projectId"] = projectID
	}
	encoded, _ := json.Marshal(input)
	target := flowLabsBase + "/media.fetchUserHistoryDirectly?input=" + url.QueryEscape(string(encoded))
	var response map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return nil, err
	}
	workflows := findJSONArray(response, "userWorkflows")
	result := make([]map[string]any, 0, len(workflows))
	for _, item := range workflows {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result, nil
}

func (c *flowClient) mediaURL(ctx context.Context, mediaID string) (string, error) {
	target := flowVideoAPI + "/media/" + url.PathEscape(mediaID) + "?key=" + url.QueryEscape(flowWebClientKey) + "&clientContext.tool=PINHOLE&returnUriOnly=true"
	var response map[string]any
	if err := c.requestJSON(ctx, http.MethodGet, target, nil, &response); err != nil {
		return "", err
	}
	for _, key := range []string{"fifeUrl", "uri", "url", "videoUrl"} {
		if link := findJSONKey(response, key); link != "" && strings.HasPrefix(link, "https://") {
			return link, nil
		}
	}
	return "", errors.New("Flow media response omitted download URL")
}

func (c *flowClient) requestJSON(ctx context.Context, method, target string, body any, dst any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return errors.New("invalid Flow request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://labs.google")
	req.Header.Set("Referer", "https://labs.google/fx/tools/flow")
	req.Header.Set("User-Agent", museUserAgent)
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("x-goog-authuser", "0")
	if c.csrf != "" {
		req.Header.Set("x-csrf-token", c.csrf)
		req.Header.Set("x-next-auth-csrf-token", c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("Flow network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &flowHTTPError{status: resp.StatusCode}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst); err != nil {
		return errors.New("Flow returned invalid JSON")
	}
	return nil
}

func flowMediaID(workflow map[string]any) string {
	if value, ok := workflow["name"].(string); ok {
		return value
	}
	return findJSONKey(workflow, "mediaGenerationId")
}

func flowProjectID(workflow map[string]any) string { return findJSONKey(workflow, "projectId") }

func findJSONKey(value any, wanted string) string {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if key == wanted {
				if text, ok := child.(string); ok {
					return text
				}
			}
			if found := findJSONKey(child, wanted); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range item {
			if found := findJSONKey(child, wanted); found != "" {
				return found
			}
		}
	}
	return ""
}

func findJSONArray(value any, wanted string) []any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if key == wanted {
				if rows, ok := child.([]any); ok {
					return rows
				}
			}
			if rows := findJSONArray(child, wanted); rows != nil {
				return rows
			}
		}
	case []any:
		for _, child := range item {
			if rows := findJSONArray(child, wanted); rows != nil {
				return rows
			}
		}
	}
	return nil
}

func isGoogleMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".googleusercontent.com", ".googleapis.com", ".google.com", ".usercontent.goog", ".gvt1.com"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return host == "googleusercontent.com" || host == "googleapis.com" || host == "flow-content.google"
}

func flowUUID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("flow-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
