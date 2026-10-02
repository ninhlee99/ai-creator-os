package reup

// Test Đợt E: reup_posts (transform + đăng + metrics) và migration v2.

import (
	"testing"
)

func TestPostCRUD(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreatePost([]int64{11, 22}, Level2)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if p.Status != PostPending || p.TransformLevel != Level2 {
		t.Errorf("bài mới sai: %+v", p)
	}
	if len(p.VideoIDs) != 2 || p.VideoIDs[0] != 11 {
		t.Errorf("video_ids sai: %v", p.VideoIDs)
	}
	if p.Views != -1 {
		t.Errorf("views mặc định phải -1 (chờ số liệu), được %d", p.Views)
	}
	if got := p.ViewsLabel(); got != "chờ số liệu" {
		t.Errorf("ViewsLabel=%q", got)
	}

	got, err := s.GetPost(p.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got.ID != p.ID || len(got.VideoIDs) != 2 {
		t.Errorf("GetPost sai: %+v", got)
	}

	if err := s.SetPostTransformed(p.ID, "/tmp/x.mp4"); err != nil {
		t.Fatalf("SetPostTransformed: %v", err)
	}
	got, _ = s.GetPost(p.ID)
	if got.Status != PostTransformed || got.FilePath != "/tmp/x.mp4" {
		t.Errorf("transformed sai: %+v", got)
	}

	by, err := s.ListPostsByStatus(PostTransformed, 10)
	if err != nil || len(by) != 1 {
		t.Fatalf("ListPostsByStatus: %v %d", err, len(by))
	}

	if err := s.SetPostTarget(p.ID, "kenh1", "tiktok"); err != nil {
		t.Fatalf("SetPostTarget: %v", err)
	}
	if err := s.MarkPostPosted(p.ID, "tiktok", "remote9", "https://tiktok/x"); err != nil {
		t.Fatalf("MarkPostPosted: %v", err)
	}
	got, _ = s.GetPost(p.ID)
	if got.Status != PostPosted || got.RemoteID != "remote9" || got.PostedAt == "" {
		t.Errorf("posted sai: %+v", got)
	}
	if got.StatusLabel() != "đã đăng" {
		t.Errorf("StatusLabel=%q", got.StatusLabel())
	}

	posted, err := s.ListPostedPosts(10)
	if err != nil || len(posted) != 1 {
		t.Fatalf("ListPostedPosts: %v %d", err, len(posted))
	}

	if err := s.SetPostMetrics(p.ID, 0); err != nil {
		t.Fatalf("SetPostMetrics: %v", err)
	}
	got, _ = s.GetPost(p.ID)
	if got.Views != 0 || got.MetricsAt == "" {
		t.Errorf("metrics sai: %+v", got)
	}
	if got.ViewsLabel() != "0" {
		t.Errorf("ViewsLabel=%q", got.ViewsLabel())
	}
	if err := s.SetPostMetrics(p.ID, -5); err == nil {
		t.Errorf("view âm phải lỗi")
	}

	// Level lạ → về mức 1.
	p2, err := s.CreatePost([]int64{5}, 99)
	if err != nil {
		t.Fatalf("CreatePost level lạ: %v", err)
	}
	if p2.TransformLevel != Level1 {
		t.Errorf("level lạ phải về 1, được %d", p2.TransformLevel)
	}
	if _, err := s.CreatePost(nil, Level1); err == nil {
		t.Errorf("tạo bài không video phải lỗi")
	}
}

func TestCountPostedSince(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreatePost([]int64{1}, Level1)
	_ = s.SetPostTarget(p.ID, "kenh1", "tiktok")
	_ = s.MarkPostPosted(p.ID, "tiktok", "r1", "")
	n, err := s.CountPostedSince("kenh1", "2000-01-01T00:00:00Z")
	if err != nil || n != 1 {
		t.Errorf("count=%d err=%v, muốn 1", n, err)
	}
	n, _ = s.CountPostedSince("kenh2", "2000-01-01T00:00:00Z")
	if n != 0 {
		t.Errorf("kênh khác phải 0, được %d", n)
	}
	n, _ = s.CountPostedSince("kenh1", "2999-01-01T00:00:00Z")
	if n != 0 {
		t.Errorf("mốc tương lai phải 0, được %d", n)
	}
}

func TestVideosNeedingTransform(t *testing.T) {
	s := newTestStore(t)
	v, err := s.QueueVideo(Video{DouyinID: "n1", URL: "https://x", Title: "N"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDownloaded(v.ID, "/tmp/n1.mp4", "sha-n1", "ytdlp", false, 5, "720x1280", "size-only"); err != nil {
		t.Fatal(err)
	}
	need, err := s.VideosNeedingTransform(10)
	if err != nil || len(need) != 1 {
		t.Fatalf("phải có 1 video chờ transform: %v %d", err, len(need))
	}
	// Tạo bài → không còn chờ.
	p, err := s.CreatePost([]int64{v.ID}, Level1)
	if err != nil {
		t.Fatal(err)
	}
	if need, _ := s.VideosNeedingTransform(10); len(need) != 0 {
		t.Fatalf("đã có bài thì không chờ nữa, còn %d", len(need))
	}
	// Bài failed → được làm lại.
	if err := s.SetPostStatus(p.ID, PostFailed, "lỗi test"); err != nil {
		t.Fatal(err)
	}
	if need, _ := s.VideosNeedingTransform(10); len(need) != 1 {
		t.Fatalf("bài failed phải được làm lại, còn %d", len(need))
	}
}

func TestVideosForCompilation(t *testing.T) {
	s := newTestStore(t)
	src, err := s.AddSource("user", "than_tien", "Thần Tiên")
	if err != nil {
		t.Fatal(err)
	}
	for i, plays := range []int64{100, 500, 300} {
		v, _ := s.QueueVideo(Video{DouyinID: string(rune('a' + i)), SourceID: src.ID, PlayCount: plays})
		_ = s.MarkDownloaded(v.ID, "/tmp/x.mp4", "sha"+string(rune('a'+i)), "ytdlp", false, 5, "", "size-only")
	}
	vs, err := s.VideosForCompilation(src.ID, 3)
	if err != nil || len(vs) != 3 {
		t.Fatalf("VideosForCompilation: %v %d", err, len(vs))
	}
	// Sắp xếp theo play_count giảm dần.
	if vs[0].PlayCount != 500 || vs[1].PlayCount != 300 || vs[2].PlayCount != 100 {
		t.Errorf("thứ tự play_count sai: %d %d %d", vs[0].PlayCount, vs[1].PlayCount, vs[2].PlayCount)
	}
}

func TestLivePostForVideo(t *testing.T) {
	s := newTestStore(t)
	// Chưa có bài → nil.
	live, err := s.LivePostForVideo(7)
	if err != nil || live != nil {
		t.Fatalf("chưa có bài phải nil, được %+v, err=%v", live, err)
	}
	p, err := s.CreatePost([]int64{7, 8}, Level1)
	if err != nil {
		t.Fatal(err)
	}
	// Bài pending → tìm thấy cho cả 2 video.
	for _, vid := range []int64{7, 8} {
		live, err = s.LivePostForVideo(vid)
		if err != nil || live == nil || live.ID != p.ID {
			t.Fatalf("video %d phải có bài #%d, được %+v, err=%v", vid, p.ID, live, err)
		}
	}
	// Bài failed → coi như không có (được transform lại).
	if err := s.SetPostStatus(p.ID, PostFailed, "lỗi test"); err != nil {
		t.Fatal(err)
	}
	live, err = s.LivePostForVideo(7)
	if err != nil || live != nil {
		t.Fatalf("bài failed phải nil, được %+v, err=%v", live, err)
	}
	// Bài mới nhất thắng khi có nhiều bài.
	p2, err := s.CreatePost([]int64{7}, Level2)
	if err != nil {
		t.Fatal(err)
	}
	live, err = s.LivePostForVideo(7)
	if err != nil || live == nil || live.ID != p2.ID {
		t.Fatalf("phải trả bài mới nhất #%d, được %+v, err=%v", p2.ID, live, err)
	}
}
