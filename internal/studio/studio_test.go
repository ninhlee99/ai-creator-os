package studio

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestParseImageResponse(t *testing.T) {
	img := []byte{0x89, 0x50, 0x4e, 0x47} // PNG magic
	data := []byte(`{"candidates":[{"content":{"parts":[` +
		`{"text":"here is your image"},` +
		`{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString(img) + `"}}` +
		`]}}]}`)
	got, err := parseImageResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(img) || got[0] != img[0] {
		t.Fatalf("image bytes wrong")
	}
}

func TestParseImageResponseNoImage(t *testing.T) {
	_, err := parseImageResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"no image"}]}}]}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestParseImageResponseAPIError(t *testing.T) {
	_, err := parseImageResponse([]byte(`{"error":{"message":"bad key","status":"INVALID_ARGUMENT"}}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestStudioJobRoundtrip(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.insertJob(KindAffiliate, "Test", AffiliateParams{Mode: "photo"})
	if err != nil {
		t.Fatal(err)
	}
	j, ok := st.GetJob(id)
	if !ok || j.Title != "Test" || j.Status != StatusQueued {
		t.Fatalf("get job wrong: %+v %v", j, ok)
	}
	st.setStatus(id, StatusRunning, 42)
	st.appendLog(id, "hello")
	j, _ = st.GetJob(id)
	if j.Progress != 42 || j.Log == "" {
		t.Fatalf("status/log wrong: %+v", j)
	}
	aid := st.addAsset(id, 0, "photo", "prompt")
	st.setAsset(aid, StatusDone, "/tmp/x.png")
	assets := st.ListAssets(id)
	if len(assets) != 1 || assets[0].Status != StatusDone {
		t.Fatalf("assets wrong: %+v", assets)
	}
	jobs := st.ListJobs(10)
	if len(jobs) != 1 {
		t.Fatalf("list wrong: %d", len(jobs))
	}
}

func TestGeminiMediaGenNoKeys(t *testing.T) {
	g := NewGeminiMediaGen(nil)
	if g.Healthy(context.Background()) {
		t.Fatal("want unhealthy with no keys")
	}
	if _, _, ok := g.pickKey(); ok {
		t.Fatal("want no key")
	}
	err := g.GenerateImage(context.Background(), "x", nil, filepath.Join(t.TempDir(), "o.png"))
	if err == nil {
		t.Fatal("want error with no keys")
	}
}

func TestProbeDurationMissing(t *testing.T) {
	if d := probeDuration(context.Background(), "/nonexistent/file.mp4"); d != 0 {
		t.Fatalf("want 0, got %f", d)
	}
}

func TestUploadDir(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "s.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := os.Stat(st.UploadDir()); err != nil {
		t.Fatalf("upload dir missing: %v", err)
	}
}
