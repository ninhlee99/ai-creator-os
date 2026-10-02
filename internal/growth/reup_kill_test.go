package growth

// Test kill rule 0-view (Đợt E): chỉ đếm bài có số liệu thật; bài "chờ số
// liệu" (views=-1) không tính, không reset chuỗi; chưa có số liệu nào →
// không kill.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mkViews(n int, views int64) []ReupPostView {
	out := make([]ReupPostView, n)
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := range out {
		out[i] = ReupPostView{PostID: int64(i + 1), Views: views, PostedAt: base.Add(time.Duration(i) * time.Hour)}
	}
	return out
}

func TestEvaluateReupZeroViewKill(t *testing.T) {
	posts := mkViews(5, 0)
	kill, reason, evaluated, skipped := EvaluateReupZeroView(posts, 5)
	if !kill {
		t.Fatalf("5 bài 0-view liên tiếp phải kill")
	}
	if evaluated != 5 || skipped != 0 {
		t.Errorf("evaluated=%d skipped=%d, muốn 5/0", evaluated, skipped)
	}
	if reason == "" {
		t.Errorf("phải có lý do tiếng Việt")
	}
}

func TestEvaluateReupZeroViewReset(t *testing.T) {
	posts := mkViews(4, 0)
	posts = append(posts, ReupPostView{PostID: 5, Views: 120, PostedAt: time.Now()})
	posts = append(posts, mkViews(3, 0)...)
	kill, _, evaluated, _ := EvaluateReupZeroView(posts, 5)
	if kill {
		t.Fatalf("chuỗi bị ngắt bởi bài có view — không được kill")
	}
	if evaluated != 8 {
		t.Errorf("evaluated=%d, muốn 8", evaluated)
	}
}

func TestEvaluateReupZeroViewSkipsUnknown(t *testing.T) {
	// 5 bài 0-view + 2 bài "chờ số liệu" xen giữa → vẫn kill (bài chờ
	// số liệu không reset chuỗi, không tính).
	var posts []ReupPostView
	base := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 7; i++ {
		v := int64(0)
		if i == 2 || i == 5 {
			v = -1
		}
		posts = append(posts, ReupPostView{PostID: int64(i + 1), Views: v, PostedAt: base.Add(time.Duration(i) * time.Hour)})
	}
	kill, _, evaluated, skipped := EvaluateReupZeroView(posts, 5)
	if !kill {
		t.Fatalf("5 bài 0-view (2 bài chờ số liệu xen giữa) phải kill")
	}
	if evaluated != 5 || skipped != 2 {
		t.Errorf("evaluated=%d skipped=%d, muốn 5/2", evaluated, skipped)
	}
}

func TestEvaluateReupZeroViewNoData(t *testing.T) {
	posts := mkViews(3, -1)
	kill, _, evaluated, skipped := EvaluateReupZeroView(posts, 5)
	if kill {
		t.Fatalf("chưa có số liệu nào — không được kill (fail-closed)")
	}
	if evaluated != 0 || skipped != 3 {
		t.Errorf("evaluated=%d skipped=%d, muốn 0/3", evaluated, skipped)
	}
}

func TestEvaluateReupZeroViewCustomN(t *testing.T) {
	posts := mkViews(3, 0)
	if kill, _, _, _ := EvaluateReupZeroView(posts, 3); !kill {
		t.Errorf("ngưỡng 3 với 3 bài 0-view phải kill")
	}
	if kill, _, _, _ := EvaluateReupZeroView(posts, 4); kill {
		t.Errorf("ngưỡng 4 với 3 bài 0-view không được kill")
	}
	// n <= 0 → về mặc định 5.
	if kill, _, _, _ := EvaluateReupZeroView(posts, 0); kill {
		t.Errorf("n=0 phải dùng mặc định 5 → không kill với 3 bài")
	}
}

func TestFetchYouTubeVideoViews(t *testing.T) {
	// Không key → ErrNotConnected (fail-closed, không bịa).
	if _, err := FetchYouTubeVideoViews(context.Background(), "", "abc"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("thiếu key phải ErrNotConnected, được %v", err)
	}
	// Mock server trả viewCount thật.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "vid1" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"items":[{"statistics":{"viewCount":"12345"}}]}`)
	}))
	defer srv.Close()
	old := youtubeAPIBase
	youtubeAPIBase = srv.URL
	defer func() { youtubeAPIBase = old }()

	n, err := FetchYouTubeVideoViews(context.Background(), "key", "vid1")
	if err != nil {
		t.Fatalf("FetchYouTubeVideoViews: %v", err)
	}
	if n != 12345 {
		t.Errorf("view=%d, muốn 12345", n)
	}
	// Video không tồn tại → lỗi (không bịa 0).
	if _, err := FetchYouTubeVideoViews(context.Background(), "key", "nope"); err == nil {
		t.Errorf("video lạ phải lỗi")
	}
}
