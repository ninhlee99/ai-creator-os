package automation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// ---------------------------------------------------------------------------
// Reup automation tick (Đợt D) — zero-touch, Ninh không chạm.
//
// ReupDiscoverTick: 6 giờ/lần — discover video viral từ nguồn Douyin đang
// bật (TikWM metadata, auto-pick theo play_count) → tải ngay qua
// Downloader (yt-dlp → TikWM fallback, dedupe sha256, QC).
//
// Cùng cổng an toàn như các tick khác: kill switch + DRY-RUN chặn;
// store chưa mở → bỏ qua im lặng; công tắc riêng mặc định BẬT
// (unset = bật — tiền lệ Đợt 3). Transform/đăng/kill rule ở
// reup_transform.go (Đợt E), cùng goroutine reup trong main.go.
// ---------------------------------------------------------------------------

// Setting keys cho reup automation (ledger settings).
const (
	KeyReupDiscoverEnabled   = "reup.discover_enabled"
	KeyReupDiscoverIntervalH = "reup.discover_interval_hours"
	KeyReupDiscoverLastRun   = "reup.discover_last_run"
	KeyReupVideosPerSource   = "reup.videos_per_source"
	defaultReupIntervalHours = 6
	defaultReupVideosPerSrc  = 3
)

// ReupScanResult là kết quả một pass discover + download.
type ReupScanResult struct {
	Found  int
	OK     int
	Dup    int
	Failed int
	Notes  []string
}

// RunReupScan chạy 1 pass discover + download (dùng chung cho tick và nút
// "Quét ngay" trên UI — không copy logic).
func (s *Service) RunReupScan(ctx context.Context) ReupScanResult {
	var res ReupScanResult
	if s.Reup == nil {
		res.Notes = []string{"kho reup chưa sẵn sàng"}
		return res
	}
	yt := reup.NewManager(s.ReupBinDir)
	if s.ReupYtDlpRelease != "" {
		yt.ReleaseBase = s.ReupYtDlpRelease // test hook
	}
	tw := reup.NewTikWM()
	if s.ReupTikWMBaseURL != "" {
		tw.BaseURL = s.ReupTikWMBaseURL // test hook
	}
	disc := reup.NewDiscoverer(s.Reup, tw)
	perSource := atInt(s.Settings, KeyReupVideosPerSource, defaultReupVideosPerSrc)
	cands, notes := disc.Discover(ctx, perSource)
	res.Found = len(cands)
	res.Notes = notes
	dl := reup.NewDownloader(yt, tw, s.Reup, s.ReupWorkDir)
	for _, c := range cands {
		v, err := dl.DownloadCandidate(ctx, c)
		if err != nil {
			res.Failed++
			log.Printf("reup: tải %s thất bại: %v", c.DouyinID, err)
			continue
		}
		switch {
		case v.Status == reup.StatusDownloaded:
			res.OK++
		case strings.Contains(v.FailReason, "trùng nội dung"):
			res.Dup++
		default:
			res.Failed++
		}
	}
	return res
}

// ReupTick chạy pass discover + download đến hạn. main.go gọi mỗi 5 phút
// (cùng vòng với ATTick).
func (s *Service) ReupTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyReupDiscoverEnabled, true) {
		return nil
	}
	if s.Reup == nil {
		return nil // kho chưa mở — bỏ qua im lặng như tick AT
	}
	interval := time.Duration(atInt(s.Settings, KeyReupDiscoverIntervalH,
		defaultReupIntervalHours)) * time.Hour
	if interval <= 0 {
		interval = defaultReupIntervalHours * time.Hour
	}
	if !dueSince(s.Settings, KeyReupDiscoverLastRun, interval) {
		return nil
	}
	res := s.RunReupScan(ctx)
	stampRun(s.Settings, KeyReupDiscoverLastRun)
	out := []string{fmt.Sprintf("reup discover: %d video mới từ nguồn → %d đã tải, %d trùng, %d lỗi",
		res.Found, res.OK, res.Dup, res.Failed)}
	return append(out, res.Notes...)
}
