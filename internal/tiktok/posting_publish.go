package tiktok

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

func (c *PostingClient) CreatorInfo() (map[string]any, error) {
	h, err := c.auth()
	if err != nil {
		return nil, err
	}
	_, raw, err := c.http()("POST", Base+"/v2/post/publish/creator_info/", h, []byte("{}"))
	if err != nil {
		return nil, err
	}
	data := decodeJSON(raw)
	if v, ok := nested(data, "data"); ok {
		if m, ok := v.(map[string]any); ok {
			return m, nil
		}
	}
	return data, nil
}

// PublishFile uploads a local mp4 and (Direct Post) publishes it, returning
// the publish_id. privacyLevel defaults to SELF_ONLY until the Direct Post
// audit passes.
func (c *PostingClient) PublishFile(path, title, privacyLevel string, draft bool) (string, error) {
	if privacyLevel == "" {
		privacyLevel = "SELF_ONLY"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	size := fi.Size()

	// TikTok: total_chunk_count = floor(video_size / chunk_size); the
	// remainder rides on the LAST chunk (which may exceed chunk_size, up
	// to 128 MB). A ceil() count is rejected for any non-multiple size.
	cs := int64(chunkSize)
	if size <= maxSingleChunk {
		cs = size
	}
	chunks := 1
	if cs > 0 && size > cs {
		chunks = int(size / cs)
	}

	endpoint := "/v2/post/publish/video/init/"
	if draft {
		endpoint = "/v2/post/publish/inbox/video/init/"
	}
	h, err := c.auth()
	if err != nil {
		return "", err
	}
	initPayload := map[string]any{
		"source_info": map[string]any{
			"source":            "FILE_UPLOAD",
			"video_size":        size,
			"chunk_size":        cs,
			"total_chunk_count": chunks,
		},
	}
	if !draft {
		initPayload["post_info"] = map[string]any{
			"title":           title,
			"privacy_level":   privacyLevel,
			"disable_comment": false,
			"disable_duet":    false,
			"disable_stitch":  false,
		}
	}
	initBody, _ := json.Marshal(initPayload)
	_, raw, err := c.http()("POST", Base+endpoint, h, initBody)
	if err != nil {
		return "", err
	}
	data := decodeJSON(raw)
	if code, _ := nestedString(data, "error", "code"); code != "ok" {
		return "", fmt.Errorf("init failed: %v", mustMap(data["error"]))
	}
	publishID, ok := nestedString(data, "data", "publish_id")
	if !ok || publishID == "" {
		return "", fmt.Errorf("init failed: no publish_id in %.300v", data)
	}
	uploadURL, ok := nestedString(data, "data", "upload_url")
	if !ok || uploadURL == "" {
		return "", fmt.Errorf("init failed: no upload_url in %.300v", data)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	for i := 0; i < chunks; i++ {
		first := int64(i) * cs
		n := cs
		if i == chunks-1 {
			n = size - first
		}
		blob := make([]byte, n)
		if _, rerr := io.ReadFull(f, blob); rerr != nil {
			return "", fmt.Errorf("chunk %d read: %w", i, rerr)
		}
		last := first + n - 1
		status, _, perr := c.http()("PUT", uploadURL,
			map[string]string{
				"Content-Type":  "video/mp4",
				"Content-Range": fmt.Sprintf("bytes %d-%d/%d", first, last, size),
			}, blob)
		if perr != nil {
			return "", fmt.Errorf("chunk %d upload failed: %w", i, perr)
		}
		if status != 200 && status != 201 && status != 206 {
			return "", fmt.Errorf("chunk %d upload failed: HTTP %d", i, status)
		}
	}
	return publishID, nil
}

// PollStatus polls until PUBLISH_COMPLETE / FAILED. The default 15s interval
// keeps the client under the 6 req/min publish-endpoint rate limit.
func (c *PostingClient) PollStatus(publishID string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	h, err := c.auth()
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"publish_id": publishID})
	for {
		_, raw, err := c.http()("POST", Base+"/v2/post/publish/status/fetch/", h, body)
		if err != nil {
			return "", err
		}
		data := decodeJSON(raw)
		st, _ := nestedString(data, "data", "status")
		if st == "PUBLISH_COMPLETE" || st == "FAILED" {
			return st, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("publish %s not done in %s (last: %s)", publishID, timeout, st)
		}
		time.Sleep(c.pollInterval())
	}
}

// mustMap renders an arbitrary value for error messages.
func mustMap(v any) any {
	if v == nil {
		return map[string]any{}
	}
	return v
}
