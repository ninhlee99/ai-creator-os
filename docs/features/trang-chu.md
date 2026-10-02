# Trang chủ (`/`)

## 1. Mục đích
Tổng quan một màn hình: thẻ số liệu (tiền, phiên live, lịch hôm nay), feed 8 quyết định
gần nhất, cảnh báo growth, trạng thái runtime local, và chip công tắc chính (MASTER_SWITCH).

## 2. Kích hoạt
- Người dùng mở `/` trên trình duyệt.
- `handleDashboard` (`internal/web/dashboard.go:11`) render template `dashboard.html`.

## 3. Luồng vận hành chi tiết
`handleDashboard` đọc và tính (không ghi gì ngoài việc đảm bảo lịch hôm nay):
1. `s.Mgr.List()` (`network.AccountManager`) → danh sách tài khoản; đếm `eligible`
   = tài khoản có `Status` là `live_ready` hoặc `live`.
2. Doanh thu: `SELECT COALESCE(SUM(commission),0) FROM orders` (bảng `orders` trong
   ledger.db — chỉ có số khi `ReconcileCommissions` ghi đơn thật, xem `daemon-tu-dong.md`).
3. `s.Ledger.TotalRevenue()` → hoa hồng đã ghi nhận (append-only).
4. `s.ensureTodaySchedule()` (`internal/web/schedule.go:70`) → nếu hôm nay chưa có slot
   nào **và** có tài khoản đủ điều kiện → tự build lịch một lần/ngày (không cần bấm nút).
5. `s.Ledger.GetSlots(today)` → các slot live hôm nay (bảng `live_slots`).
6. 8 quyết định mới nhất: `SELECT ... FROM decisions ORDER BY id DESC LIMIT 8`
   (bảng `decisions` — nhật ký mọi quyết định của hệ thống + con người).
7. `s.recentGrowthAlerts(accounts)` → cảnh báo growth chưa đọc.
8. `s.localRuntimeViews()` → trạng thái FFmpeg / llama-server / VieNeu / avatar sidecar
   (probe thật bằng `exec.LookPath`, xem `cai-dat.md`).
9. Chip công tắc chính: `automation.MasterOn(s.settings())` — đọc key `master.switch`
   trong ledger settings, nhãn trung thực "TẮT — daemon đang đứng yên" /
   "BẬT — daemon được phép chạy theo lịch".
10. Checklist "bắt đầu nhanh" khi chưa có tài khoản: `HasKeys` (có `GEMINI_API_KEYS`
    trong keyring DB hoặc env), `HasStudioJobs` (đã có job Studio nào chưa).

## 4. Fail-closed & an toàn
- Trang chủ chỉ đọc + build lịch; không thao tác thật nên không bị dry-run chặn.
- Lỗi đọc DB ở bước nào → `s.fail` trả lỗi 500 với tên bước, không render nửa trang.

## 5. API key rotation
Không dùng key cloud trực tiếp. `HasKeys` chỉ kiểm tra sự tồn tại của key
(keyring DB cho engine `tts`/`llm` loại `gemini`, hoặc env `GEMINI_API_KEYS` /
`TTS_API_KEY`) — không đọc giá trị key ra UI.
