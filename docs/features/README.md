# Tài liệu tính năng — AI Creator OS

Mỗi file mô tả **chính xác theo logic thật trong code** (đọc từ source, không bịa):
mục đích → cách kích hoạt (bấm gì / tự động khi nào) → luồng vận hành từng bước
(file, hàm, bảng SQLite, engine/API) → fail-closed & an toàn → key rotation (nếu dùng key cloud).

> **PIVOT 2026-10-02:** live + phim điện ảnh đã park (build tag `parked`).
> File nào ghi 🅿️ PARKED thì tính năng đó **không còn trong binary** — chỉ còn
> code nguồn + test chạy bằng `go test -tags parked ./...`.

## Mục lục (sidebar hiện tại: 8 mục)

| # | Tính năng | File | Trạng thái |
|---|---|---|---|
| 1 | Trang chủ (`/`) — thẻ số liệu, feed quyết định, chip công tắc chính | [trang-chu.md](trang-chu.md) | ✅ |
| 2 | Kênh (`/accounts`, sidebar "Kênh") — 3 tab Tổng quan / Autopilot / Kết nối | [tai-khoan.md](tai-khoan.md) | ✅ |
| 3 | Phát triển kênh (`/growth`) — plan 30 ngày, ngưỡng, sản xuất | [phat-trien-kenh.md](phat-trien-kenh.md) | ✅ |
| 4 | Studio AI (`/studio`) — Video affiliate / Video chữ động / Jobs / Trends | [studio-ai.md](studio-ai.md) | ✅ (phim đã park) |
| 5 | Affiliate (`/products`) — kho sản phẩm, kệ hàng, lịch autopilot | [san-pham.md](san-pham.md) | ✅ (Accesstrade: đợt B/C) |
| 5b | **Reup Douyin (`/reup`) — nguồn, yt-dlp sidecar, hàng đợi tải** | [reup.md](reup.md) | ✅ (Đợt D; transform/đăng ở Đợt E) |
| 6 | Đa nền tảng (`/publishers`) — TikTok/YouTube/Facebook/RTMP | [da-nen-tang.md](da-nen-tang.md) | ✅ |
| 7 | Cài đặt — 4 trang con (Hệ thống / Nhà cung cấp / Model local / An toàn) | [cai-dat.md](cai-dat.md) | ✅ |
| 8 | Lịch live (`/schedule`) | [lich-live.md](lich-live.md) | 🅿️ PARKED |
| 9 | Agent Team (`/team`) | [agent-team.md](agent-team.md) | 🅿️ PARKED (sẽ định nghĩa lại) |
| 10 | Wizard lần đầu (`/onboard`) | [wizard-lan-dau.md](wizard-lan-dau.md) | ✅ |
| 11 | Sao lưu & khôi phục | [sao-luu-khoi-phuc.md](sao-luu-khoi-phuc.md) | ✅ |
| 12 | API nội bộ (`/api/*`) — mặc định TẮT | [api-noi-bo.md](api-noi-bo.md) | ✅ |
| 13 | Daemon tự động (`internal/automation`) — AutopilotTick / GrowthTick / SyncOneAccount / ReconcileCommissions | [daemon-tu-dong.md](daemon-tu-dong.md) | ✅ |
| 14 | Kill switch + Dry-run + MASTER_SWITCH | [kill-switch-dry-run-master.md](kill-switch-dry-run-master.md) | ✅ |
| 15 | Quy tắc dùng chung: key rotation & fail-closed | [key-rotation.md](key-rotation.md) | ✅ |

## Nguồn sự thật duy nhất về cấu hình

Mọi cài đặt vận hành đi qua `automation.Settings` — hiện thực `automation.LedgerSettings`
đọc/ghi bảng `settings` trong `ledger.db` (`internal/automation/settings.go`).
Lưu env qua UI gọi `(*Config).RefreshEnv()` để có hiệu lực ngay không cần restart.

## Quy ước fail-closed toàn app

- Không có nguồn dữ liệu thật → giữ số 0 + ghi decision log lý do, **không bịa số**.
- `Gate == nil` (chưa nối) → coi như dry-run (an toàn nhất).
- Kill switch thắng mọi tick; dry-run chặn mọi thao tác thật.
