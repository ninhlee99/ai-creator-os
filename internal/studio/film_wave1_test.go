package studio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Film Wave 1 acceptance tests: Veo clamp, TTS chunking, resume, startup sweep.

// P0-7: Veo 3 renders at most ~8s per call — requests must be clamped.
func TestClampVeoSeconds(t *testing.T) {
	cases := []struct{ in, want int }{
		{25, 8}, {12, 8}, {9, 8}, {8, 8}, {5, 5}, {4, 4}, {3, 8}, {0, 8}, {-5, 8},
	}
	for _, c := range cases {
		if got := clampVeoSeconds(c.in); got != c.want {
			t.Errorf("clampVeoSeconds(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// P0-6: long narration splits into sentence-boundary chunks ≤800 chars,
// in order.
func TestSplitTextChunks(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("Câu thứ " + strings.Repeat("x", 60) + " của đoạn hội thoại dài. ")
	}
	chunks := splitTextChunks(sb.String(), maxTTSChunkChars)
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if utf8.RuneCountInString(c) > maxTTSChunkChars {
			t.Errorf("chunk %d too long: %d runes", i, utf8.RuneCountInString(c))
		}
	}
	if !strings.HasPrefix(chunks[0], "Câu thứ") {
		t.Errorf("order broken: first chunk = %q", chunks[0][:40])
	}
	// Short text stays one chunk.
	if got := splitTextChunks("Xin chào bạn.", 800); len(got) != 1 {
		t.Errorf("short text: got %d chunks, want 1", len(got))
	}
	// Empty/blank text gives nothing.
	if got := splitTextChunks("   ", 800); len(got) != 0 {
		t.Errorf("blank text: got %d chunks, want 0", len(got))
	}
}

// P0-3: a scene counts as resumable only when its asset is done AND the
// file still exists on disk.
func TestResumeAssetPath(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.insertJob(KindFilm, "T", FilmParams{Topic: "x"})
	if err != nil {
		t.Fatal(err)
	}
	mp4 := filepath.Join(dir, "scene00.mp4")
	if err := os.WriteFile(mp4, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	aid := st.addAsset(id, 0, "clip", "p")
	st.setAsset(aid, StatusDone, mp4)
	if got := st.resumeAssetPath(id, 0, "clip"); got != mp4 {
		t.Fatalf("resume path = %q, want %q", got, mp4)
	}
	// File deleted after the fact -> must NOT resume.
	if err := os.Remove(mp4); err != nil {
		t.Fatal(err)
	}
	if got := st.resumeAssetPath(id, 0, "clip"); got != "" {
		t.Fatalf("resume path with missing file = %q, want empty", got)
	}
	// Failed asset -> never resume.
	aid2 := st.addAsset(id, 1, "clip", "p")
	st.setAsset(aid2, StatusFailed, "")
	if got := st.resumeAssetPath(id, 1, "clip"); got != "" {
		t.Fatalf("resume path for failed asset = %q, want empty", got)
	}
}

// P0-3: startup sweep fails jobs stuck "running" and leaves others alone.
func TestMarkInterruptedJobs(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idRun, _ := st.insertJob(KindFilm, "Run", FilmParams{Topic: "x"})
	idDone, _ := st.insertJob(KindFilm, "Done", FilmParams{Topic: "x"})
	st.setStatus(idRun, StatusRunning, 50)
	st.setStatus(idDone, StatusDone, 100)
	if n := st.MarkInterruptedJobs(); n != 1 {
		t.Fatalf("swept %d jobs, want 1", n)
	}
	j, _ := st.GetJob(idRun)
	if j.Status != StatusFailed {
		t.Errorf("running job status = %q, want failed", j.Status)
	}
	if !strings.Contains(j.Log, "Gián đoạn khi khởi động lại") {
		t.Errorf("running job log missing interrupt note: %q", j.Log)
	}
	j, _ = st.GetJob(idDone)
	if j.Status != StatusDone {
		t.Errorf("done job status = %q, want done", j.Status)
	}
	// Idempotent: second sweep finds nothing.
	if n := st.MarkInterruptedJobs(); n != 0 {
		t.Errorf("second sweep = %d, want 0", n)
	}
}

// P0-3: RerunFilmJob validates, then re-runs the job from stored params.
func TestRerunFilmJob(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RerunFilmJob("nope"); err == nil {
		t.Error("want error for missing job, got nil")
	}
	idAff, _ := st.insertJob(KindAffiliate, "A", AffiliateParams{Mode: "photo"})
	if err := st.RerunFilmJob(idAff); err == nil {
		t.Error("want error for non-film job, got nil")
	}
	idFilm, _ := st.insertJob(KindFilm, "F", FilmParams{Topic: "x", Seconds: 90})
	st.setStatus(idFilm, StatusFailed, 100)
	if err := st.RerunFilmJob(idFilm); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	// The goroutine runs runFilm with nil LLM -> fails fast with "thiếu LLM".
	deadline := time.Now().Add(10 * time.Second)
	for {
		j, _ := st.GetJob(idFilm)
		if j.Status == StatusFailed && strings.Contains(j.Log, "thiếu LLM") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never re-ran: status=%q log=%q", j.Status, j.Log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Rerunning while already running is refused.
	idRun2, _ := st.insertJob(KindFilm, "R2", FilmParams{Topic: "x"})
	st.mu.Lock()
	st.running[idRun2] = func() {}
	st.mu.Unlock()
	if err := st.RerunFilmJob(idRun2); err == nil {
		t.Error("want error when job already running, got nil")
	}
	st.mu.Lock()
	delete(st.running, idRun2)
	st.mu.Unlock()
}

// P0-7: the render method is stored per asset and surfaced in the list.
func TestAssetMethodRoundtrip(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, _ := st.insertJob(KindFilm, "T", FilmParams{Topic: "x"})
	aid := st.addAsset(id, 0, "clip", "p")
	st.setAssetMethod(aid, "veo")
	assets := st.ListAssets(id)
	if len(assets) != 1 || assets[0].Method != "veo" {
		t.Fatalf("method roundtrip wrong: %+v", assets)
	}
}
