# Tài liệu tính năng — AI Creator OS

Mỗi file mô tả **chính xác theo logic thật trong code** (đọc từ source, không bịa):
mục đích → cách kích hoạt (bấm gì / tự động khi nào) → luồng vận hành từng bước
(file, hàm, bảng SQLite, engine/API) → fail-closed & an toàn → key rotation (nếu dùng key cloud).

## Mục lục

| # | Tính năng | File |
|---|-----------|------|
| 1 | Trang chủ (`/`) — thẻ số liệu, feed quyết định, chip công tắc chính | [trang-chu.md](trang-chu.md) |
| 2 | Tài khoản — 3 tab Tổng quan / Autopilot / Kết nối | [tai-khoan.md](tai-khoan.md) |
| 3 | Lịch live (`/schedule`) | [lich-live.md](lich-live.md) |
| 4 | Phát triển kênh (`/growth`) — plan 30 ngày, ngưỡng, sản xuất | [phat-trien-kenh.md](phat-trien-kenh.md) |
| 5 | Studio AI — Video chữ động / Phim / Affiliate / Jobs / Trends (+ sơ đồ pipeline phim) | [studio-ai.md](studio-ai.md) |
| 6 | Agent Team (`/team`) — cây trạng thái suy ra từ job | [agent-team.md](agent-team.md) |
| 7 | Sản phẩm — Kệ hàng, tìm kiếm, lịch autopilot affiliate | [san-pham.md](san-pham.md) |
| 8 | Đa nền tảng (`/publishers`) — TikTok/YouTube/Facebook/RTMP | [da-nen-tang.md](da-nen-tang.md) |
| 9 | Cài đặt — 5 trang con (Hệ thống / Nhà cung cấp / Model local / Nhân vật / An toàn) | [cai-dat.md](cai-dat.md) |
| 10 | Wizard lần đầu (`/onboard`) | [wizard-lan-dau.md](wizard-lan-dau.md) |
| 11 | Sao lưu & khôi phục | [sao-luu-khoi-phuc.md](sao-luu-khoi-phuc.md) |
| 12 | API nội bộ (`/api/*`) — mặc định TẮT | [api-noi-bo.md](api-noi-bo.md) |
| 13 | Daemon tự động (`internal/automation`) — GrowthTick / SyncOneAccount / ReconcileCommissions / AutopilotTick / AutoPublish | [daemon-tu-dong.md](daemon-tu-dong.md) |
| 14 | Kill switch + Dry-run + MASTER_SWITCH | [kill-switch-dry-run-master.md](kill-switch-dry-run-master.md) |
| 15 | Quy tắc dùng chung: key rotation & fail-closed | [key-rotation.md](key-rotation.md) |

## Nguồn sự thật duy nhất về cấu hình

Mọi cài đặt vận hành đi qua `automation.Settings` — hiện thực `automation.LedgerSettings`
đọc/ghi bảng `settings` trong `ledger.db` (`internal/automation/settings.go`).
`web.Config` không còn là control plane cho cài đặt UI (R2-W4); lưu env qua UI gọi
`(*Config).RefreshEnv()` để có hiệu lực ngay không cần restart.

## Quy ước fail-closed toàn app

- Không có nguồn dữ liệu thật → giữ số 0 + ghi decision log lý do, **không bịa số**.
- `Gate == nil` (chưa nối) → coi như dry-run (an toàn nhất).
- Kill switch thắng mọi tick; dry-run chặn mọi thao tác thật.
