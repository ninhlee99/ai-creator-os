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

### Watchdog ngoài (tùy chọn, Đợt S)

`KeepAlive` của launchd chỉ chạy lại app khi process **thoát**. Process bị
treo (deadlock) mà chưa chết thì launchd tưởng vẫn khỏe. `scripts/watch-health.sh`
gọi `GET /healthz` định kỳ để phát hiện trường hợp này:

```bash
# cron: kiểm tra mỗi 5 phút
*/5 * * * * ~/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
```

Không kết nối được hoặc `db_ok=false` → ghi 1 dòng log + exit 1. Khi khỏe thì
im lặng (không spam log).

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

Gỡ: `systemctl --user disable --now aicos`.

### Watchdog ngoài (tùy chọn)

```bash
# cron: kiểm tra mỗi 5 phút
*/5 * * * * /path/to/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
```

(Cùng script với Mac — pure sh, không phụ thuộc launchd.)

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
  `docs/features/daemon-tu-dong.md`).
- Chạy binary tay trong khi watchdog đang chạy: app phát hiện đã có instance
  (khóa `aicos.lock`) và thoát êm — không chạy đúp, không giành database.
- Data dir nên ở ổ đĩa local của máy (không để trên ổ mạng/USB): file lock
  trên một số ổ mạng không đảm bảo, 2 instance cùng chạy có thể làm hỏng database.
