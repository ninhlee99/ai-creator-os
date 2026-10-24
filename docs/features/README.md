# Tài liệu tính năng — MiuNova

Các file có trạng thái ✅/⚠ mô tả **logic hiện có trong code** (đọc từ source,
không bịa):
mục đích → cách kích hoạt (bấm gì / tự động khi nào) → luồng vận hành từng bước
(file, hàm, bảng SQLite, engine/API) → fail-closed & an toàn → key rotation (nếu dùng key cloud).

> **Trọng tâm code hiện tại:** Accesstrade discovery/orders, viral source discovery,
> YouTube Story và governance có các luồng hoạt động riêng. Affiliate Autopilot
> dừng trước discovery; viral character replacement đang tắt. Harness, Brand DNA
> và Mission Engine có giới hạn, chưa phải production controller.

Đọc [IMPLEMENTATION_STATUS](../IMPLEMENTATION_STATUS.md) trước khi dùng tài liệu
này làm cam kết.

## Mục lục tính năng (20 năng lực as-built)

| # | Tính năng | File | Trạng thái |
|---|---|---|---|
| 1 | Trang chủ (`/`) — job, commission đã sync, công tắc lịch, cấu hình nền tảng, tick/quota; kết quả publish và blocker theo pipeline còn thiếu | [dashboard.md](dashboard.md) | ⚠ |
| 2 | Harness (`/harness`) — OODA quan sát/read-only, bài học do người vận hành lưu; chưa có production action | [harness-agent.md](harness-agent.md) | ⚠ |
| 3 | Kênh (`/accounts`, sidebar "Kênh") — hồ sơ / Brand DNA / kết nối; Autopilot config lưu được nhưng producer tạm dừng | [accounts.md](accounts.md) | ⚠ |
| 4 | Brand DNA & Mission Engine (`/accounts/{id}?tab=brand-dna`, `/team`) — hồ sơ có giới hạn, phân loại mission theo từ khóa; chưa có QC gate hoặc tiến độ live | [brand-dna-mission-control.md](brand-dna-mission-control.md) | ⚠ |
| 5 | Phát triển kênh (`/growth`) — plan 30 ngày, metrics/thresholds; persona áp dụng variant, TikTok có tải MP4 thủ công nhưng chưa tự tải nháp | [channel-growth.md](channel-growth.md) | ⚠ |
| 6 | Đội ngũ (`/team`) — sức khỏe 3 pipeline từ dữ liệu thật | [agent-team.md](agent-team.md) | ✅ |
| 7 | Studio AI (`/studio`) — affiliate / Muse-Flow creation / Jobs / Trends; Autopilot producer paused | [studio-ai.md](studio-ai.md) | ⚠ |
| 8 | Affiliate (`/products`) — discovery/catalog, kệ Bio, AccessTrade links/orders; demand evidence và video autopilot chưa đạt | [products.md](products.md) | ⚠ |
| 9 | Viral (`/viral`) — YouTube source radar/storyboard; Flow character edit cần provider, QC và xác nhận credits | [video-viral.md](video-viral.md) | ⚠ |
| 10 | Kể chuyện (`/stories`) — form có; job mới bị chặn bởi gate ảnh tham chiếu; Story mặc định 9:16, Growth có variant riêng; timeline dài có thể lặp cảnh/đệm audio | [storytelling.md](storytelling.md) | ⚠ |
| 11 | Đa nền tảng (`/publishers`) — OAuth/adapters có; quyền live và một số publish flow chưa xác minh/hoàn tất | [multi-platform.md](multi-platform.md) | ⚠ |
| 12 | **Kho Tài Khoản Web AI (`/web-ai`) — account backup, Muse/Flow, account pool** | [web-ai.md](web-ai.md) | ⚠ |
| 13 | Cài đặt — 5 trang (Hệ thống / Accesstrade / Nhà cung cấp / Model local / An toàn) | [settings.md](settings.md) | ✅ |
| 14 | Wizard lần đầu (`/onboard`) | [onboarding-wizard.md](onboarding-wizard.md) | ✅ |
| 15 | Sao lưu & khôi phục | [backup-restore.md](backup-restore.md) | ✅ |
| 16 | API nội bộ (`/api/*`) — mặc định TẮT | [internal-api.md](internal-api.md) | ✅ |
| 17 | Daemon tự động (`internal/automation`) — tick scheduler có; affiliate Autopilot paused, TikTok chưa tự tải nháp | [autonomous-daemon.md](autonomous-daemon.md) | ⚠ |
| 18 | Kill switch + Dry-run + MASTER_SWITCH | [kill-switch-dry-run-master.md](kill-switch-dry-run-master.md) | ✅ |
| 19 | Quy tắc dùng chung: key rotation & fail-closed | [key-rotation.md](key-rotation.md) | ✅ |
| 20 | Bio storefront (`/bio`, `/bio-admin`) — listing và shop redirect có; try-on là ảnh ước lượng, không xác nhận fit hay đơn hàng | [bio-storefront-tryon.md](bio-storefront-tryon.md) | ⚠ |

## Nguồn sự thật duy nhất về cấu hình

Mọi cài đặt vận hành đi qua `automation.Settings` — hiện thực `automation.LedgerSettings`
đọc/ghi bảng `settings` trong `ledger.db` (`internal/automation/settings.go`).
Lưu env qua UI gọi `(*Config).RefreshEnv()` để có hiệu lực ngay không cần restart.

## Quy ước fail-closed toàn app

- Một số luồng as-built có thể lưu giá trị số 0 kèm decision log khi thiếu nguồn;
  đích sản phẩm phải phân biệt missing/unknown với zero theo PRODUCT_SPEC.
- `Gate == nil` trong `automation.Service` → production ticks fail-closed như dry-run.
- Kill switch/dry-run áp dụng theo workflow; không phải blanket gate cho mọi UI,
  sync hoặc discovery. `MASTER_SWITCH` chỉ điều khiển network daemon.
