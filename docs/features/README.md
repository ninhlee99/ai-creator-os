# Tài liệu tính năng — AI Creator OS

Mỗi file mô tả **chính xác theo logic thật trong code** (đọc từ source, không bịa):
mục đích → cách kích hoạt (bấm gì / tự động khi nào) → luồng vận hành từng bước
(file, hàm, bảng SQLite, engine/API) → fail-closed & an toàn → key rotation (nếu dùng key cloud).

> **Trọng tâm sản phẩm:** Hệ điều hành tăng trưởng tự động (AI Creator OS) với 3 pipeline
> sản xuất chính (Affiliate Accesstrade, Video Viral AI 100% Gốc, YouTube Kể Chuyện), tích hợp Bản sắc thương hiệu (Brand DNA),
> Quản trị cơ hội xu hướng (Trend Intelligence) và Bộ điều phối nhiệm vụ tự chủ (AI Mission Control).

## Mục lục tính năng (18 tài liệu)

| # | Tính năng | File | Trạng thái |
|---|---|---|---|
| 1 | Trang chủ (`/`) — thẻ số liệu, feed quyết định, chip công tắc chính | [dashboard.md](dashboard.md) | ✅ |
| 2 | **Bộ Não Tự Hành Cấp Cao (`/harness`) — Master Agent Harness, OODA Cognitive Engine, Episodic Memory** | [harness-agent.md](harness-agent.md) | ✅ |
| 3 | Kênh (`/accounts`, sidebar "Kênh") — 4 tab Tổng quan / Autopilot / Bản sắc / Kết nối | [accounts.md](accounts.md) | ✅ |
| 4 | **Bản sắc thương hiệu & Mission Control (`/accounts/{id}?tab=brand-dna`, `/missions`) — Brand DNA, Trend Intelligence, Mission Engine** | [brand-dna-mission-control.md](brand-dna-mission-control.md) | ✅ |
| 5 | Phát triển kênh (`/growth`) — plan 30 ngày, ngưỡng, sản xuất | [channel-growth.md](channel-growth.md) | ✅ |
| 6 | Đội ngũ (`/team`) — sức khỏe 3 pipeline từ dữ liệu thật | [agent-team.md](agent-team.md) | ✅ |
| 7 | Studio AI (`/studio`) — Video affiliate / Video chữ động / Jobs / Trends | [studio-ai.md](studio-ai.md) | ✅ |
| 8 | Affiliate (`/products`) — kho sản phẩm, kệ hàng, lịch autopilot | [products.md](products.md) | ✅ |
| 9 | Video Viral AI (`/viral`) — kịch bản tâm lý học 3s, phân cảnh 9:16 MediaGen, âm nhạc trending, 100% gốc | [video-viral.md](video-viral.md) | ✅ |
| 10 | Kể chuyện (`/stories`) — truyện ngôi thứ nhất → ảnh 16:9 → TTS → dựng 16:9 | [storytelling.md](storytelling.md) | ✅ |
| 11 | Đa nền tảng (`/publishers`) — TikTok / YouTube / Facebook OAuth | [multi-platform.md](multi-platform.md) | ✅ |
| 12 | **Kho Tài Khoản Web AI (`/settings/system#tab-webai`) — CDP Đăng nhập tự động, cô lập phiên, xoay vòng ChatGPT/Claude/Gemini/Grok/Muse** | [web-ai.md](web-ai.md) | ✅ |
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

- Không có nguồn dữ liệu thật → giữ số 0 + ghi decision log lý do, **không bịa số**.
- `Gate == nil` (chưa nối) → coi như dry-run (an toàn nhất).
- Kill switch thắng mọi tick; dry-run chặn mọi thao tác thật.
