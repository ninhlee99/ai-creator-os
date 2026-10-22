package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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
	c.cookie = mergeFlowCookieHeader(c.cookie, resp.Cookies())
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return &WebSessionAuthError{Platform: PlatformGoogleFlow, StatusCode: resp.StatusCode}
		}
		return &flowHTTPError{status: resp.StatusCode, retryAfter: retryAfterDuration(resp)}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst); err != nil {
		return errors.New("Flow returned invalid JSON")
	}
	return nil
}

func retryAfterDuration(resp *http.Response) time.Duration {
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
	}
	return 0
}
