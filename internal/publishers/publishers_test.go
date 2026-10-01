package publishers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// envFor: per-account NAME_<USERNAME> wins over shared NAME.
func TestEnvForPrefersPerAccount(t *testing.T) {
	t.Setenv("WIDGET_ALICE", "per-account")
	t.Setenv("WIDGET", "shared")
	if got := envFor("alice", "WIDGET"); got != "per-account" {
		t.Fatalf("got %q", got)
	}
	if got := envFor("bob", "WIDGET"); got != "shared" {
		t.Fatalf("fallback got %q", got)
	}
	os.Unsetenv("WIDGET_ALICE")
	if got := envFor("alice", "WIDGET"); got != "shared" {
		t.Fatalf("after unset got %q", got)
	}
	os.Unsetenv("WIDGET")
	if got := envFor("alice", "WIDGET"); got != "" {
		t.Fatalf("empty got %q", got)
	}
}

func TestEnvForSanitizesUsername(t *testing.T) {
	t.Setenv("THING_USER_NAME_2", "hit")
	if got := envFor("user-name.2", "THING"); got != "hit" {
		t.Fatalf("got %q", got)
	}
}

// BuildPublishers with no credentials configured returns nothing.
func TestBuildPublishersExcludesUnconfigured(t *testing.T) {
	clearPublishEnv(t)
	pubs := BuildPublishers("ghost", "", nil)
	if len(pubs) != 0 {
		names := []string{}
		for _, p := range pubs {
			names = append(names, p.Name())
		}
		t.Fatalf("expected none, got %v", names)
	}
}

// BuildPublishers returns only configured publishers, filtered by kinds.
func TestBuildPublishersConfiguredAndKindFiltered(t *testing.T) {
	clearPublishEnv(t)
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	// Configure TikTok only: client key/secret + token file.
	t.Setenv("TIKTOK_CLIENT_KEY", "k")
	t.Setenv("TIKTOK_CLIENT_SECRET", "s")
	if err := os.WriteFile("tiktok_token_alice.json", []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	pubs := BuildPublishers("alice", "", nil)
	if len(pubs) != 1 || pubs[0].Name() != "tiktok" {
		t.Fatalf("got %v", names(pubs))
	}

	// kinds filter: tiktok handles short_video but not ai_music.
	if got := BuildPublishers("alice", "", nil, "ai_music"); len(got) != 0 {
		t.Fatalf("ai_music should exclude tiktok, got %v", names(got))
	}
	if got := BuildPublishers("alice", "", nil, "short_video"); len(got) != 1 {
		t.Fatalf("short_video should keep tiktok, got %v", names(got))
	}

	// Configure YouTube for ai_music; both appear for ai_music.
	t.Setenv("YOUTUBE_CLIENT_ID", "cid")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "csec")
	if err := os.WriteFile("youtube_token_alice.json", []byte(`{"refresh_token":"r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := BuildPublishers("alice", "UCchan", []string{"ai_music"}, "ai_music")
	if len(got) != 1 || got[0].Name() != "youtube" {
		t.Fatalf("ai_music should yield youtube only, got %v", names(got))
	}
	got = BuildPublishers("alice", "UCchan", []string{"ai_music", "short_film"}, "short_film")
	if len(got) != 2 {
		t.Fatalf("short_film should yield tiktok+youtube, got %v", names(got))
	}
	// YouTube requires non-empty allowed kinds.
	got = BuildPublishers("alice", "UCchan", nil, "ai_music")
	if len(got) != 0 {
		t.Fatalf("empty allowed kinds should exclude youtube, got %v", names(got))
	}
}

// Handles matrices.
func TestHandles(t *testing.T) {
	tp := NewTikTokPublisher("u")
	if !tp.Handles("short_video") || !tp.Handles("short_film") || tp.Handles("ai_music") {
		t.Fatal("tiktok handles matrix wrong")
	}
	fp := NewFacebookPublisher("u")
	if !fp.Handles("short_film") || fp.Handles("ai_remix") {
		t.Fatal("facebook handles matrix wrong")
	}
	yp := NewYouTubePublisher("u", []string{"ai_music", "ai_remix"}, "ch")
	if !yp.Handles("ai_music") || yp.Handles("short_film") {
		t.Fatal("youtube handles matrix wrong")
	}
}

// TikTok draft-first defaults.
func TestTikTokDraftDefaults(t *testing.T) {
	clearPublishEnv(t)
	p := NewTikTokPublisher("u")
	if !p.draftOnly {
		t.Fatal("TIKTOK_DRAFT_ONLY should default to true")
	}
	if p.privacy != "SELF_ONLY" {
		t.Fatalf("privacy = %q", p.privacy)
	}
	t.Setenv("TIKTOK_DRAFT_ONLY", "0")
	t.Setenv("TIKTOK_PRIVACY", "PUBLIC")
	p = NewTikTokPublisher("u")
	if p.draftOnly || p.privacy != "PUBLIC" {
		t.Fatal("env overrides not honored")
	}
}

// Facebook is unconfigured without a page id (and stays so without the CLI).
func TestFacebookNotConfiguredWithoutPage(t *testing.T) {
	clearPublishEnv(t)
	p := NewFacebookPublisher("u")
	if p.IsConfigured() {
		t.Fatal("facebook should not be configured without FB_PAGE_ID")
	}
	if res := p.Publish(context.Background(), "v.mp4", "t", "d", "short_video"); res.Ok || res.Error == "" {
		t.Fatalf("expected failure result, got %+v", res)
	}
}

// firstID pulls draft/post ids from CLI-shaped responses.
func TestFirstID(t *testing.T) {
	if got := firstID(map[string]any{"draft_id": "d1"}, "draft_id", "id"); got != "d1" {
		t.Fatalf("got %q", got)
	}
	if got := firstID(map[string]any{"data": map[string]any{"id": "d2"}}, "draft_id", "id"); got != "d2" {
		t.Fatalf("got %q", got)
	}
	if got := firstID(map[string]any{"post_id": "p3"}, "post_id", "id"); got != "p3" {
		t.Fatalf("got %q", got)
	}
	if got := firstID(map[string]any{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

// YouTube category mapping and privacy default.
func TestYouTubeCategoryAndPrivacy(t *testing.T) {
	clearPublishEnv(t)
	p := NewYouTubePublisher("u", []string{"short_film"}, "ch")
	if p.privacy != "private" {
		t.Fatalf("privacy = %q", p.privacy)
	}
	want := map[string]string{
		"short_film": "24", "ai_music": "10", "ai_remix": "10", "short_video": "22",
	}
	for kind, cat := range want {
		if CategoryByKind[kind] != cat {
			t.Fatalf("category %s = %q", kind, CategoryByKind[kind])
		}
	}
	if res := p.Publish(context.Background(), "v.mp4", "t", "d", "short_video"); res.Ok {
		t.Fatal("unconfigured youtube publish should fail")
	}
}

// YouTube full resumable-upload flow with fake HTTP.
func TestYouTubePublishFlow(t *testing.T) {
	clearPublishEnv(t)
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	t.Setenv("YOUTUBE_CLIENT_ID", "cid")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "csec")
	if err := os.WriteFile("youtube_token_u.json", []byte(`{"refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("v.mp4", make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotRange string
	var gotInitBody map[string]any
	p := NewYouTubePublisher("u", []string{"short_video"}, "ch")
	p.HTTP = func(method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
		switch {
		case method == "POST" && strings.Contains(url, "oauth2.googleapis.com"):
			return 200, nil, []byte(`{"access_token":"ya29.test","expires_in":3600}`), nil
		case method == "POST":
			_ = json.Unmarshal(body, &gotInitBody)
			return 200, map[string]string{"Location": "https://upload.example/sess"}, []byte(`{}`), nil
		case method == "PUT":
			gotRange = headers["Content-Range"]
			return 200, nil, []byte(`{"id":"vidABC"}`), nil
		}
		t.Fatalf("unexpected %s %s", method, url)
		return 0, nil, nil, nil
	}
	res := p.Publish(context.Background(), "v.mp4", "My title", "desc", "short_video")
	if !res.Ok {
		t.Fatalf("publish failed: %+v", res)
	}
	if res.RemoteID != "vidABC" || res.URL != "https://youtu.be/vidABC" {
		t.Fatalf("result = %+v", res)
	}
	if !res.Draft {
		t.Fatal("private upload should be marked draft")
	}
	if gotRange != "bytes 0-2047/2048" {
		t.Fatalf("Content-Range = %q", gotRange)
	}
	snippet := gotInitBody["snippet"].(map[string]any)
	if snippet["categoryId"] != "22" {
		t.Fatalf("category = %v", snippet["categoryId"])
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func clearPublishEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET", "TIKTOK_DRAFT_ONLY", "TIKTOK_PRIVACY",
		"FB_PAGE_ID", "FB_DRAFT_ONLY",
		"YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "YOUTUBE_DEFAULT_PRIVACY",
	} {
		os.Unsetenv(k)
	}
	// Per-account variants could linger from the ambient env; scrub the
	// pattern for the test usernames used here.
	for _, k := range []string{
		"TIKTOK_CLIENT_KEY_ALICE", "TIKTOK_CLIENT_SECRET_ALICE",
		"FB_PAGE_ID_ALICE", "YOUTUBE_CLIENT_ID_ALICE", "YOUTUBE_CLIENT_SECRET_ALICE",
	} {
		os.Unsetenv(k)
	}
}

func names(pubs []Publisher) []string {
	out := make([]string, 0, len(pubs))
	for _, p := range pubs {
		out = append(out, p.Name())
	}
	return out
}
