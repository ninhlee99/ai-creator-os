package automation

// Đợt L — tự chữa lành (self-heal) cho máy chạy 24/7 không người trông.
// Chạy 1 giờ/lần từ story tick (luôn có khi Studio bật):
//  1. Quét job treo: "running" quá 12h mà app không restart (ffmpeg treo,
//     goroutine chết lặng) → failed + log rõ. Restart đã có
//     MarkInterruptedJobs trong main.go.
//  2. Canh đĩa: đầy quá ngưỡng → tự dọn theo thứ tự an toàn:
//     file tạm của job lỗi. Không chạm job đang chạy/chờ hoặc output hoàn tất.
// Mọi hành động đều ghi alert + note để Ninh thấy trên dashboard.

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
)

const (
	KeySelfHealLastRun = "selfheal.last_run"
	KeyDiskMaxPct      = "system.disk_max_pct" // mặc định 85
)

const (
	defaultDiskMaxPct    = 85
	defaultDiskPruneDays = 30
	selfHealInterval     = time.Hour
	hungJobMaxAge        = 12 * time.Hour
)

// SelfHealTick chạy 1 giờ/lần. Tôn trọng kill switch + dry-run như mọi tick.
func (s *Service) SelfHealTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Maintenance == nil {
		return nil
	}
	if !dueSince(s.Settings, KeySelfHealLastRun, selfHealInterval) {
		return nil
	}
	defer stampRun(s.Settings, KeySelfHealLastRun)

	var notes []string
	if n := s.Maintenance.SweepHungJobs(hungJobMaxAge); n > 0 {
		msg := fmt.Sprintf("%d job treo quá 12h → đánh dấu thất bại (log chi tiết trong từng job)", n)
		notes = append(notes, "tự chữa: "+msg)
		s.alert("selfheal_hung", "warn", msg,
			"Đã đánh dấu thất bại để Studio tiếp tục xử lý các job khác.")
	}
	notes = append(notes, s.diskGuard()...)
	notes = append(notes, s.pruneTables()...)
	return notes
}

// pruneTables dọn các bảng phình theo thời gian (Đợt V — máy 24/7):
// decisions/usage 90 ngày, alert info 30 ngày (warn/critical 90 ngày),
// metric_snapshots/product_snapshots 90 ngày, job/workflow studio đã xong 60 ngày.
// Chạy trong SelfHealTick (1 giờ/lần).
func (s *Service) pruneTables() []string {
	var notes []string
	if s.Ledger != nil {
		if n, err := s.Ledger.PruneOld(90, 90); err != nil {
			notes = append(notes, fmt.Sprintf("dọn bảng: lỗi ledger — %v", err))
		} else if n > 0 {
			notes = append(notes, fmt.Sprintf("dọn bảng: xóa %d dòng decisions/usage cũ", n))
		}
	}
	if s.Growth != nil {
		if n, err := s.Growth.PruneOldAlerts(30, 90); err != nil {
			notes = append(notes, fmt.Sprintf("dọn bảng: lỗi alerts — %v", err))
		} else if n > 0 {
			notes = append(notes, fmt.Sprintf("dọn bảng: xóa %d alert cũ", n))
		}
		if n, err := s.Growth.PruneOldSnapshots(90); err != nil {
			notes = append(notes, fmt.Sprintf("dọn bảng: lỗi growth snapshots — %v", err))
		} else if n > 0 {
			notes = append(notes, fmt.Sprintf("dọn bảng: xóa %d metric snapshots cũ", n))
		}
	}
	if s.Products != nil {
		if n, err := s.Products.PruneOldSnapshots(90); err != nil {
			notes = append(notes, fmt.Sprintf("dọn bảng: lỗi product snapshots — %v", err))
		} else if n > 0 {
			notes = append(notes, fmt.Sprintf("dọn bảng: xóa %d product snapshots cũ", n))
		}
	}
	if n := s.Maintenance.PruneOldJobs(60 * 24 * time.Hour); n > 0 {
		notes = append(notes, fmt.Sprintf("dọn bảng: xóa %d job/workflow studio cũ (>60 ngày)", n))
	}
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
	if s.Maintenance == nil {
		msg := fmt.Sprintf("đĩa %d%% đầy (ngưỡng %d%%) — cần Ninh dọn tay", pct, maxPct)
		s.alert("disk_guard", "warn", msg, "Chưa dọn tự động được. Xoá bớt video cũ trong thư mục dữ liệu.")
		return []string{"tự chữa đĩa: " + msg}
	}

	fj, freed := s.Maintenance.PruneFailedWork()

	msg := fmt.Sprintf("đĩa %d%% đầy (ngưỡng %d%%): đã dọn file tạm của %d job lỗi, giải phóng %s",
		pct, maxPct, fj, formatBytes(freed))
	action := "Đã dọn file tạm của job lỗi; video hoàn tất được giữ nguyên."

	if pct2, err := DiskUsagePct(s.DataDir); err == nil && pct2 >= maxPct {
		msg += fmt.Sprintf(" — vẫn %d%%, Ninh cần dọn tay hoặc thêm dung lượng", pct2)
		action += " Đĩa vẫn đầy sau khi dọn: cần can thiệp tay."
	}
	s.alert("disk_guard", "warn", msg, action)
	// Đợt V: đĩa gần kẹt (>=95%) thì alert qua DB có thể INSERT lỗi im lặng
	// (SQLite đầy → ghi alert thất bại → Telegram không có gì để gửi). Gửi
	// thẳng Telegram + log file, không qua DB.
	if pct2, err := DiskUsagePct(s.DataDir); err == nil && pct2 >= 95 {
		s.alertCriticalDirect("disk_critical",
			fmt.Sprintf("🚨 ĐĨA GẦN KẸT: %d%% — app có thể ngừng ghi DB bất cứ lúc nào. Dọn đĩa ngay.", pct2))
	}
	return []string{"tự chữa đĩa: " + msg}
}

// alertCriticalDirect gửi cảnh báo nghiêm trọng KHÔNG qua database — dùng
// khi chính DB có thể không ghi được (đĩa đầy) hoặc cần chắc chắn tới tay
// Ninh. Ghi log file + thử Telegram trực tiếp, best-effort.
func (s *Service) alertCriticalDirect(kind, msg string) {
	log.Printf("[alert-direct] %s: %s", kind, msg)
	if s.Settings == nil || !atOn(s.Settings, KeyNotifyTelegramOn, false) {
		return
	}
	bot, _ := s.Settings.Get(KeyNotifyTelegramBot)
	chat, _ := s.Settings.Get(KeyNotifyTelegramChat)
	if bot == "" || chat == "" {
		return
	}
	if err := notify.SendTelegram(bot, chat, msg); err != nil {
		log.Printf("[alert-direct] telegram thất bại: %v", err)
	}
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

// RaiseAlert là bản public của alert — cho cmd/aicos gọi khi supervisor
// tick phát hiện panic/treo (Đợt R) hoặc ngắt mạch quota mở (Đợt S).
// Alert warn/critical sẽ tới Telegram của Ninh ở NotifyTick kế tiếp.
func (s *Service) RaiseAlert(kind, severity, msg, action string) {
	s.alert(kind, severity, msg, action)
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
