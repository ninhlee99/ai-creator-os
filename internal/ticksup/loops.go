package ticksup

// Vòng lặp khởi động các automation tick (Đợt R).
//
// Tách khỏi cmd/aicos để file main giữ mỏng theo STANDARDS (≤700 dòng):
// main chỉ quyết định tick nào được arm (theo store đã wiring xong),
// còn cadence/timeout/dispatch nằm gọn ở đây.

import (
	"context"
	"log"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
)

// Deps là những gì ArmLoops cần: factory build Service mới mỗi lần gọi
// (lazy — luôn thấy đủ store đã wiring) và cờ tick nào được arm.
type Deps struct {
	NewService func() *automation.Service
	Autopilot  bool // studio + products sẵn sàng
	Growth     bool
	AT         bool
	Story      bool // studio sẵn sàng
}

// ArmLoops khởi động mọi vòng tick đã bật, mỗi tick đi qua Supervisor
// (timeout + recover + heartbeat + bỏ qua khi kẹt).
func ArmLoops(ctx context.Context, sup *Supervisor, d Deps) {
	if d.NewService == nil || sup == nil {
		return
	}
	// Đợt V: watchdog trong app — tick nào quá 3×nhịp mà chưa Dispatch thì
	// alert critical (deadlock/loop chết mà /healthz không phát hiện được).
	go sup.RunWatchdog(ctx, 5*time.Minute)
	if d.Autopilot {
		// Kiểm tra mỗi phút; chạy khi đủ autopilot_interval_hours (mặc định 6h).
		sup.Watch("autopilot", time.Minute, 15*time.Minute)
		go loop(ctx, time.Minute, func() {
			sup.Dispatch(ctx, "autopilot", 15*time.Minute, d.NewService().AutopilotTick)
		})
		log.Printf("autopilot: scheduler armed (managed in web UI /products)")
	}
	if d.Growth {
		sup.Watch("growth", 5*time.Minute, 20*time.Minute)
		go loop(ctx, 5*time.Minute, func() {
			auto := d.NewService()
			sup.Dispatch(ctx, "growth", 20*time.Minute,
				func(c context.Context) []string { return auto.GrowthTick(c, false) })
		})
		log.Printf("growth: automation tick armed (mặc định bật ở /growth; dry-run là cổng an toàn)")
	}
	if d.AT {
		sup.Watch("accesstrade", 5*time.Minute, 15*time.Minute)
		go loop(ctx, 5*time.Minute, func() {
			sup.Dispatch(ctx, "accesstrade", 15*time.Minute, d.NewService().ATTick)
		})
		log.Printf("accesstrade: automation tick armed (hunter daily, order sync 30m, campaign check daily)")
	}
	if d.Story {
		for _, n := range []string{"story", "selfheal", "backup", "story_public", "story_retry", "notify", "draft_digest"} {
			sup.Watch(n, 5*time.Minute, 30*time.Minute)
		}
		go loop(ctx, 5*time.Minute, func() {
			auto := d.NewService()
			sup.Dispatch(ctx, "story", 30*time.Minute, auto.StoryTick)
			// Đợt L: tự chữa lành (job treo + canh đĩa), 1 giờ/lần.
			sup.Dispatch(ctx, "selfheal", 10*time.Minute, auto.SelfHealTick)
			// Đợt M2: tự sao lưu data dir, 1 ngày/lần.
			sup.Dispatch(ctx, "backup", 10*time.Minute, auto.BackupTick)
			// Đợt N: tự chuyển public video private quá thời gian chờ.
			sup.Dispatch(ctx, "story_public", 10*time.Minute, auto.StoryPublicTick)
			// Đợt P: tự thử lại job story lỗi (backoff + giới hạn).
			sup.Dispatch(ctx, "story_retry", 10*time.Minute, auto.StoryRetryTick)
			// Đợt O2: gửi alert mới qua Telegram (nếu Ninh đã bật).
			sup.Dispatch(ctx, "notify", 5*time.Minute, auto.NotifyTick)
			// Digest "bản nháp chờ đăng" 1 lần/ngày → Telegram (Ninh bấm Đăng tay,
			// không dùng Playwright tự đăng để tránh ban kênh).
			sup.Dispatch(ctx, "draft_digest", 30*time.Minute, auto.DraftDigestTick)
		})
		log.Printf("story: automation tick armed (tạo truyện 24 giờ/lần, tự đăng private-first, self-heal 1 giờ/lần)")
	}
}

// loop chạy fn mỗi interval cho tới khi ctx hủy.
func loop(ctx context.Context, interval time.Duration, fn func()) {
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				fn()
			}
		}
	}()
}
