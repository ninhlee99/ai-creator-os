package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func TestTikTokCookieSaveAndDelete(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	store, err := webai.NewStore(dir + "/webai.db")
	if err != nil {
		t.Fatalf("webai.NewStore: %v", err)
	}
	s.WebAI = store

	// 1. Save valid TikTok cookie
	cookieVal := "sessionid=test_tiktok_session_123456; ttwid=test_ttwid; msToken=test_mstoken"
	form := url.Values{"cookie": {cookieVal}}
	req := httptest.NewRequest(http.MethodPost, "/settings/tiktok/cookie/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleTikTokCookieSave(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect StatusSeeOther, got %d", rec.Code)
	}

	// Verify account saved in WebAI store
	accs, err := s.WebAI.GetAccounts(webai.PlatformTikTok)
	if err != nil || len(accs) == 0 {
		t.Fatalf("expected TikTok account saved, got err=%v, count=%d", err, len(accs))
	}
	if !strings.Contains(accs[0].Cookie, "sessionid=test_tiktok_session_123456") {
		t.Errorf("cookie does not contain sessionid: %s", accs[0].Cookie)
	}

	// 2. Render settings page and ensure cookie card displays active status
	reqPage := httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	recPage := httptest.NewRecorder()
	s.handleSettingsHeThong(recPage, reqPage)
	if recPage.Code != http.StatusOK {
		t.Fatalf("settings page returned %d", recPage.Code)
	}
	body := recPage.Body.String()
	if !strings.Contains(body, "Đã kết nối Cookie") {
		t.Errorf("expected 'Đã kết nối Cookie' in rendered HTML")
	}

	// 3. Delete TikTok cookie
	reqDel := httptest.NewRequest(http.MethodPost, "/settings/tiktok/cookie/delete", nil)
	recDel := httptest.NewRecorder()
	s.handleTikTokCookieDelete(recDel, reqDel)
	if recDel.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect StatusSeeOther, got %d", recDel.Code)
	}

	accsAfter, _ := s.WebAI.GetAccounts(webai.PlatformTikTok)
	if len(accsAfter) != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", len(accsAfter))
	}
}
