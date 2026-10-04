package automation

// Test Đợt E: transform tick + post tick + kill tick.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// fakeVoice tạo WAV sine 3s bằng ffmpeg (không gọi VieNeu thật).
type fakeVoice struct{ dir string }

func (f fakeVoice) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text rỗng")
	}
	out := filepath.Join(f.dir, fmt.Sprintf("vo-%d.wav", time.Now().UnixNano()))
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-c:a", "pcm_s16le", "-ar", "24000", "-ac", "1", out)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

type fakeAccounts struct{ accts []*network.Account }

func (f *fakeAccounts) List(statuses ...string) ([]*network.Account, error) { return f.accts, nil }
func (f *fakeAccounts) Get(id int64) (*network.Account, error) {
	for _, a := range f.accts {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("không thấy")
}
func (f *fakeAccounts) Transition(id int64, status string, extra map[string]any) (*network.Account, error) {
	return nil, nil
}

// newE2EHarness dựng service đủ cho tick E: ledger + growth + reup.
func newE2EHarness(t *testing.T, st mapSettings) (*Service, *reup.Store, *growth.Store) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ledger.db")
	l, err := ledger.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	sqldb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	gs, err := growth.NewStore(sqldb)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := reup.NewStore(filepath.Join(dir, "reup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rs.Close() })
	svc := NewService()
	svc.Gate = &fakeGate{}
	svc.Ledger = l
	svc.Growth = gs
	svc.Settings = st
	svc.Env = fakeEnv{}
	svc.Reup = rs
	svc.ReupWorkDir = filepath.Join(dir, "reup")
	svc.ReupTTS = fakeVoice{dir: dir}
	return svc, rs, gs
}

func makeClip(t *testing.T, dir, name string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=640x1138:duration=5:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("tạo clip: %v", err)
	}
	return out
}

func queueDownloaded(t *testing.T, rs *reup.Store, clip, douyinID string) reup.Video {
	t.Helper()
	v, err := rs.QueueVideo(reup.Video{DouyinID: douyinID, URL: "https://douyin/x", Title: "Clip " + douyinID})
	if err != nil {
		t.Fatal(err)
	}
	if err := rs.MarkDownloaded(v.ID, clip, "sha-"+douyinID, "ytdlp", false, 5, "640x1138", "ffprobe"); err != nil {
		t.Fatal(err)
	}
	v, err = rs.GetVideo(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReupTransformTickGates(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newE2EHarness(t, mapSettings{})
	svc.Gate = &fakeGate{kill: true}
	if n := svc.ReupTransformTick(ctx); len(n) != 0 {
		t.Errorf("kill switch phải chặn transform tick")
	}
	svc, _, _ = newE2EHarness(t, mapSettings{})
	svc.Gate = &fakeGate{dry: true}
	if n := svc.ReupTransformTick(ctx); len(n) != 0 {
		t.Errorf("dry-run phải chặn transform tick")
	}
	svc, _, _ = newE2EHarness(t, mapSettings{KeyReupTransformEnabled: "0"})
	if n := svc.ReupTransformTick(ctx); len(n) != 0 {
		t.Errorf("tắt công tắc phải bỏ qua")
	}
	svc, _, _ = newE2EHarness(t, mapSettings{})
	svc.Reup = nil
	if n := svc.ReupTransformTick(ctx); len(n) != 0 {
		t.Errorf("thiếu store phải bỏ qua im lặng")
	}
}

func TestReupTransformTickRuns(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("thiếu ffmpeg")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dir := t.TempDir()
	clip := makeClip(t, dir, "a.mp4")
	svc, rs, _ := newE2EHarness(t, mapSettings{})
	queueDownloaded(t, rs, clip, "e1")

	notes := svc.ReupTransformTick(ctx)
	t.Logf("notes: %v", notes)
	posts, err := rs.ListPostsByStatus(reup.PostTransformed, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("phải có 1 bài transformed, được %d (notes: %v)", len(posts), notes)
	}
	if _, err := os.Stat(posts[0].FilePath); err != nil {
		t.Errorf("file transform không tồn tại: %v", err)
	}
	if posts[0].TransformLevel != reup.Level1 {
		t.Errorf("mức mặc định phải 1, được %d", posts[0].TransformLevel)
	}
	// Chạy lại → không transform trùng.
	notes2 := svc.ReupTransformTick(ctx)
	_ = notes2
	posts2, _ := rs.ListPosts(0)
	if len(posts2) != 1 {
		t.Errorf("tick lại không được tạo bài trùng, có %d bài", len(posts2))
	}
}

func TestReupKillTickNoData(t *testing.T) {
	ctx := context.Background()
	svc, rs, _ := newE2EHarness(t, mapSettings{})
	// 3 bài đã đăng nhưng chưa có số liệu → "chờ số liệu", không kill.
	for i := 0; i < 3; i++ {
		p, _ := rs.CreatePost([]int64{int64(i + 1)}, reup.Level1)
		_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
		_ = rs.MarkPostPosted(p.ID, "tiktok", fmt.Sprintf("r%d", i), "")
	}
	notes := svc.ReupKillTick(ctx)
	t.Logf("notes: %v", notes)
	if v, _ := svc.Settings.Get(KeyReupPostEnabled); v == "0" {
		t.Errorf("chưa có số liệu mà tắt đăng là sai (fail-closed)")
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "chờ số liệu") {
			found = true
		}
	}
	if !found {
		t.Errorf("phải báo 'chờ số liệu', được %v", notes)
	}
}

func TestReupKillTickTriggers(t *testing.T) {
	ctx := context.Background()
	svc, rs, gs := newE2EHarness(t, mapSettings{})
	// 5 bài 0-view liên tiếp (số liệu thật) → kill.
	for i := 0; i < 5; i++ {
		p, _ := rs.CreatePost([]int64{int64(i + 1)}, reup.Level1)
		_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
		_ = rs.MarkPostPosted(p.ID, "youtube", fmt.Sprintf("yt%d", i), "")
		_ = rs.SetPostMetrics(p.ID, 0)
	}
	notes := svc.ReupKillTick(ctx)
	t.Logf("notes: %v", notes)
	if v, _ := svc.Settings.Get(KeyReupPostEnabled); v != "0" {
		t.Errorf("kill rule phải tắt reup.post_enabled, settings=%q", v)
	}
	alerts, err := gs.ListAlerts(0, 10)
	if err != nil || len(alerts) == 0 {
		t.Fatalf("phải có alert kill, err=%v n=%d", err, len(alerts))
	}
	if alerts[0].Kind != "reup_kill" {
		t.Errorf("alert kind=%q, muốn reup_kill", alerts[0].Kind)
	}
	// Bài đã đăng không bị xoá.
	posted, _ := rs.ListPostedPosts(10)
	if len(posted) != 5 {
		t.Errorf("kill không được xoá bài đã đăng, còn %d", len(posted))
	}
}

// TestReupKillTickPerSource: nguồn 5 bài 0-view liên tiếp → tự tắt;
// nguồn có view vẫn sống; kill toàn cục không kích hoạt oan.
func TestReupKillTickPerSource(t *testing.T) {
	ctx := context.Background()
	svc, rs, gs := newE2EHarness(t, mapSettings{})
	srcA, _ := rs.AddSource("user", "chet", "Nguồn Chết")
	srcB, _ := rs.AddSource("user", "song", "Nguồn Sống")
	// Nguồn A: 5 bài 0-view liên tiếp → bị tắt.
	for i := 0; i < 5; i++ {
		v, _ := rs.QueueVideo(reup.Video{SourceID: srcA.ID, DouyinID: fmt.Sprintf("a%d", i), URL: "https://x/a", Status: reup.StatusDownloaded})
		p, _ := rs.CreatePost([]int64{v.ID}, reup.Level1)
		_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
		_ = rs.MarkPostPosted(p.ID, "youtube", fmt.Sprintf("yta%d", i), "")
		_ = rs.SetPostMetrics(p.ID, 0)
	}
	// Nguồn B: 2 bài có view → sống.
	for i := 0; i < 2; i++ {
		v, _ := rs.QueueVideo(reup.Video{SourceID: srcB.ID, DouyinID: fmt.Sprintf("b%d", i), URL: "https://x/b", Status: reup.StatusDownloaded})
		p, _ := rs.CreatePost([]int64{v.ID}, reup.Level1)
		_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
		_ = rs.MarkPostPosted(p.ID, "youtube", fmt.Sprintf("ytb%d", i), "")
		_ = rs.SetPostMetrics(p.ID, 100)
	}
	notes := svc.ReupKillTick(ctx)
	t.Logf("notes: %v", notes)
	srcs, _ := rs.ListSources()
	for _, sc := range srcs {
		if sc.ID == srcA.ID && sc.Enabled {
			t.Errorf("nguồn 5 bài 0-view phải bị tắt tự động")
		}
		if sc.ID == srcB.ID && !sc.Enabled {
			t.Errorf("nguồn có view không được tắt")
		}
	}
	alerts, _ := gs.ListAlerts(0, 10)
	found := false
	for _, a := range alerts {
		if a.Kind == "reup_kill_source" {
			found = true
		}
	}
	if !found {
		t.Errorf("phải có alert reup_kill_source, notes=%v", notes)
	}
}

func TestReupPostTickNoAccount(t *testing.T) {
	ctx := context.Background()
	svc, rs, _ := newE2EHarness(t, mapSettings{})
	p, _ := rs.CreatePost([]int64{1}, reup.Level1)
	_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
	notes := svc.ReupPostTick(ctx)
	found := false
	for _, n := range notes {
		if strings.Contains(n, "chưa chọn kênh") {
			found = true
		}
	}
	if !found {
		t.Errorf("chưa chọn kênh phải báo rõ, được %v", notes)
	}
}

func TestReupPostTickNoPublisher(t *testing.T) {
	ctx := context.Background()
	svc, rs, _ := newE2EHarness(t, mapSettings{KeyReupPostAccount: "kenh1"})
	svc.Accounts = &fakeAccounts{accts: []*network.Account{{ID: 1, Username: "kenh1"}}}
	p, _ := rs.CreatePost([]int64{1}, reup.Level1)
	_ = rs.SetPostTransformed(p.ID, "/tmp/x.mp4")
	notes := svc.ReupPostTick(ctx)
	t.Logf("notes: %v", notes)
	got, _ := rs.GetPost(p.ID)
	if got.Status != reup.PostFailed {
		t.Errorf("không publisher nào cấu hình → bài phải failed (fail-closed), được %s", got.Status)
	}
}
