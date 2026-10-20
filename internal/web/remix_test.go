package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestReorderPhotos(t *testing.T) {
	p := studio.AffiliateParams{ProductPhoto: "a", ExtraPhotos: []string{"b", "c"}, ModelPhoto: "m"}
	if r := reorderPhotos(p, "reverse"); r.ProductPhoto != "c" || r.ExtraPhotos[1] != "a" || r.ModelPhoto != "m" {
		t.Fatalf("reverse = %+v", r)
	}
	if r := reorderPhotos(p, "rotate"); r.ProductPhoto != "b" || r.ExtraPhotos[1] != "a" {
		t.Fatalf("rotate = %+v", r)
	}
	if r := reorderPhotos(p, ""); r.ProductPhoto != "a" {
		t.Fatal("trống = giữ nguyên")
	}
}

// Dựng lại tạo job MỚI với tham số đã sửa, job cũ giữ nguyên.
func TestStudioRemixCreatesEditedJob(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	oldID, err := st.CreateAffiliateJob(studio.AffiliateParams{ProductName: "Túi", Seconds: 30,
		ProductPhoto: "a.jpg", ExtraPhotos: []string{"b.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("seconds", "45")
	_ = mw.WriteField("order", "reverse")
	_ = mw.WriteField("caption", "Chú thích mới #tui")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/studio/jobs/"+oldID+"/remix", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("remix = %d", rec.Code)
	}
	jobs := st.ListJobs(10)
	if len(jobs) != 2 {
		t.Fatalf("phải có 2 job (cũ + bản sửa), got %d", len(jobs))
	}
	var newJob studio.Job
	for _, j := range jobs {
		if j.ID != oldID {
			newJob = j
		}
	}
	full, _ := st.GetJob(newJob.ID)
	var p studio.AffiliateParams
	_ = json.Unmarshal([]byte(full.Params), &p)
	if p.Seconds != 45 || p.ProductPhoto != "b.jpg" || p.FixedCaption != "Chú thích mới #tui" {
		t.Fatalf("params bản sửa sai: %+v", p)
	}
}
