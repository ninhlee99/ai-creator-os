package webai

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoogleFlowPlatformAndAccount(t *testing.T) {
	// 1. Verify PlatformGoogleFlow is in SupportedPlatforms
	platforms := SupportedPlatforms()
	var foundFlow bool
	var foundMuse bool
	for _, p := range platforms {
		if p.ID == PlatformMuseAI {
			foundMuse = true
			if p.Badge != "Tạo nội dung" {
				t.Errorf("expected Muse content-generation badge, got %s", p.Badge)
			}
			if p.URL != "https://muse.ai" {
				t.Errorf("expected Muse URL https://muse.ai, got %s", p.URL)
			}
		}
		if p.ID == PlatformGoogleFlow {
			foundFlow = true
			if p.URL != "https://flow.google.com/" {
				t.Errorf("expected Flow URL https://flow.google.com/, got %s", p.URL)
			}
			if p.Badge != "Video · web API" {
				t.Errorf("expected Flow web API badge, got %s", p.Badge)
			}
		}
	}
	if !foundFlow {
		t.Fatalf("PlatformGoogleFlow not found in SupportedPlatforms")
	}
	if !foundMuse {
		t.Fatalf("PlatformMuseAI not found in SupportedPlatforms")
	}

	// 2. Test Store Account saving for Google Flow
	dbPath := filepath.Join(t.TempDir(), "webai_flow_test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = store.SaveAccount(Account{
		Platform:    PlatformGoogleFlow,
		AccountName: "Google Flow Main",
		Email:       "ninh.flow@gmail.com",
		SessionKey:  "flow-session-key-xyz",
		Status:      StatusConnected,
		Tier:        "free",
	})
	if err != nil {
		t.Fatalf("SaveAccount for Google Flow failed: %v", err)
	}

	acc, err := store.PickActiveAccount(PlatformGoogleFlow)
	if err != nil {
		t.Fatalf("PickActiveAccount failed: %v", err)
	}
	if acc.Platform != PlatformGoogleFlow {
		t.Errorf("expected platform googleflow, got %s", acc.Platform)
	}
}

func TestMuseCookieJarReturnsRotatedSession(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	museURL, _ := url.Parse(museHTTPBase)
	jar.SetCookies(museURL, parseCookieHeader("hatch_sess=old-session; theme=light"))
	client := &http.Client{Jar: jar}
	gw := &museGateway{client: client}
	if got := gw.cookieHeader(); !strings.Contains(got, "hatch_sess=old-session") || !strings.Contains(got, "theme=light") {
		t.Fatalf("initial Muse cookie header = %q", got)
	}
	jar.SetCookies(museURL, []*http.Cookie{{Name: "hatch_sess", Value: "rotated-session"}})
	if got := gw.cookieHeader(); !strings.Contains(got, "hatch_sess=rotated-session") || strings.Contains(got, "old-session") {
		t.Fatalf("rotated Muse cookie header = %q", got)
	}
}

func TestMuseMediaURLsAllowQueryAndRequireTrustedHost(t *testing.T) {
	got := extractMuseMediaURLs(map[string]any{}, "https://muse.ai/files/a/clip.mp4?download=1 https://muse.ai.evil.test/files/b.mp4")
	if len(got) != 1 || got[0] != "https://muse.ai/files/a/clip.mp4?download=1" {
		t.Fatalf("media links = %#v", got)
	}
}

func TestFlowHeadlessFallbackIsRestrictedToPreSubmitErrors(t *testing.T) {
	preSubmit := &FlowAPIUnavailableError{cause: errors.New("Flow denied API access")}
	if !CanFallbackToGoogleFlowHeadless(fmt.Errorf("preflight: %w", preSubmit)) {
		t.Fatal("pre-submit Flow API failure should allow headless fallback")
	}
	if CanFallbackToGoogleFlowHeadless(errors.New("generation accepted, then history timed out")) {
		t.Fatal("post-submit Flow failure must not retry and risk duplicate generation")
	}
}

func TestFlowMediaURLsAllowSignedFlowCDNOnly(t *testing.T) {
	for _, target := range []struct {
		url  string
		want bool
	}{
		{"https://flow-content.google/video/abc?Signature=secret", true},
		{"https://storage.googleapis.com/video/abc.mp4", true},
		{"https://flow-content.google.evil.test/video/abc", false},
		{"http://flow-content.google/video/abc", false},
	} {
		if got := isGoogleMediaURL(target.url); got != target.want {
			t.Errorf("isGoogleMediaURL(%q) = %t, want %t", target.url, got, target.want)
		}
	}
}

func TestExpiredAccountIsNotSelected(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "webai_expired_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveAccount(Account{Platform: PlatformGoogleFlow, SessionKey: "stale", Cookie: "SID=x", Status: StatusExpired}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PickActiveAccount(PlatformGoogleFlow); err == nil {
		t.Fatal("expected expired Flow account to be skipped")
	}
}

func TestVideoProviderPoolsRotateAccountsIndependently(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "webai_video_pool_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, platform := range []string{PlatformGoogleFlow, PlatformMuseAI} {
		for _, email := range []string{"first@example.com", "second@example.com"} {
			if err := store.SaveAccount(Account{
				Platform:    platform,
				AccountName: email,
				Email:       email,
				SessionKey:  "session-" + email,
				Status:      StatusConnected,
			}); err != nil {
				t.Fatalf("SaveAccount(%s, %s): %v", platform, email, err)
			}
		}
	}

	for _, platform := range []string{PlatformGoogleFlow, PlatformMuseAI} {
		first, err := store.PickActiveAccount(platform)
		if err != nil {
			t.Fatalf("PickActiveAccount(%s) first: %v", platform, err)
		}
		second, err := store.PickActiveAccount(platform)
		if err != nil {
			t.Fatalf("PickActiveAccount(%s) second: %v", platform, err)
		}
		if first.ID == second.ID {
			t.Fatalf("%s pool selected same account twice despite another unused account", platform)
		}
	}
}
