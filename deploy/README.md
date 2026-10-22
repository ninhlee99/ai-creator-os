# Chạy MiuNova

App chỉ chạy khi người dùng mở binary hoặc chạy lệnh thủ công. Repo không cài,
bật hay cấu hình service chạy lúc đăng nhập/khởi động máy. Nếu cần tự khởi động,
người dùng tự cấu hình bằng công cụ hệ điều hành.

Để gỡ LaunchAgent cũ trên macOS:

```bash
./scripts/install-launchd.sh --uninstall
```

## Truy cập từ thiết bị khác

App mặc định chỉ nghe ở `127.0.0.1` (an toàn). Muốn mở từ xa:

1. **Đặt mật khẩu trước**: Cài đặt · Hệ thống → Mật khẩu truy cập. Chưa có
   mật khẩu thì app **tự chặn** mọi truy cập không đến từ chính máy chủ (kể cả
   qua tunnel) và hiện hướng dẫn — không có chuyện quên mật khẩu mà lộ app.
2. **Cách khuyên dùng — Tailscale** (miễn phí, không mở port, mã hóa sẵn):
   cài Tailscale trên máy chạy app và trên điện thoại, đăng nhập cùng tài
   khoản, rồi trên máy chạy app:

   ```bash
   tailscale serve --bg 8080
   ```

   Mở `https://<tên-máy>.<tailnet>.ts.net` trên điện thoại.
3. Thay thế: Cloudflare Tunnel (`cloudflared tunnel --url http://127.0.0.1:8080`).
   Không cần đổi `-addr`; app nhận ra request đi qua tunnel.

Không khuyên mở `-addr :8080` thẳng ra Internet (không có HTTPS).

## Xoay log (tự động trong app — Đợt R, mọi OS)

App tự ghi và xoay log tại `<data-dir>/logs/aicos.log`: mỗi file tối đa
20MB, giữ 7 bản cũ (`aicos.log.1` … `aicos.log.7`). Không cần cấu hình gì thêm.

App tự quản lý file log để giữ giới hạn dung lượng nhất quán trên mọi hệ điều hành.

## Lưu ý chung

- Các tick nền chỉ chạy khi app đang mở (xem `docs/features/autonomous-daemon.md`).
- Data dir nên ở ổ đĩa local của máy (không để trên ổ mạng/USB): file lock
  trên một số ổ mạng không đảm bảo, 2 instance cùng chạy có thể làm hỏng database.
