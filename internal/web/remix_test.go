package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	_ "modernc.org/sqlite"
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

// Legacy remix without user-supplied motion clips must not queue another
// job that would call a paid video API or fail asynchronously.
func TestStudioRemixRequiresMotionClips(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "studio.db")
	st, err := studio.New(dbPath, nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	params, err := json.Marshal(studio.AffiliateParams{Mode: studio.AffiliateModeShots, ProductName: "Túi", Seconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		"legacy-affiliate", studio.KindAffiliate, "Túi", studio.StatusDone, string(params), time.Now().Format("2006-01-02T15:04:05"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("seconds", "45")
	_ = mw.WriteField("order", "reverse")
	_ = mw.WriteField("caption", "Chú thích mới #tui")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/studio/jobs/legacy-affiliate/remix", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("remix = %d", rec.Code)
	}
	jobs := st.ListJobs(10)
	if len(jobs) != 1 || jobs[0].ID != "legacy-affiliate" {
		t.Fatalf("remix without clips queued a job: %+v", jobs)
	}
}
