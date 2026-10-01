package studio

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

const kworbFixture = `<table><thead><tr><th>Pos</th><th>P+</th><th class="mp text">Artist - Title</th></thead><tbody>
<tr><td>1</td><td>=</td><td class="mp text"><div>Dunghoangpham - Mật Ngọt - Nam Con Remix</div></td></tr>
<tr><td>2</td><td>+19</td><td class="mp text"><div>Vũ. - Vì Anh Đâu Có Biết</div></td></tr>
<tr><td>3</td><td>-1</td><td class="mp text"><div>Không Dấu Gạch Nối</div></td></tr>
</tbody></table>`

func TestParseKworbVN(t *testing.T) {
	sounds := parseKworbVN(kworbFixture)
	if len(sounds) != 3 {
		t.Fatalf("got %d sounds, want 3", len(sounds))
	}
	if sounds[0].Rank != 1 || sounds[0].Artist != "Dunghoangpham" || sounds[0].Title != "Mật Ngọt - Nam Con Remix" {
		t.Fatalf("row 1 wrong: %+v", sounds[0])
	}
	if sounds[1].Movement != "+19" {
		t.Fatalf("row 2 movement wrong: %+v", sounds[1])
	}
	if sounds[2].Artist != "" || sounds[2].Title != "Không Dấu Gạch Nối" {
		t.Fatalf("row 3 wrong: %+v", sounds[2])
	}
}

func TestParseKworbVNEmpty(t *testing.T) {
	if got := parseKworbVN("<html></html>"); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

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
	if d := ProbeDuration(context.Background(), "/nonexistent/file.mp4"); d != 0 {
		t.Fatalf("want 0, got %f", d)
	}
}

func TestDownloadAudioBadURL(t *testing.T) {
	err := DownloadAudio(context.Background(), "http://127.0.0.1:1/nope.mp3", filepath.Join(t.TempDir(), "a.mp3"))
	if err == nil {
		t.Fatal("want error")
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
