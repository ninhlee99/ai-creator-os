package publishers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// facebookKinds are the content kinds the Facebook publisher handles.
var facebookKinds = []string{"short_video", "short_film"}

// FacebookPublisher posts managed-Page videos via the `facebook-cli`
// binary, which handles its own auth + approval flow.
//
// Draft-first: creates a native Page draft with the MP4 attached, then
// publishes the same draft (skipped when FB_DRAFT_ONLY=1 so a human can
// review the draft in the Page first).
//
// Per-account Page mapping: FB_PAGE_ID_<USERNAME>, else shared FB_PAGE_ID.
type FacebookPublisher struct {
	Username  string
	pageID    string
	draftOnly bool
}

// NewFacebookPublisher wires FB_PAGE_ID[_<USERNAME>] and FB_DRAFT_ONLY
// (default "1") from the environment.
func NewFacebookPublisher(username string) *FacebookPublisher {
	draftOnly := true
	if v, ok := os.LookupEnv("FB_DRAFT_ONLY"); ok {
		draftOnly = v == "1"
	}
	return &FacebookPublisher{
		Username:  username,
		pageID:    envFor(username, "FB_PAGE_ID"),
		draftOnly: draftOnly,
	}
}

// Name returns the platform key.
func (p *FacebookPublisher) Name() string { return "facebook" }

// IsConfigured reports whether a page id exists and facebook-cli is on PATH.
func (p *FacebookPublisher) IsConfigured() bool {
	if p.pageID == "" {
		return false
	}
	_, err := exec.LookPath("facebook-cli")
	return err == nil
}

// Handles reports whether kind is one of short_video, short_film.
func (p *FacebookPublisher) Handles(kind string) bool { return handlesKind(kind, facebookKinds) }

// newRequestID generates a random idempotency key (uuid4-shaped).
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-fallback"
	}
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// run invokes facebook-cli and parses its JSON stdout; on parse failure it
// returns a diagnostic map mirroring the Python _raw/_stderr/_exit shape.
func (p *FacebookPublisher) run(args ...string) map[string]any {
	cmd := exec.Command("facebook-cli", args...)
	out, err := cmd.Output()
	var stderr string
	if ee, ok := err.(*exec.ExitError); ok {
		stderr = string(ee.Stderr)
	}
	var m map[string]any
	if jerr := json.Unmarshal(out, &m); jerr == nil && m != nil {
		return m
	}
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return map[string]any{
		"_raw":    truncate(string(out), 500),
		"_stderr": truncate(stderr, 500),
		"_exit":   code,
	}
}

// firstID pulls draft_id / post_id / id / data.id from a CLI response.
func firstID(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	if data, ok := m["data"].(map[string]any); ok {
		if v, ok := data["id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Publish creates a Page draft (and publishes it unless draft-only).
// Expected CLI/API failures come back as PublishResult{Ok:false}.
func (p *FacebookPublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	if !p.IsConfigured() {
		return PublishResult{Ok: false, Platform: p.Name(), Error: "facebook page not configured"}
	}
	// Mirror Python's f"{title}\n\n{description}".strip()[:2000].
	text := truncate(strings.TrimSpace(title+"\n\n"+description), 2000)

	draft := p.run("pages", "drafts", "create",
		"--page-id", p.pageID,
		"--text", text,
		"--request-id", newRequestID(),
		"--file", videoPath)
	draftID := firstID(draft, "draft_id", "id")
	if draftID == "" {
		return PublishResult{Ok: false, Platform: p.Name(), Draft: true,
			Error: truncate(fmt.Sprintf("draft create failed: %v", draft), 300)}
	}
	if p.draftOnly {
		return PublishResult{Ok: true, Platform: p.Name(), RemoteID: draftID, Draft: true}
	}
	pub := p.run("pages", "drafts", "publish",
		"--page-id", p.pageID,
		"--draft-id", draftID,
		"--request-id", newRequestID(),
		"--privacy", "PUBLIC")
	postID := firstID(pub, "post_id", "id")
	if postID == "" {
		postID = draftID
	}
	if _, hasErr := pub["error"]; hasErr {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: truncate(fmt.Sprintf("draft publish failed: %v", pub), 300)}
	}
	if code, ok := pub["_exit"]; ok {
		if n, ok := asInt(code); ok && n != 0 {
			return PublishResult{Ok: false, Platform: p.Name(),
				Error: truncate(fmt.Sprintf("draft publish failed: %v", pub), 300)}
		}
	}
	return PublishResult{Ok: true, Platform: p.Name(), RemoteID: postID}
}

// asInt converts JSON-decoded numbers to int.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
