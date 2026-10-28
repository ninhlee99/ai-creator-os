package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestStudioTikTokDraftRequiresPerVideoConsent(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "/studio/jobs/job-1/tiktok-draft", strings.NewReader(""))
	r.SetPathValue("id", "job-1")
	w := httptest.NewRecorder()

	s.handleStudioTikTokDraft(w, r)

	if w.Code != 303 {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("redirect location: %v", err)
	}
	if got := location.Query().Get("err"); !strings.Contains(got, "xem trước và đồng ý") {
		t.Fatalf("error = %q, want explicit preview and consent prompt", got)
	}
}
