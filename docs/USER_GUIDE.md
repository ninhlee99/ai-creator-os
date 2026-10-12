# Hướng dẫn sử dụng (dành cho Ninh)

> Viết lại 2026-10-12 theo app thật (post-pivot: 3 trụ — Affiliate Accesstrade,
> Reup Douyin, YouTube kể chuyện). Mọi quản trị đều qua **dashboard web** —
> không cần chạm CLI.

## 1. Cài đặt

**Cách nhanh nhất:** tải binary đúng máy tại **GitHub Releases**
(`github.com/ninhlee99/ai-creator-os` → Releases), chạy là dùng — không cài
thêm gì (Go runtime không cần).

| Máy | File | Chạy |
|---|---|---|
| Mac M1/M2/M3 | `aicos-darwin-arm64` | `chmod +x aicos-darwin-arm64 && ./aicos-darwin-arm64` |
| Mac Intel | `aicos-darwin-amd64` | tương tự |
| Windows | `aicos-windows-amd64.exe` | nhấp đúp |
| Linux | `aicos-linux-amd64` | `chmod +x` rồi chạy |

Mở trình duyệt: `http://127.0.0.1:8080`. Lần đầu app hỏi thiết lập 1 bước
(dry-run bật sẵn cho an toàn).

**Công cụ ngoài** (trang Cài đặt · Hệ thống tự kiểm tra và báo thiếu, hướng dẫn
cài theo đúng hệ điều hành của máy):

| Công cụ | Để làm gì | Cài |
|---|---|---|
| FFmpeg | dựng video (bắt buộc) | Mac: `brew install ffmpeg` · Win: `winget install Gyan.FFmpeg` · Linux: `sudo apt install ffmpeg` |
| llama.cpp | LLM chạy trên máy (không bắt buộc — có Gemini free) | Mac: `brew install llama.cpp` · Win/Linux: tải bản build tại `github.com/ggerganov/llama.cpp/releases` |
| uv | giọng đọc VieNeu local (không bắt buộc) | Mac: `brew install uv` · Win: `winget install astral-sh.uv` · Linux: `curl -LsSf astral.sh/uv/install.sh \| sh` |

**Build từ source** (nếu cần): cài Go 1.26 rồi `go build -o aicos ./cmd/aicos`.

## 2. Các trang trong dashboard

| Trang | Làm gì |
|---|---|
| Trang chủ | Tổng quan + kill switch |
| Kênh | Tài khoản TikTok/YouTube/Facebook đã kết nối |
| Phát triển kênh | Kế hoạch 30 ngày, canary 7 ngày, luật kill/nhân đôi |
| Đội ngũ | Pipeline Hunter → Director → Producer → Publisher → Analyst |
| Studio AI | Tạo video tay: kịch bản → ảnh → dựng → caption |
| Affiliate | Chiến dịch Accesstrade, tạo link, video affiliate tự đăng |
| Reup | Nguồn Douyin, transform, lịch đăng, kill rule |
| Kể chuyện | Truyện YouTube: hàng đợi chủ đề, job, tự thử lại khi lỗi |
| Đa nền tảng | Kết nối OAuth TikTok/YouTube/Facebook |
| Cài đặt | Nhà cung cấp AI, Reup, Accesstrade, An toàn, Hệ thống |

## 3. Vận hành hằng ngày (hands-off)

1. **Mở máy + mạng + start app** — đó là toàn bộ việc của Ninh. Mọi quyết định
   vận hành (chọn provider, fallback khi hết quota, lịch đăng) do app tự quyết
   theo nguyên tắc "free trước".
2. Telegram báo khi: lỗi/quota/có sự kiện kill (bật ở Cài đặt · Hệ thống).
3. Muốn app tự chạy lại khi crash/restart máy (Mac): làm theo
   `deploy/README.md` (watchdog launchd).
4. Kiểm tra sức khỏe định kỳ: `scripts/watch-health.sh` (cron mỗi 5 phút) —
   phát hiện app treo mà process chưa chết.

## 4. Cập nhật app

1. Tải binary mới từ GitHub Releases, thay file cũ.
2. Restart app (watchdog launchd tự chạy lại bản mới).
3. Xem version đang chạy ở Cài đặt · Hệ thống (thẻ "Phiên bản app").

## 5. An toàn

- **Kill switch:** nút trên thanh topbar — dừng mọi hoạt động tự động trong vài giây.
- **Dry-run:** bật ở Cài đặt · An toàn để xem trước mọi hành động mà không tác động ra ngoài.
- **Mật khẩu dashboard:** đặt ở Cài đặt · Hệ thống → "Mật khẩu truy cập".
  Bắt buộc đặt trước khi mở app ra mạng LAN/VPS. Quên mật khẩu: dừng app, chạy
  `sqlite3 data/ledger.db "DELETE FROM settings WHERE key='auth.password_hash'"`,
  mở app lại và đặt mật khẩu mới.
- Không đọc, in ra hay gửi đi API key/secret/token.

## 6. Backup & khôi phục

App **tự sao lưu mỗi ngày** vào `backups/` (giữ 7 bản mới nhất) — đủ 5 database
(ledger, studio, products, accesstrade, reup). Khôi phục: dừng app, giải nén bản
muốn về, chép đè 5 file `.db` vào `data/`, mở app lại. Chi tiết:
`docs/features/sao-luu-khoi-phuc.md`.
