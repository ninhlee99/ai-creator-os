package reup

// Test Đợt D: không tải thật — mock yt-dlp bằng script giả, mock TikWM
// bằng httptest (BaseURL inject được).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ------------------------------------------------------------------ store

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "reup.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreSources(t *testing.T) {
	s := newTestStore(t)
	a, err := s.AddSource("user", "@than_tien", "Thần Tiên")
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if a.Value != "than_tien" || !a.Enabled {
		t.Errorf("chuẩn hoá sai: %+v", a)
	}
	// Thêm trùng → trả về cũ, không lỗi, không nhân đôi.
	b, err := s.AddSource("user", "than_tien", "Tên mới")
	if err != nil {
		t.Fatalf("AddSource trùng: %v", err)
	}
	if b.ID != a.ID {
		t.Errorf("trùng phải trả về cùng ID: %d vs %d", a.ID, b.ID)
	}
	all, _ := s.ListSources()
	if len(all) != 1 {
		t.Fatalf("phải có đúng 1 nguồn, được %d", len(all))
	}
	if err := s.SetSourceEnabled(a.ID, false); err != nil {
		t.Fatalf("SetSourceEnabled: %v", err)
	}
	if on, _ := s.EnabledSources(); len(on) != 0 {
		t.Errorf("tắt rồi mà EnabledSources vẫn trả về")
	}
	if err := s.SetSourceEnabled(a.ID, true); err != nil {
		t.Fatalf("bật lại: %v", err)
	}
	if on, _ := s.EnabledSources(); len(on) != 1 {
		t.Errorf("bật rồi mà EnabledSources trống")
	}
	if err := s.DeleteSource(a.ID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	if all, _ := s.ListSources(); len(all) != 0 {
		t.Errorf("xoá rồi vẫn còn nguồn")
	}
	if err := s.DeleteSource(9999); err == nil {
		t.Errorf("xoá nguồn không tồn tại phải lỗi")
	}
}

func TestStoreVideosDedupe(t *testing.T) {
	s := newTestStore(t)
	v, err := s.QueueVideo(Video{DouyinID: "123", URL: "https://www.douyin.com/video/123", Title: "A"})
	if err != nil {
		t.Fatalf("QueueVideo: %v", err)
	}
	if v.Status != StatusQueued {
		t.Errorf("mặc định phải queued, được %q", v.Status)
	}
	if !s.HasDouyinID("123") {
		t.Errorf("HasDouyinID phải true sau khi queue")
	}
	if s.HasDouyinID("456") {
		t.Errorf("HasDouyinID phải false với id lạ")
	}
	// Queue trùng douyin_id → trả về bản cũ.
	dup, err := s.QueueVideo(Video{DouyinID: "123", URL: "https://x/video/123"})
	if err != nil {
		t.Fatalf("QueueVideo trùng: %v", err)
	}
	if dup.ID != v.ID {
		t.Errorf("trùng phải trả về cùng ID")
	}
	if err := s.MarkDownloaded(v.ID, "/tmp/a.mp4", "abc123", "ytdlp", false, 12.5, "1080x1920", "size-only"); err != nil {
		t.Fatalf("MarkDownloaded: %v", err)
	}
	if !s.HasSHA256("abc123") {
		t.Errorf("HasSHA256 phải true sau khi mark downloaded")
	}
	if s.HasSHA256("zzz") {
		t.Errorf("HasSHA256 phải false với sha lạ")
	}
	got, _ := s.GetVideo(v.ID)
	if got.Status != StatusDownloaded || got.SHA256 != "abc123" || got.WatermarkFree {
		t.Errorf("MarkDownloaded ghi sai: %+v", got)
	}
	st, _ := s.Stats()
	if st.Downloaded != 1 {
		t.Errorf("Stats.Downloaded phải = 1, được %+v", st)
	}
	if err := s.SetStatus(v.ID, StatusFailed, "lý do"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, _ = s.GetVideo(v.ID)
	if got.Status != StatusFailed || got.FailReason != "lý do" {
		t.Errorf("SetStatus ghi sai: %+v", got)
	}
}

func TestExtractDouyinID(t *testing.T) {
	cases := map[string]string{
		"https://www.douyin.com/video/7381234567890123456": "7381234567890123456",
		"https://v.douyin.com/abc/video/12345?x=1":         "12345",
		"https://www.douyin.com/user/xxx":                  "",
		"":                                                 "",
	}
	for in, want := range cases {
		if got := ExtractDouyinID(in); got != want {
			t.Errorf("ExtractDouyinID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsExtractorBroken(t *testing.T) {
	if !IsExtractorBroken("ERROR: Unable to extract video data") {
		t.Errorf("phải nhận diện 'unable to extract'")
	}
	if !IsExtractorBroken("Fresh cookies needed") {
		t.Errorf("phải nhận diện 'fresh cookies'")
	}
	if IsExtractorBroken("ERROR: Video unavailable") {
		t.Errorf("lỗi thường không phải extractor gãy")
	}
}

// noFFprobePATH dựng PATH chỉ chứa coreutils tối thiểu cho script giả
// (mkdir/dirname/sed), KHÔNG có ffprobe → QC đi nhánh size-only một cách
// deterministic, không phụ thuộc máy chạy test có cài ffprobe hay không.
func noFFprobePATH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, b := range []string{"mkdir", "dirname", "sed"} {
		p, err := exec.LookPath(b)
		if err != nil {
			t.Skipf("môi trường test thiếu %s", b)
		}
		if err := os.Symlink(p, filepath.Join(dir, b)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// ------------------------------------------------------------- fake yt-dlp

// fakeYtDlp ghi script giả tên đúng asset của OS hiện tại vào binDir.
// Script hiểu: --version → in version; download → tạo file nội dung cố
// định (để test dedupe sha256) và in 4 dòng print.
func fakeYtDlp(t *testing.T, binDir, version string, failMode string) *Manager {
	t.Helper()
	name, err := assetName()
	if err != nil {
		t.Fatalf("assetName: %v", err)
	}
	// Script giả: --version → in version; download → tạo file nội dung cố
	// định (để test dedupe sha256) và in 4 dòng print như yt-dlp thật.
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "` + version + `"; exit 0; fi
OUT=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then OUT="$a"; fi
  prev="$a"
done
# thay %(id)s.%(ext)s bằng tên cố định
OUTFILE=$(echo "$OUT" | sed 's/%(id)s/vid999/; s/%(ext)s/mp4/')
mkdir -p "$(dirname "$OUTFILE")"
if [ "` + failMode + `" = "broken" ]; then echo "ERROR: Unable to extract video data (signature)" >&2; exit 1; fi
if [ "` + failMode + `" = "empty" ]; then : > "$OUTFILE"; else printf 'fake-video-bytes' > "$OUTFILE"; fi
echo "$OUTFILE"
echo "vid999"
echo "Tieu de gia"
echo "15.5"
`
	p := filepath.Join(binDir, name)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	m := NewManager(binDir)
	return m
}

func TestYtDlpVersionAndDownload(t *testing.T) {
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "")
	ctx := context.Background()
	if !m.Installed() {
		t.Fatalf("fake binary phải pass health check")
	}
	ver, err := m.Version(ctx)
	if err != nil || ver != "2026.10.01" {
		t.Fatalf("Version = %q, %v", ver, err)
	}
	outDir := filepath.Join(dir, "out")
	os.MkdirAll(outDir, 0o755)
	res, broken, err := m.Download(ctx, "https://www.douyin.com/video/1", outDir)
	if err != nil || broken {
		t.Fatalf("Download: %v broken=%v", err, broken)
	}
	if res.VideoID != "vid999" || res.Title != "Tieu de gia" || res.Duration != 15.5 {
		t.Errorf("parse print sai: %+v", res)
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Errorf("file tải không tồn tại: %v", err)
	}
}

func TestYtDlpExtractorBrokenDetected(t *testing.T) {
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "broken")
	outDir := filepath.Join(dir, "out")
	os.MkdirAll(outDir, 0o755)
	_, broken, err := m.Download(context.Background(), "https://www.douyin.com/video/1", outDir)
	if err == nil || !broken {
		t.Fatalf("phải báo extractor broken, err=%v broken=%v", err, broken)
	}
}

func TestYtDlpUpdateFromRelease(t *testing.T) {
	old := minBinaryBytes
	minBinaryBytes = 10
	defer func() { minBinaryBytes = old }()
	// Server giả làm GitHub release: trả script chạy được.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#!/bin/sh\necho 2099.01.01\n")
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "bin"))
	m.ReleaseBase = srv.URL
	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	ver, err := m.Version(context.Background())
	if err != nil || ver != "2099.01.01" {
		t.Fatalf("binary sau update phải chạy được: %q %v", ver, err)
	}
}

func TestYtDlpUpdateBadRelease(t *testing.T) {
	old := minBinaryBytes
	minBinaryBytes = 10
	defer func() { minBinaryBytes = old }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	m := NewManager(t.TempDir())
	m.ReleaseBase = srv.URL
	if err := m.Update(context.Background()); err == nil {
		t.Fatalf("release 404 phải lỗi rõ ràng")
	}
}

// ------------------------------------------------------------------ tikwm

func tikwmMock(t *testing.T) *TikWM {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/user/posts"):
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{"videos":[
				{"video_id":"v1","title":"Một","play_count":100,"digg_count":10,"duration":10,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}},
				{"video_id":"v2","title":"Hai","play_count":300,"digg_count":30,"duration":12,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}},
				{"video_id":"v3","title":"Ba","play_count":200,"digg_count":20,"duration":11,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}},
				{"video_id":"v4","title":"Bốn","play_count":50,"digg_count":5,"duration":9,"author":{"unique_id":"than_tien","nickname":"Thần Tiên"}}
			],"cursor":0,"hasMore":false}}`)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			fmt.Fprint(w, `{"code":0,"msg":"success","data":{
				"id":"v9","title":"Chín","cover":"http://x/c.jpg",
				"wmplay":"http://x/9wm.mp4",
				"duration":14,"play_count":999,"digg_count":99,
				"author":{"id":"a1","unique_id":"than_tien","nickname":"Thần Tiên"}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	tw := NewTikWM()
	tw.BaseURL = srv.URL
	return tw
}

func TestTikWMLookup(t *testing.T) {
	tw := tikwmMock(t)
	v, err := tw.Lookup(context.Background(), "https://www.douyin.com/video/v9")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if v.ID != "v9" || v.WMPlayURL != "http://x/9wm.mp4" {
		t.Errorf("parse lookup sai: %+v", v)
	}
	if v.Nickname != "Thần Tiên" || v.PlayCount != 999 {
		t.Errorf("metadata sai: %+v", v)
	}
}

// TestTikWMLookupRequiresWatermark: thiếu wmplay → fail-closed, không
// bao giờ rơi xuống bản no-watermark (ranh giới cứng Đợt I).
func TestTikWMLookupRequiresWatermark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{
			"id":"v9","title":"Chín","play":"http://x/9.mp4","hdplay":"http://x/9hd.mp4",
			"duration":14,"author":{"unique_id":"than_tien"}}}`)
	}))
	defer srv.Close()
	tw := NewTikWM()
	tw.BaseURL = srv.URL
	if _, err := tw.Lookup(context.Background(), "https://www.douyin.com/video/v9"); err == nil {
		t.Fatalf("thiếu wmplay phải lỗi (không dùng bản no-watermark)")
	} else if !strings.Contains(err.Error(), "wmplay") {
		t.Errorf("lỗi phải nhắc tới wmplay: %v", err)
	}
}

func TestTikWMUserPosts(t *testing.T) {
	tw := tikwmMock(t)
	vs, err := tw.UserPosts(context.Background(), "than_tien", 30)
	if err != nil {
		t.Fatalf("UserPosts: %v", err)
	}
	if len(vs) != 4 {
		t.Fatalf("phải có 4 video, được %d", len(vs))
	}
}

// -------------------------------------------------------------- downloader

func TestDownloadDedupeDouyinID(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "")
	d := NewDownloader(m, tikwmMock(t), s, filepath.Join(dir, "out"))
	// Video đã có sẵn trong kho.
	existing, _ := s.QueueVideo(Video{DouyinID: "777", URL: "https://www.douyin.com/video/777", Status: StatusDownloaded})
	got, err := d.DownloadVideo(context.Background(), "https://www.douyin.com/video/777")
	if err != nil {
		t.Fatalf("DownloadVideo: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("trùng douyin_id phải trả về bản cũ, không tải lại")
	}
}

func TestDownloadQCFailsOnEmpty(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "empty")
	d := NewDownloader(m, tikwmMock(t), s, filepath.Join(dir, "out"))
	_, err := d.DownloadVideo(context.Background(), "https://www.douyin.com/video/555")
	if err == nil || !strings.Contains(err.Error(), "QC") {
		t.Fatalf("file rỗng phải rớt QC, err=%v", err)
	}
	vs, _ := s.ListVideos(0)
	if len(vs) != 1 || vs[0].Status != StatusFailed {
		t.Fatalf("video rớt QC phải status=failed: %+v", vs)
	}
}

func TestDownloadSha256Dedupe(t *testing.T) {
	noFFprobePATH(t) // fake video không phải mp4 thật → QC size-only
	s := newTestStore(t)
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "")
	d := NewDownloader(m, tikwmMock(t), s, filepath.Join(dir, "out"))
	ctx := context.Background()
	// Fake script luôn ghi cùng nội dung → sha256 trùng nhau.
	// (ffprobe có thể không có trong môi trường test → QC size-only.)
	if _, err := d.DownloadVideo(ctx, "https://www.douyin.com/video/aaa"); err != nil {
		t.Fatalf("tải 1: %v", err)
	}
	got, err := d.DownloadVideo(ctx, "https://www.douyin.com/video/bbb")
	if err != nil {
		t.Fatalf("tải 2: %v", err)
	}
	if got.Status != StatusFailed || !strings.Contains(got.FailReason, "trùng nội dung") {
		t.Errorf("video trùng sha256 phải failed với lý do trùng: %+v", got)
	}
}

func TestDownloadTikWMFallback(t *testing.T) {
	noFFprobePATH(t) // fake video không phải mp4 thật → QC size-only
	s := newTestStore(t)
	dir := t.TempDir()
	// yt-dlp không có binary + release gãy → Ensure thất bại → fallback TikWM.
	m := NewManager(filepath.Join(dir, "bin-missing"))
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()
	m.ReleaseBase = bad.URL
	// File server cho link wmplay (có watermark).
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		fmt.Fprint(w, "fake-tikwm-video-bytes")
	}))
	defer fileSrv.Close()
	// API server trả wmplay trỏ về file server.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"id":"v9","title":"Chín","wmplay":%q,"duration":14}}`, fileSrv.URL+"/v.mp4")
	}))
	defer apiSrv.Close()
	tw := NewTikWM()
	tw.BaseURL = apiSrv.URL
	d := NewDownloader(m, tw, s, filepath.Join(dir, "out"))
	v, err := d.DownloadVideo(context.Background(), "https://www.douyin.com/video/v9")
	if err != nil {
		t.Fatalf("DownloadVideo qua TikWM fallback: %v", err)
	}
	if v.Status != StatusDownloaded || v.Via != "tikwm" {
		t.Errorf("phải downloaded qua tikwm: %+v", v)
	}
	if v.WatermarkFree {
		t.Errorf("TikWM fallback phải giữ watermark (wmplay) — WatermarkFree phải false")
	}
	if v.WatermarkLabel() != "có watermark" {
		t.Errorf("nhãn phải là 'có watermark', được %q", v.WatermarkLabel())
	}
}

func TestDownloadRetryOnlyFailed(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	m := fakeYtDlp(t, filepath.Join(dir, "bin"), "2026.10.01", "")
	d := NewDownloader(m, tikwmMock(t), s, filepath.Join(dir, "out"))
	v, _ := s.QueueVideo(Video{URL: "https://www.douyin.com/video/1", Status: StatusDownloaded})
	if _, err := d.Retry(context.Background(), v.ID); err == nil {
		t.Errorf("retry video đã downloaded phải lỗi")
	}
}

// --------------------------------------------------------------- discover

func TestDiscoverPicksTopPlayCount(t *testing.T) {
	s := newTestStore(t)
	src, _ := s.AddSource("user", "than_tien", "Thần Tiên")
	// v3 đã có trong kho → discover phải bỏ qua.
	if _, err := s.QueueVideo(Video{DouyinID: "v3", URL: "https://x/3", Status: StatusDownloaded}); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(s, tikwmMock(t))
	cands, notes := d.Discover(context.Background(), 2)
	if len(notes) != 0 {
		t.Fatalf("notes phải rỗng, được %v", notes)
	}
	if len(cands) != 2 {
		t.Fatalf("phải pick 2 video, được %d: %+v", len(cands), cands)
	}
	// v2 (300) > v1 (100); v3 (200) đã có nên bị loại.
	if cands[0].DouyinID != "v2" || cands[1].DouyinID != "v1" {
		t.Errorf("phải pick theo play_count cao nhất (v2, v1), được %+v", cands)
	}
	if cands[0].SourceID != src.ID {
		t.Errorf("candidate thiếu source_id")
	}
	// Đã queue vào kho ở trạng thái queued.
	if !s.HasDouyinID("v2") {
		t.Errorf("video pick phải được queue vào kho")
	}
}

// TestEngagementScore: video ít view nhưng tỷ lệ like cao phải thắng
// video nhiều view nhưng like thấp.
func TestEngagementScore(t *testing.T) {
	viral := TikWMVideo{ID: "a", PlayCount: 1000, DiggCount: 500} // 1000×1.5=1500
	liked := TikWMVideo{ID: "b", PlayCount: 800, DiggCount: 800}  // 800×2=1600
	flat := TikWMVideo{ID: "c", PlayCount: 5000, DiggCount: 10}   // 5000×1.002≈5010
	if engagementScore(liked) <= engagementScore(viral) {
		t.Errorf("like-rate cao phải thắng view thuần: %f vs %f",
			engagementScore(liked), engagementScore(viral))
	}
	if engagementScore(flat) <= engagementScore(liked) {
		t.Errorf("view vượt trội vẫn phải thắng: %f vs %f",
			engagementScore(flat), engagementScore(liked))
	}
	if engagementScore(TikWMVideo{}) != 0 {
		t.Errorf("video 0 view phải điểm 0")
	}
}

// TestDiscoverFiltersDuration: video quá ngắn/dài bị loại khỏi pick.
func TestDiscoverFiltersDuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success","data":{"videos":[
			{"video_id":"ngan","title":"Ngắn","play_count":9999,"digg_count":999,"duration":2,"author":{"unique_id":"u","nickname":"U"}},
			{"video_id":"dai","title":"Dài","play_count":9999,"digg_count":999,"duration":500,"author":{"unique_id":"u","nickname":"U"}},
			{"video_id":"vua","title":"Vừa","play_count":100,"digg_count":10,"duration":30,"author":{"unique_id":"u","nickname":"U"}}
		],"cursor":0,"hasMore":false}}`)
	}))
	defer srv.Close()
	tw := NewTikWM()
	tw.BaseURL = srv.URL
	s := newTestStore(t)
	if _, err := s.AddSource("user", "u", "U"); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(s, tw)
	cands, _ := d.Discover(context.Background(), 5)
	if len(cands) != 1 || cands[0].DouyinID != "vua" {
		t.Errorf("chỉ video 30s được pick, được %+v", cands)
	}
}

func TestDiscoverHashtagHonest(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddSource("hashtag", "thantien", ""); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(s, tikwmMock(t))
	cands, notes := d.Discover(context.Background(), 3)
	if len(cands) != 0 {
		t.Errorf("hashtag không được bịa candidate")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "URL trực tiếp") {
		t.Errorf("phải note trung thực về hashtag: %v", notes)
	}
}

func TestDiscoverNoSources(t *testing.T) {
	s := newTestStore(t)
	d := NewDiscoverer(s, tikwmMock(t))
	cands, notes := d.Discover(context.Background(), 3)
	if len(cands) != 0 || len(notes) == 0 {
		t.Errorf("chưa có nguồn phải note rõ, không tải gì: %v %v", cands, notes)
	}
}

func TestVideoLabels(t *testing.T) {
	v := Video{Status: StatusDownloaded, Via: "tikwm"}
	if v.StatusLabel() != "đã tải" || v.WatermarkLabel() != "có watermark" {
		t.Errorf("label sai: %+v", v)
	}
	// Bản ghi cũ trước Đợt I (từng tải no-watermark) vẫn giữ nhãn lịch sử.
	v0 := Video{Status: StatusDownloaded, Via: "tikwm", WatermarkFree: true}
	if v0.WatermarkLabel() != "không watermark" {
		t.Errorf("bản ghi cũ phải giữ nhãn lịch sử: %q", v0.WatermarkLabel())
	}
	v2 := Video{Status: StatusDownloaded, Via: "ytdlp"}
	if v2.WatermarkLabel() != "có thể có watermark" {
		t.Errorf("ytdlp phải ghi 'có thể có watermark': %q", v2.WatermarkLabel())
	}
	v3 := Video{Status: StatusFailed}
	if v3.StatusLabel() != "tải lỗi" || v3.WatermarkLabel() != "—" {
		t.Errorf("label failed sai: %+v", v3)
	}
}
