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
| 1 | Trang chủ (`/`) — thẻ số liệu, feed quyết định, chip công tắc chính | [trang-chu.md](trang-chu.md) | ✅ |
| 2 | Kênh (`/accounts`, sidebar "Kênh") — 4 tab Tổng quan / Autopilot / Bản sắc / Kết nối | [tai-khoan.md](tai-khoan.md) | ✅ |
| 3 | **Bản sắc thương hiệu & Mission Control (`/accounts/{id}?tab=brand-dna`, `/missions`) — Brand DNA, Trend Intelligence, Mission Engine** | [brand-dna-va-mission-control.md](brand-dna-va-mission-control.md) | ✅ |
| 4 | Phát triển kênh (`/growth`) — plan 30 ngày, ngưỡng, sản xuất | [phat-trien-kenh.md](phat-trien-kenh.md) | ✅ |
| 5 | Đội ngũ (`/team`) — sức khỏe 3 pipeline từ dữ liệu thật | [agent-team.md](agent-team.md) | ✅ |
| 6 | Studio AI (`/studio`) — Video affiliate / Video chữ động / Jobs / Trends | [studio-ai.md](studio-ai.md) | ✅ |
| 7 | Affiliate (`/products`) — kho sản phẩm, kệ hàng, lịch autopilot | [san-pham.md](san-pham.md) | ✅ |
| 8 | Video Viral AI (`/viral`) — kịch bản tâm lý học 3s, phân cảnh 9:16 MediaGen, âm nhạc trending, 100% gốc | [video-viral.md](video-viral.md) | ✅ |
| 9 | Kể chuyện (`/stories`) — truyện ngôi thứ nhất → ảnh 16:9 → TTS → dựng 16:9 | [ke-chuyen.md](ke-chuyen.md) | ✅ |
| 10 | Đa nền tảng (`/publishers`) — TikTok / YouTube / Facebook OAuth | [da-nen-tang.md](da-nen-tang.md) | ✅ |
| 11 | Cài đặt — 5 trang (Hệ thống / Accesstrade / Nhà cung cấp / Model local / An toàn) | [cai-dat.md](cai-dat.md) | ✅ |
| 12 | Wizard lần đầu (`/onboard`) | [wizard-lan-dau.md](wizard-lan-dau.md) | ✅ |
| 13 | Sao lưu & khôi phục | [sao-luu-khoi-phuc.md](sao-luu-khoi-phuc.md) | ✅ |
| 14 | API nội bộ (`/api/*`) — mặc định TẮT | [api-noi-bo.md](api-noi-bo.md) | ✅ |
| 15 | Daemon tự động (`internal/automation`) — AutopilotTick / GrowthTick / SyncOneAccount / ReconcileCommissions | [daemon-tu-dong.md](daemon-tu-dong.md) | ✅ |
| 16 | Kill switch + Dry-run + MASTER_SWITCH | [kill-switch-dry-run-master.md](kill-switch-dry-run-master.md) | ✅ |
| 17 | Quy tắc dùng chung: key rotation & fail-closed | [key-rotation.md](key-rotation.md) | ✅ |

## Nguồn sự thật duy nhất về cấu hình

Mọi cài đặt vận hành đi qua `automation.Settings` — hiện thực `automation.LedgerSettings`
đọc/ghi bảng `settings` trong `ledger.db` (`internal/automation/settings.go`).
Lưu env qua UI gọi `(*Config).RefreshEnv()` để có hiệu lực ngay không cần restart.

## Quy ước fail-closed toàn app

- Không có nguồn dữ liệu thật → giữ số 0 + ghi decision log lý do, **không bịa số**.
- `Gate == nil` (chưa nối) → coi như dry-run (an toàn nhất).
- Kill switch thắng mọi tick; dry-run chặn mọi thao tác thật.
