package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetiredTrendPresetRoutesAreGone(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/studio/tiktok-topics"},
		{http.MethodPost, "/viral/quick"},
		{http.MethodPost, "/viral/clips/import"},
		{http.MethodGet, "/clips/viral/old.mp4"},
		{http.MethodPost, "/studio/viral"},
		{http.MethodPost, "/studio/viral/quick"},
		{http.MethodPost, "/team/agents/create"},
		{http.MethodPost, "/team/agents/toggle"},
		{http.MethodPost, "/team/agents/delete"},
		{http.MethodPost, "/team/departments/assign"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		serveLocalTestRoute(s, rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 404 or 405", tc.method, tc.path, rec.Code)
		}
	}
}
