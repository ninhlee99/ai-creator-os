# Tài liệu tính năng — MiuNova

Các file có trạng thái ✅/⚠ mô tả **logic hiện có trong code** (đọc từ source,
không bịa):
mục đích → cách kích hoạt (bấm gì / tự động khi nào) → luồng vận hành từng bước
(file, hàm, bảng SQLite, engine/API) → fail-closed & an toàn → key rotation (nếu dùng key cloud).

> **Trọng tâm code hiện tại:** các năng lực của MiuNova gồm Affiliate Accesstrade,
> viral source discovery, YouTube Kể Chuyện và governance; tự động sản xuất video
> affiliate/viral đang giới hạn theo chính sách free-only, tích hợp Bản sắc thương hiệu (Brand DNA),
> Quản trị cơ hội xu hướng (Trend Intelligence) và Bộ điều phối nhiệm vụ tự chủ (AI Mission Control).

Đọc [IMPLEMENTATION_STATUS](../IMPLEMENTATION_STATUS.md) trước khi dùng tài liệu
này làm cam kết.

## Mục lục tính năng (19 năng lực as-built)

| # | Tính năng | File | Trạng thái |
|---|---|---|---|
| 1 | Trang chủ (`/`) — thẻ số liệu, feed quyết định, chip công tắc chính | [dashboard.md](dashboard.md) | ✅ |
| 2 | **Bộ Não Tự Hành Cấp Cao (`/harness`) — Master Agent Harness, OODA Cognitive Engine, Episodic Memory** | [harness-agent.md](harness-agent.md) | ✅ |
| 3 | Kênh (`/accounts`, sidebar "Kênh") — 4 tab Tổng quan / Autopilot / Bản sắc / Kết nối | [accounts.md](accounts.md) | ✅ |
| 4 | **Bản sắc thương hiệu & Mission Control (`/accounts/{id}?tab=brand-dna`, `/missions`) — Brand DNA, Trend Intelligence, Mission Engine** | [brand-dna-mission-control.md](brand-dna-mission-control.md) | ✅ |
| 5 | Phát triển kênh (`/growth`) — plan 30 ngày, ngưỡng, sản xuất | [channel-growth.md](channel-growth.md) | ✅ |
| 6 | Đội ngũ (`/team`) — sức khỏe 3 pipeline từ dữ liệu thật | [agent-team.md](agent-team.md) | ✅ |
| 7 | Studio AI (`/studio`) — affiliate / Muse-Flow creation / Jobs / Trends | [studio-ai.md](studio-ai.md) | ✅ |
| 8 | Affiliate (`/products`) — kho sản phẩm, kệ hàng, lịch autopilot | [products.md](products.md) | ✅ |
| 9 | Viral (`/viral`) — radar nguồn hoạt động; thay nhân vật tạm dừng vì editor video cần billing | [video-viral.md](video-viral.md) | ⚠ |
| 10 | Kể chuyện (`/stories`) — truyện ngôi thứ nhất → ảnh 16:9 → TTS → dựng 16:9 | [storytelling.md](storytelling.md) | ✅ |
| 11 | Đa nền tảng (`/publishers`) — TikTok / YouTube / Facebook OAuth | [multi-platform.md](multi-platform.md) | ✅ |
| 12 | **Kho Tài Khoản Web AI (`/web-ai`) — account backup, Muse/Flow, account pool** | [web-ai.md](web-ai.md) | ⚠ |
| 13 | Cài đặt — 5 trang (Hệ thống / Accesstrade / Nhà cung cấp / Model local / An toàn) | [settings.md](settings.md) | ✅ |
| 14 | Wizard lần đầu (`/onboard`) | [onboarding-wizard.md](onboarding-wizard.md) | ✅ |
| 15 | Sao lưu & khôi phục | [backup-restore.md](backup-restore.md) | ✅ |
| 16 | API nội bộ (`/api/*`) — mặc định TẮT | [internal-api.md](internal-api.md) | ✅ |
| 17 | Daemon tự động (`internal/automation`) — AutopilotTick / GrowthTick / SyncOneAccount / ReconcileCommissions | [autonomous-daemon.md](autonomous-daemon.md) | ✅ |
| 18 | Kill switch + Dry-run + MASTER_SWITCH | [kill-switch-dry-run-master.md](kill-switch-dry-run-master.md) | ✅ |
| 19 | Quy tắc dùng chung: key rotation & fail-closed | [key-rotation.md](key-rotation.md) | ✅ |

## Nguồn sự thật duy nhất về cấu hình

Mọi cài đặt vận hành đi qua `automation.Settings` — hiện thực `automation.LedgerSettings`
đọc/ghi bảng `settings` trong `ledger.db` (`internal/automation/settings.go`).
Lưu env qua UI gọi `(*Config).RefreshEnv()` để có hiệu lực ngay không cần restart.

## Quy ước fail-closed toàn app

- Một số luồng as-built có thể lưu giá trị số 0 kèm decision log khi thiếu nguồn;
  đích sản phẩm phải phân biệt missing/unknown với zero theo PRODUCT_SPEC.
- `Gate == nil` (chưa nối) → coi như dry-run (an toàn nhất).
- Kill switch thắng mọi tick; dry-run chặn mọi thao tác thật.
