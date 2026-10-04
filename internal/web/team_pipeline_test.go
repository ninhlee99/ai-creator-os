package web

import (
	"net/http"
	"strings"
	"testing"
)

// Đợt H2: trang /team định nghĩa lại quanh 3 pipeline.
// Trang phải render 200 ngay cả khi mọi store đều nil (fail-closed),
// và phải ghi rõ trạng thái suy ra từ dữ liệu thật.
func TestTeamPipelines(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/team")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /team = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Team Affiliate", "Team Reup", "Team Kể chuyện",
		"không phải agent đang chạy"} {
		if !strings.Contains(body, want) {
			t.Errorf("team page missing %q", want)
		}
	}
	// Mọi store nil → các chặng ở trạng thái idle trung thực, không panic.
	for _, want := range []string{"Hunter", "Producer", "Publisher", "Analyst", "Director", "QC", "Writer"} {
		if !strings.Contains(body, want) {
			t.Errorf("team page missing stage %q", want)
		}
	}
}

// storyStageFor: ánh xạ tiến độ job → chặng (ước lượng).
func TestStoryStageFor(t *testing.T) {
	cases := []struct {
		progress int
		want     int
	}{
		{0, 0}, {19, 0}, {20, 1}, {44, 1}, {45, 2}, {64, 2},
		{65, 3}, {89, 3}, {90, 4}, {100, 4},
	}
	for _, c := range cases {
		if got := storyStageFor(&studioJobLite{Progress: c.progress}); got != c.want {
			t.Errorf("progress %d → stage %d, want %d", c.progress, got, c.want)
		}
	}
}
