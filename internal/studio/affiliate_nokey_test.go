package studio

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Không có media-gen + có ảnh sản phẩm → vẫn ra video 1080x1920, không chữ.
func TestAffiliateOwnPhotosWithoutKey(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	photo := makeTestPNG(t, dir, "p.png", "testsrc")
	id, err := st.CreateAffiliateJob(AffiliateParams{
		ProductName: "Túi da", Niche: "thời trang/phụ kiện", ProductPhoto: photo, Seconds: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	var j Job
	for i := 0; i < 240; i++ {
		j, _ = st.GetJob(id)
		if j.Status == StatusDone || j.Status == StatusFailed {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if j.Status != StatusDone {
		t.Fatalf("status=%s log=%s", j.Status, j.Log)
	}
	if w, h := ProbeDims(t.Context(), filepath.Join(dir, "out", j.Output)); w != 1080 || h != 1920 {
		t.Fatalf("khổ %dx%d", w, h)
	}
	if !strings.Contains(j.Log, "ảnh có sẵn") || !strings.Contains(j.Caption, "#thoitrang") {
		t.Fatalf("log/caption sai: %q / %q", j.Log, j.Caption)
	}
	// Xuất gói CapCut: video hoàn chỉnh + ảnh từng cảnh + caption.
	dst, err := st.ExportForEditing(id, filepath.Join(dir, "export"))
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dst, "*"))
	var hasVideo, hasCaption bool
	photos := 0
	for _, f := range files {
		b := filepath.Base(f)
		switch {
		case strings.HasPrefix(b, "00-video-hoan-chinh"):
			hasVideo = true
		case b == "caption.txt":
			hasCaption = true
		case strings.HasSuffix(b, "-photo.png"):
			photos++
		}
	}
	if !hasVideo || !hasCaption || photos < 3 {
		t.Fatalf("gói CapCut thiếu file: %v", files)
	}
}

// Không key + không ảnh → thất bại với hướng dẫn rõ ràng.
func TestAffiliateNoKeyNoPhotoExplains(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, _ := st.CreateAffiliateJob(AffiliateParams{ProductName: "X"})
	var j Job
	for i := 0; i < 40; i++ {
		j, _ = st.GetJob(id)
		if j.Status == StatusFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j.Status != StatusFailed || !strings.Contains(j.Log, "tải lên ít nhất 1 ảnh") {
		t.Fatalf("status=%s log=%s", j.Status, j.Log)
	}
}

func TestTrendReadiness(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	photo := makeTestPNG(t, dir, "p.png", "testsrc")
	out := filepath.Join(dir, "v.mp4")
	if err := AssembleBeatBounce(t.Context(), []string{photo, photo, photo}, 3, 120, "", 0, out, "9:16"); err != nil {
		t.Fatal(err)
	}
	r := TrendReadiness(t.Context(), out, AffiliateParams{}, "Đẹp quá")
	if r.Total != 7 || r.Score != 2 { // 9:16 + độ dài 9s; câm, không nhạc/hashtag/CTA/link
		t.Fatalf("score %d/%d: %+v", r.Score, r.Total, r.Checks)
	}
	r = TrendReadiness(t.Context(), out, AffiliateParams{MusicTitle: "X", MusicPath: "m.m4a", AffLink: "https://a"}, "Xem link mua #tui")
	if r.Score != 6 {
		t.Fatalf("score %d: %+v", r.Score, r.Checks)
	}
}
