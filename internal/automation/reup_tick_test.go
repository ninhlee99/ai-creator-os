package automation

// Reup tick (Đợt D): discover 6h/lần → download. Test với TikWM mock;
// yt-dlp bị ép gãy (release base 404) để đi nhánh fallback TikWM.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// reupTestServer mock TikWM: user posts + lookup (wmplay có watermark
// trỏ về file server) + file mp4 giả (QC đi nhánh size-only vì test ẩn
// ffprobe khỏi PATH).
func reupTestServer() *httptest.Server {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user/posts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"videos":[
			{"video_id":"r1","title":"Một","play_count":500,"digg_count":50,"duration":10,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}},
			{"video_id":"r2","title":"Hai","play_count":900,"digg_count":90,"duration":12,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}}
		],"cursor":0,"hasMore":false}}`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Trích id từ ?url= (share URL) để mỗi video có wmplay riêng —
		// tránh dedupe sha256 trong test.
		id := "r9"
		if u := r.URL.Query().Get("url"); u != "" {
			if i := strings.LastIndex(u, "/video/"); i >= 0 {
				if s := strings.SplitN(u[i+7:], "?", 2)[0]; s != "" {
					id = s
				}
			}
		}
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{
			"id":%q,"title":"Chín","wmplay":"%s/f.mp4?v=%s","duration":14,
			"play_count":999,"digg_count":99,
			"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}}}`, id, base, id)
	})
	mux.HandleFunc("/f.mp4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		// Mỗi video nội dung khác nhau — tránh dedupe sha256 trong test.
		fmt.Fprintf(w, "fake-reup-video-bytes-%s", r.URL.Query().Get("v"))
	})
	// GitHub release giả: luôn 404 → yt-dlp Ensure thất bại → fallback TikWM.
	mux.HandleFunc("/release/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	return srv
}

func newReupService(t *testing.T, st mapSettings, srv *httptest.Server) *Service {
	t.Helper()
	dir := t.TempDir()
	rs, err := reup.NewStore(filepath.Join(dir, "reup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rs.Close() })
	svc := NewService()
	svc.Gate = &fakeGate{}
	svc.Settings = st
	svc.Reup = rs
	svc.ReupWorkDir = filepath.Join(dir, "reup")
	svc.ReupBinDir = filepath.Join(dir, "bin")
	svc.ReupTikWMBaseURL = srv.URL
	svc.ReupYtDlpRelease = srv.URL + "/release"
	return svc
}

// hideFFprobe ẩn ffprobe khỏi PATH để QC đi nhánh size-only deterministic.
func hideFFprobe(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestReupTickGates(t *testing.T) {
	srv := reupTestServer()
	defer srv.Close()
	ctx := context.Background()

	svc := newReupService(t, mapSettings{}, srv)
	svc.Gate = &fakeGate{kill: true}
	if n := svc.ReupTick(ctx); len(n) != 0 {
		t.Errorf("kill switch phải chặn tick")
	}

	svc = newReupService(t, mapSettings{}, srv)
	svc.Gate = &fakeGate{dry: true}
	if n := svc.ReupTick(ctx); len(n) != 0 {
		t.Errorf("dry-run phải chặn tick")
	}

	svc = newReupService(t, mapSettings{KeyReupDiscoverEnabled: "0"}, srv)
	if n := svc.ReupTick(ctx); len(n) != 0 {
		t.Errorf("tắt công tắc phải bỏ qua")
	}

	svc = newReupService(t, mapSettings{}, srv)
	svc.Reup = nil
	if n := svc.ReupTick(ctx); len(n) != 0 {
		t.Errorf("thiếu store phải bỏ qua im lặng")
	}
}

func TestReupTickNoSources(t *testing.T) {
	srv := reupTestServer()
	defer srv.Close()
	svc := newReupService(t, mapSettings{}, srv)
	notes := svc.ReupTick(context.Background())
	if len(notes) == 0 {
		t.Fatalf("phải có note khi chưa có nguồn")
	}
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "chưa có nguồn") {
		t.Errorf("note phải nói rõ chưa có nguồn: %v", notes)
	}
	// Đã stamp last run (không quét lại liên tục).
	if _, ok := svc.Settings.Get(KeyReupDiscoverLastRun); !ok {
		t.Errorf("phải stamp last run")
	}
	// Chưa đến hạn → bỏ qua.
	if n := svc.ReupTick(context.Background()); len(n) != 0 {
		t.Errorf("chưa đến hạn 6h phải bỏ qua")
	}
}

func TestReupTickDiscoversAndDownloads(t *testing.T) {
	hideFFprobe(t)
	srv := reupTestServer()
	defer srv.Close()
	svc := newReupService(t, mapSettings{}, srv)
	if _, err := svc.Reup.AddSource("user", "than_tien", "Thần Tiên"); err != nil {
		t.Fatal(err)
	}
	notes := svc.ReupTick(context.Background())
	if len(notes) == 0 {
		t.Fatalf("tick phải trả notes")
	}
	if !strings.Contains(notes[0], "2 video mới") || !strings.Contains(notes[0], "2 đã tải") {
		t.Errorf("notes sai: %v", notes)
	}
	vs, err := svc.Reup.ListVideos(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("phải có 2 video trong kho, được %d", len(vs))
	}
	for _, v := range vs {
		if v.Status != reup.StatusDownloaded {
			t.Errorf("video %s phải downloaded: %+v", v.DouyinID, v)
		}
		if v.Via != "tikwm" || v.WatermarkFree {
			t.Errorf("fallback tikwm phải ghi via=tikwm và giữ watermark (watermark_free=false): %+v", v)
		}
		if v.QCMethod != "size-only" {
			t.Errorf("QC phải ghi phương pháp trung thực: %+v", v)
		}
		if _, err := os.Stat(v.FilePath); err != nil {
			t.Errorf("file video phải tồn tại: %v", err)
		}
	}
	// Tick lại ngay → chưa đến hạn, không tải trùng.
	notes2 := svc.ReupTick(context.Background())
	if len(notes2) != 0 {
		t.Errorf("tick lại khi chưa đến hạn phải bỏ qua: %v", notes2)
	}
}
