# Trang chủ (`/`)

## 1. Mục đích
Tổng quan một màn hình: thẻ số liệu (doanh thu, tài khoản theo trạng thái,
job Studio), feed 8 quyết định gần nhất, cảnh báo growth, trạng thái runtime
local, và chip công tắc chính (MASTER_SWITCH).

## 2. Kích hoạt
- Người dùng mở `/` trên trình duyệt.
- `handleDashboard` (`internal/web/dashboard.go:11`) render template `dashboard.html`.

## 3. Luồng vận hành chi tiết
`handleDashboard` đọc và tính (không ghi gì ngoài việc đọc):
1. `s.Mgr.List()` (`network.AccountManager`) → danh sách tài khoản;
   `sortedStatusCounts` đếm theo từng trạng thái.
2. Doanh thu: `SELECT COALESCE(SUM(commission),0) FROM orders` (bảng `orders`
   trong ledger.db — chỉ có số khi đối soát ghi đơn thật, xem
   `daemon-tu-dong.md`).
3. `s.Ledger.TotalRevenue()` → hoa hồng đã ghi nhận (append-only).
4. 8 quyết định mới nhất: `SELECT ... FROM decisions ORDER BY id DESC LIMIT 8`
   (bảng `decisions` — nhật ký mọi quyết định của hệ thống + con người).
5. `s.recentGrowthAlerts(accounts)` → cảnh báo growth chưa đọc.
6. `s.localRuntimeViews()` → trạng thái FFmpeg / llama-server / VieNeu
   (probe thật bằng `exec.LookPath`).
7. `s.Jobs.Count()` → số job video chữ động; `HasStudioJobs` → đã có job
   Studio nào chưa (checklist "bắt đầu nhanh").
8. Chip công tắc chính: `automation.MasterOn(s.settings())` — đọc key
   `master.switch` trong ledger settings, nhãn trung thực "TẮT — daemon đang
   đứng yên" / "BẬT — daemon được phép chạy theo lịch".

> Đợt A đã gỡ mọi thẻ/khối liên quan live (lịch live hôm nay, avatar sidecar).
> Thẻ 3 trụ (doanh thu AT, reup, kể chuyện) sẽ thêm theo từng pipeline ở các
> đợt B→F.

## 4. Fail-closed & an toàn
- Trang chủ chỉ đọc; không thao tác thật nên không bị dry-run chặn.
- Lỗi đọc DB ở bước nào → `s.fail` trả lỗi 500 với tên bước, không render nửa trang.

## 5. API key rotation
Không dùng key cloud trực tiếp. `hasAnyAPIKey` chỉ kiểm tra sự tồn tại của key
(keyring DB cho engine `tts`/`llm` loại `gemini`, hoặc env `GEMINI_API_KEYS` /
`TTS_API_KEY`) — không đọc giá trị key ra UI.
