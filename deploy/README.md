# Vận hành 24/7 (Đợt O1, mở rộng đa nền tảng ở Đợt X)

Thư mục này chứa mảnh cuối của "máy chạy không người trông": nếu app
crash lúc 3h sáng hoặc máy khởi động lại, hệ điều hành **tự chạy lại app**
trong vài giây — Ninh không cần mở Terminal.

## 1. Trên Mac (launchd)

### Cài 1 lần duy nhất (trên máy Mac của Ninh)

```bash
cd ~/ai-creator-os
GEMINI_API_KEYS="key1,key2" ./scripts/install-launchd.sh \
  ~/ai-creator-os/aicos-darwin-arm64 ~/aicos-data
```

(Không bắt buộc nhúng key: bỏ trống cũng được, nhập key sau ở
Cài đặt · Nhà cung cấp trong dashboard.)

Kiểm tra đang chạy:

```bash
launchctl print gui/$(id -u)/com.ninhlee.aicos | grep -i state
tail -f ~/aicos-data/logs/aicos.log
```

Gỡ:

```bash
./scripts/install-launchd.sh --uninstall
```

### Watchdog sức khỏe (tự cài, Đợt S → AK)

`KeepAlive` của launchd chỉ chạy lại app khi process **thoát**. Process bị
treo (deadlock) mà chưa chết thì launchd tưởng vẫn khỏe. `scripts/watch-health.sh`
gọi `GET /healthz` định kỳ để phát hiện trường hợp này:

```bash
# cron: kiểm tra mỗi 5 phút
*/5 * * * * ~/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
```

**Từ Đợt AK, `install-launchd.sh` tự cài watchdog này chạy mỗi 5 phút** —
không cần cron. Không kết nối được, `db_ok=false` hoặc `needs_restart=true`
(tác vụ nền kẹt) **2 lần liên tiếp** → tự `launchctl kickstart -k` app. Khi
khỏe thì im lặng (không spam log). Log: `<data>/logs/watchdog.log`. Chỉ muốn
ghi log, không tự restart: `AICOS_WATCHDOG_RESTART=0`.

### Lưu ý riêng Mac

- `GEMINI_API_KEYS` được nhúng vào plist lúc cài (launchd không đọc
  shell profile). Đổi key → chạy lại script cài (hoặc nhập ở dashboard).
- Muốn app không chạy nữa: gỡ watchdog (lệnh trên) rồi tắt app —
  nếu chỉ tắt app mà không gỡ, launchd sẽ chạy lại nó (đúng thiết kế).

## 2. Trên Windows (Task Scheduler)

### Tự chạy lại khi crash / đăng nhập

1. Mở **Task Scheduler** → Create Task (đừng dùng Create Basic Task).
2. Tab **General**: đặt tên `aicos`; nếu muốn chạy cả khi chưa đăng nhập thì
   chọn "Run whether user is logged on or not".
3. Tab **Triggers** → New → "At log on" (máy của Ninh).
4. Tab **Actions** → New:
   - Program: `C:\path\to\aicos-windows-amd64.exe`
   - Add arguments: `-data C:\aicos-data`
   - **Start in: `C:\path\to\`** (thư mục chứa file exe — để app tìm đúng
     thư mục làm việc; nếu để trống, một số tính năng nền có thể mất ảnh nền).
5. Tab **Settings**: tick "If the task fails, restart every" → `1 minute`
   (tương đương KeepAlive/restart on failure).

Kiểm tra: Task Scheduler → Task Status, hoặc mở `http://127.0.0.1:8080/healthz`.

Gỡ: xóa task `aicos` trong Task Scheduler.

### Watchdog ngoài (tùy chọn)

Task thứ hai, trigger mỗi 5 phút lặp vô thời hạn:

- Program: `powershell.exe`
- Add arguments: `-NoProfile -ExecutionPolicy Bypass -File "C:\path\to\ai-creator-os\scripts\watch-health.ps1"`
- (Tùy chọn) đổi URL kiểm tra bằng biến môi trường `AICOS_HEALTH_URL`.
- Lỗi 2 lần liên tiếp (mất kết nối, DB lỗi, tác vụ nền kẹt) → script tự
  Stop + Start task `aicos` (Đợt AK). Tên task khác: đặt `AICOS_TASK_NAME`.

Không kết nối được hoặc `db_ok=false` → ghi 1 dòng log + exit 1. Khi khỏe thì
im lặng (muốn ghi cả khi khỏe: set `WATCHDOG_VERBOSE=1`).

## 3. Trên Linux (systemd)

### Tự chạy lại khi crash / khởi động

Tạo file `~/.config/systemd/user/aicos.service`:

```ini
[Unit]
Description=AI Creator OS
After=network-online.target

[Service]
ExecStart=/path/to/aicos-linux-amd64 -data /path/to/aicos-data
Restart=always
RestartSec=30

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now aicos
# kiểm tra:
systemctl --user status aicos
```

**Bắt buộc cho 24/7:** dịch vụ `--user` mặc định DỪNG khi bạn đăng xuất. Bật
chạy cả khi không đăng nhập (1 lần):

```bash
sudo loginctl enable-linger "$USER"
```

Gỡ: `systemctl --user disable --now aicos`.

### Watchdog ngoài (tùy chọn)

```bash
# cron: kiểm tra mỗi 5 phút
*/5 * * * * /path/to/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
```

(Cùng script với Mac.) Lỗi 2 lần liên tiếp → tự `systemctl --user restart aicos`
(Đợt AK).

## 4. Dùng từ bất kỳ đâu (điện thoại, máy khác) — Đợt AK

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

> Vì sao không dùng newsyslog nữa (Mac): launchd mở `StandardOutPath` một lần lúc
> start job và giữ file descriptor — newsyslog rename file nhưng launchd vẫn
> ghi vào inode cũ, file "đã xoay" tiếp tục phình vô hạn. App tự quản lý file
> log của mình thì xoay mới thật sự có tác dụng. Plist mẫu cũng đã bỏ
> `StandardOutPath`/`StandardErrorPath` để tránh ghi đúp.

## Lưu ý chung

- Watchdog (launchd / Task Scheduler / systemd) chỉ trông **process app**.
  Tự chữa job treo / canh đĩa / sao lưu vẫn do các tick trong app đảm nhiệm (xem
  `docs/features/autonomous-daemon.md`).
- Chạy binary tay trong khi watchdog đang chạy: app phát hiện đã có instance
  (khóa `aicos.lock`) và thoát êm — không chạy đúp, không giành database.
- Data dir nên ở ổ đĩa local của máy (không để trên ổ mạng/USB): file lock
  trên một số ổ mạng không đảm bảo, 2 instance cùng chạy có thể làm hỏng database.
