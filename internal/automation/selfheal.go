package automation

// Đợt L — tự chữa lành (self-heal) cho máy chạy 24/7 không người trông.
// Chạy 1 giờ/lần từ story tick (luôn có khi Studio bật):
//  1. Quét job treo: "running" quá 12h mà app không restart (ffmpeg treo,
//     goroutine chết lặng) → failed + log rõ. Restart đã có
//     MarkInterruptedJobs trong main.go.
//  2. Canh đĩa: đầy quá ngưỡng → tự dọn theo thứ tự an toàn:
//     file tạm của job lỗi trước, video truyện đã đăng YouTube (cũ) sau.
//     Không bao giờ chạm job đang chạy/chờ hoặc video chưa đăng.
// Mọi hành động đều ghi alert + note để Ninh thấy trên dashboard.

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

const (
	KeySelfHealLastRun = "selfheal.last_run"
	KeyDiskMaxPct      = "system.disk_max_pct"    // mặc định 85
	KeyDiskPruneDays   = "system.disk_prune_days" // mặc định 30
)

const (
	defaultDiskMaxPct    = 85
	defaultDiskPruneDays = 30
	selfHealInterval     = time.Hour
	hungJobMaxAge        = 12 * time.Hour
)

// hungSweeper + workPruner: khả năng tự chữa của Studio, soft-assert để
// automation không phụ thuộc cứng vào *studio.Studio.
type hungSweeper interface {
	SweepHungJobs(maxAge time.Duration) int
}

type workPruner interface {
	PruneFailedWork() (int, int64)
	PruneOldPublishedOutputs(maxAge time.Duration, isPublished func(id string) bool) (int, int64)
}

// SelfHealTick chạy 1 giờ/lần. Tôn trọng kill switch + dry-run như mọi tick.
func (s *Service) SelfHealTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Story == nil {
		return nil
	}
	if !dueSince(s.Settings, KeySelfHealLastRun, selfHealInterval) {
		return nil
	}
	defer stampRun(s.Settings, KeySelfHealLastRun)

	var notes []string
	if sw, ok := s.Story.(hungSweeper); ok && sw != nil {
		if n := sw.SweepHungJobs(hungJobMaxAge); n > 0 {
			msg := fmt.Sprintf("%d job treo quá 12h → đánh dấu thất bại (log chi tiết trong từng job)", n)
			notes = append(notes, "tự chữa: "+msg)
			s.alert("selfheal_hung", "warn", msg,
				"Đã đánh dấu thất bại để pipeline không kẹt. Tạo lại thủ công từ trang Kể chuyện/Studio nếu cần.")
		}
	}
	notes = append(notes, s.diskGuard()...)
	return notes
}

// diskGuard: đĩa đầy quá ngưỡng → dọn theo thứ tự an toàn, báo alert.
func (s *Service) diskGuard() []string {
	if s.DataDir == "" {
		return nil
	}
	pct, err := DiskUsagePct(s.DataDir)
	if err != nil {
		return nil // không đo được thì im lặng, không spam
	}
	maxPct := atInt(s.Settings, KeyDiskMaxPct, defaultDiskMaxPct)
	if pct < maxPct {
		return nil
	}
	pr, ok := s.Story.(workPruner)
	if !ok || pr == nil {
		msg := fmt.Sprintf("đĩa %d%% đầy (ngưỡng %d%%) — cần Ninh dọn tay", pct, maxPct)
		s.alert("disk_guard", "warn", msg, "Chưa dọn tự động được. Xoá bớt video cũ trong thư mục dữ liệu.")
		return []string{"tự chữa đĩa: " + msg}
	}

	fj, freed1 := pr.PruneFailedWork()
	pruneDays := atInt(s.Settings, KeyDiskPruneDays, defaultDiskPruneDays)
	isPub := func(id string) bool {
		_, done := s.Settings.Get(KeyStoryPublishedPrefix + id)
		return done
	}
	po, freed2 := pr.PruneOldPublishedOutputs(time.Duration(pruneDays)*24*time.Hour, isPub)
	freed := freed1 + freed2

	msg := fmt.Sprintf("đĩa %d%% đầy (ngưỡng %d%%): đã dọn %d job lỗi + %d video đã đăng cũ hơn %d ngày, giải phóng %s",
		pct, maxPct, fj, po, pruneDays, formatBytes(freed))
	action := "Đã dọn file tạm của job lỗi và file local của video đã đăng YouTube (bản đăng vẫn còn trên YouTube)."

	if pct2, err := DiskUsagePct(s.DataDir); err == nil && pct2 >= maxPct {
		msg += fmt.Sprintf(" — vẫn %d%%, Ninh cần dọn tay hoặc thêm dung lượng", pct2)
		action += " Đĩa vẫn đầy sau khi dọn: cần can thiệp tay."
	}
	s.alert("disk_guard", "warn", msg, action)
	return []string{"tự chữa đĩa: " + msg}
}

// alert ghi growth alert nếu Growth khả dụng; im lặng nếu không.
func (s *Service) alert(kind, severity, msg, action string) {
	if s.Growth == nil {
		return
	}
	_ = s.Growth.InsertAlert(growth.Alert{
		Severity:    severity,
		Kind:        kind,
		Message:     msg,
		ActionTaken: action,
	})
}

// DiskUsagePct trả về % đĩa đã dùng tại path. Pure Go (syscall.Statfs),
// chạy được trên linux/darwin, không cgo.
func DiskUsagePct(path string) (int, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return 0, fmt.Errorf("không đo được dung lượng đĩa")
	}
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	return int(used * 100 / total), nil
}

// formatBytes định dạng bytes cho người đọc (tiếng Việt, ngắn gọn).
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
