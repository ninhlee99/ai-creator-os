package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func TestFacebookLoginButtonRequiresMetaAppCredentials(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("creator.one", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("META_APP_ID", "")
	t.Setenv("META_APP_SECRET", "")
	body := get(t, s, "/publishers").Body.String()
	if strings.Contains(body, "/publishers/facebook/authorize") {
		t.Fatal("Facebook OAuth button appeared without app credentials")
	}
	t.Setenv("META_APP_ID", "meta-app-id")
	t.Setenv("META_APP_SECRET", "meta-app-secret")
	body = get(t, s, "/publishers").Body.String()
	if !strings.Contains(body, "/publishers/facebook/authorize?username=creator.one") {
		t.Fatal("Facebook OAuth button missing with app ID and secret")
	}
	if strings.Contains(body, "meta-app-secret") {
		t.Fatal("Meta app secret leaked into page")
	}
}

func TestStudioAIAndFacebookSelectionRoutesRender(t *testing.T) {
	s := newTestServer(t)
	studioPage := get(t, s, "/studio/create")
	if studioPage.Code != http.StatusOK || !strings.Contains(studioPage.Body.String(), "Tạo nội dung AI") {
		t.Fatalf("studio create route = %d, expected rendered page", studioPage.Code)
	}
	facebookPage := httptest.NewRecorder()
	s.render(facebookPage, "facebook_pages", s.ctx(
		"Username", "creator.one",
		"State", "test-state",
		"Pages", []facebookPageOption{{ID: "123456789", Name: "Creator Page"}},
	))
	if facebookPage.Code != http.StatusOK || !strings.Contains(facebookPage.Body.String(), "Creator Page") {
		t.Fatalf("Facebook selection page = %d, expected rendered page", facebookPage.Code)
	}
}

func TestFacebookOAuthAuthorizeAndPerAccountPageSave(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("creator.one", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("META_APP_ID", "meta-app-id")
	t.Setenv("META_APP_SECRET", "meta-app-secret")
	requestRecorder := httptest.NewRecorder()
	requestInput := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/publishers/facebook/authorize?username=creator.one", nil)
	s.Routes().ServeHTTP(requestRecorder, requestInput)
	request := requestRecorder
	if request.Code != http.StatusFound {
		t.Fatalf("authorize = %d, want redirect", request.Code)
	}
	location, err := url.Parse(request.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if location.Host != "www.facebook.com" || query.Get("client_id") != "meta-app-id" ||
		query.Get("state") == "" || query.Get("redirect_uri") != "http://127.0.0.1:8080/publishers/facebook/callback" ||
		!strings.Contains(query.Get("scope"), "pages_manage_posts") {
		t.Fatalf("bad Facebook authorize URL: %s", location)
	}
	page := facebookOAuthPage{facebookPageOption: facebookPageOption{ID: "123456789", Name: "Creator Page"}, AccessToken: "page-token-secret"}
	if err := s.saveFacebookPage("creator.one", page); err != nil {
		t.Fatal(err)
	}
	if !publishers.FacebookPageConfigured("creator.one") {
		t.Fatal("saved page credentials not visible to publisher")
	}
	for _, name := range []string{"FB_PAGE_ID_CREATOR_ONE", "META_PAGE_TOKEN_CREATOR_ONE"} {
		t.Cleanup(func() { _ = os.Unsetenv(name) })
	}
}

func TestAccountNewLoginButtonsFollowClientCredentials(t *testing.T) {
	s := newTestServer(t)
	t.Setenv("YOUTUBE_CLIENT_ID", "")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "")
	t.Setenv("TIKTOK_CLIENT_KEY", "")
	t.Setenv("TIKTOK_CLIENT_SECRET", "")
	t.Setenv("META_APP_ID", "")
	t.Setenv("META_APP_SECRET", "")
	body := get(t, s, "/accounts/new").Body.String()
	for _, route := range []string{"/publishers/youtube/authorize", "/publishers/tiktok/authorize", "/publishers/facebook/authorize"} {
		if strings.Contains(body, "href=\""+route) {
			t.Fatalf("OAuth login button %s appeared without client credentials", route)
		}
	}
	t.Setenv("YOUTUBE_CLIENT_ID", "youtube-id")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "youtube-secret")
	t.Setenv("TIKTOK_CLIENT_KEY", "tiktok-key")
	t.Setenv("TIKTOK_CLIENT_SECRET", "tiktok-secret")
	t.Setenv("META_APP_ID", "meta-id")
	t.Setenv("META_APP_SECRET", "meta-secret")
	body = get(t, s, "/accounts/new").Body.String()
	for _, route := range []string{"/publishers/youtube/authorize", "/publishers/tiktok/authorize", "/publishers/facebook/authorize"} {
		if !strings.Contains(body, "href=\""+route) {
			t.Fatalf("OAuth login button %s missing after credentials configured", route)
		}
	}
	for _, secret := range []string{"youtube-secret", "tiktok-secret", "meta-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("client secret leaked into account creation page: %s", secret)
		}
	}
}

func TestFacebookOAuthCanCreateChannelFromPage(t *testing.T) {
	s := newTestServer(t)
	page := facebookOAuthPage{facebookPageOption: facebookPageOption{ID: "123456789", Name: "Creator Page"}, AccessToken: "page-token-secret"}
	username, accountID, err := s.connectFacebookPage("__new_facebook__", page)
	if err != nil {
		t.Fatal(err)
	}
	if username != "facebook_123456789" || accountID == 0 || !s.accountExists(username) {
		t.Fatalf("new Facebook channel = %q/%d, want created account", username, accountID)
	}
	if !publishers.FacebookPageConfigured(username) {
		t.Fatal("created Facebook channel does not have saved page credentials")
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(accountEnvName("FB_PAGE_ID", username))
		_ = os.Unsetenv(accountEnvName("META_PAGE_TOKEN", username))
	})
}
