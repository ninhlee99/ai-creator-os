package studio

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Affiliate jobs need an approved shot plan before they can be queued.
func TestAffiliateRequiresPreparedShotPlanBeforeQueue(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	photo := makeTestPNG(t, dir, "p.png", "testsrc")
	_, err = st.CreateAffiliateJob(AffiliateParams{
		ProductName: "Túi da", Niche: "thời trang/phụ kiện", ProductPhoto: photo, Seconds: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "shot plan") {
		t.Fatalf("CreateAffiliateJob error = %v, want prepared shot plan requirement", err)
	}
	if got := st.ListJobs(10); len(got) != 0 {
		t.Fatalf("queued %d jobs without motion footage", len(got))
	}
}

func TestTrendReadiness(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	photo := makeTestPNG(t, dir, "p.png", "testsrc")
	out := filepath.Join(dir, "v.mp4")
	if err := AssembleStillPhotoList(t.Context(), []string{photo, photo, photo}, 3.4, "", 0, out, "9:16"); err != nil {
		t.Fatal(err)
	}
	r := TrendReadiness(t.Context(), out, AffiliateParams{}, "Đẹp quá")
	if r.Total != 6 || r.Score != 2 { // 9:16 + độ dài 9s; câm, không hashtag/CTA/link
		t.Fatalf("score %d/%d: %+v", r.Score, r.Total, r.Checks)
	}
	r = TrendReadiness(t.Context(), out, AffiliateParams{AffLink: "https://a"}, "Xem link mua #tui")
	if r.Score != 5 {
		t.Fatalf("score %d: %+v", r.Score, r.Checks)
	}
}
