package studio

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func TestPhotoListFilter(t *testing.T) {
	f, total := photoListFilter(5, 6)
	if total != 5*6-4*0.6 {
		t.Fatalf("total=%v want %v", total, 5*6-4*0.6)
	}
	if got := strings.Count(f, "zoompan"); got != 5 {
		t.Fatalf("want 5 zoompan, got %d", got)
	}
	if got := strings.Count(f, "xfade"); got != 4 {
		t.Fatalf("want 4 xfade, got %d", got)
	}
	if !strings.Contains(f, "[vout]") {
		t.Fatal("missing [vout]")
	}
	// first xfade offset = 6 - 0.6 = 5.40
	if !strings.Contains(f, "offset=5.40") {
		t.Fatalf("bad first offset in %s", f)
	}
	f1, total1 := photoListFilter(1, 6)
	if total1 != 6 || strings.Count(f1, "xfade") != 0 || !strings.Contains(f1, "[vout]") {
		t.Fatalf("single photo: %v %s", total1, f1)
	}
}

func TestModelStoreRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ms, err := NewModelStore(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "model.png")
	if err := os.WriteFile(src, []byte("fakepng"), 0o644); err != nil {
		t.Fatal(err)
	}
	mp, err := ms.Add(7, src)
	if err != nil {
		t.Fatal(err)
	}
	if mp.AccountID != 7 || mp.Path == "" {
		t.Fatalf("bad photo: %+v", mp)
	}
	ok, err := ms.HasPhotos(7)
	if err != nil || !ok {
		t.Fatalf("HasPhotos: %v %v", ok, err)
	}
	list, err := ms.List(7)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v %d", err, len(list))
	}
	if err := ms.Delete(mp.ID); err != nil {
		t.Fatal(err)
	}
	ok, _ = ms.HasPhotos(7)
	if ok {
		t.Fatal("photo should be gone")
	}
	if _, err := os.Stat(mp.Path); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}
}

// stubProvider feeds autopilot without network.
type stubProvider struct{ ps []products.Product }

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) Search(ctx context.Context, q products.Query) ([]products.Product, error) {
	return s.ps, nil
}

func testPNGServer(t *testing.T) string {
	t.Helper()
	// 1x1 PNG
	png := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82,
		0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0, 144, 119, 83, 222}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/p.png"
}

func TestAutopilotRun(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.New(filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	mgr, err := network.NewAccountManager(l, filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	acct, err := mgr.Add("acc1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetTheme(acct.ID, "thoi-trang-nu"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetAutopilot(acct.ID, true, 0.10); err != nil {
		t.Fatal(err)
	}

	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	models, err := st.ModelLibrary()
	if err != nil {
		t.Fatal(err)
	}

	// no model photo yet -> clear error asking Ninh to upload
	pstore, err := products.NewStore(filepath.Join(dir, "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer pstore.Close()
	ap := NewAutopilot(st, mgr, pstore, []products.Provider{&stubProvider{}}, models)
	if _, err := ap.Run(context.Background(), acct.ID); err == nil ||
		!strings.Contains(err.Error(), "ảnh mẫu") {
		t.Fatalf("want model-photo error, got %v", err)
	}

	// add model photo
	src := filepath.Join(dir, "model.png")
	_ = os.WriteFile(src, []byte("png"), 0o644)
	if _, err := models.Add(acct.ID, src); err != nil {
		t.Fatal(err)
	}

	// seed a high-commission product in the store
	imgURL := testPNGServer(t)
	pid, err := pstore.Save(products.Product{
		Source: "stub", SourceID: "p1", Title: "Túi kem",
		ImageURLs: []string{imgURL}, CommissionRate: 0.25,
		Theme: "thoi-trang-nu",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := ap.Run(context.Background(), acct.ID)
	if err != nil {
		t.Fatalf("autopilot run: %v", err)
	}
	if res.JobID == "" || res.Product.Title != "Túi kem" {
		t.Fatalf("bad result: %+v", res)
	}
	// job was queued (it will fail async without media-gen — that's fine)
	j, ok := st.GetJob(res.JobID)
	if !ok || j.Kind != KindAffiliate {
		t.Fatalf("job missing: %+v", j)
	}
	// product marked used for this account
	top, err := pstore.TopByTheme("thoi-trang-nu", 0.10, acct.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range top {
		if p.ID == pid {
			t.Fatal("used product should be excluded for this account")
		}
	}
}

func TestAutopilotNotReady(t *testing.T) {
	dir := t.TempDir()
	l, _ := ledger.New(filepath.Join(dir, "ledger.db"))
	defer l.Close()
	mgr, _ := network.NewAccountManager(l, filepath.Join(dir, "ledger.db"))
	defer mgr.Close()
	acct, _ := mgr.Add("acc2", "", "")
	st, _ := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	defer st.Close()
	models, _ := st.ModelLibrary()
	pstore, _ := products.NewStore(filepath.Join(dir, "products.db"))
	defer pstore.Close()
	ap := NewAutopilot(st, mgr, pstore, nil, models)
	if _, err := ap.Run(context.Background(), acct.ID); err == nil ||
		!strings.Contains(err.Error(), "chưa sẵn sàng") {
		t.Fatalf("want not-ready error, got %v", err)
	}
	fmt.Println("ok")
}
