# Vận hành 24/7 trên Mac (Đợt O1)

Thư mục này chứa mảnh cuối của "máy chạy không người trông": nếu app
crash lúc 3h sáng hoặc Mac khởi động lại, **launchd tự chạy lại app**
trong vài giây — Ninh không cần mở Terminal.

## Cài 1 lần duy nhất (trên máy Mac của Ninh)

```bash
cd ~/ai-creator-os
GEMINI_API_KEYS="key1,key2" ./scripts/install-launchd.sh \
  ~/ai-creator-os/aicos-darwin-arm64 ~/aicos-data
```

Kiểm tra đang chạy:

```bash
launchctl print gui/$(id -u)/com.ninhlee.aicos | grep -i state
tail -f ~/aicos-data/logs/aicos.log
```

Gỡ:

```bash
./scripts/install-launchd.sh --uninstall
```

## Watchdog ngoài (tùy chọn, Đợt S)

`KeepAlive` của launchd chỉ chạy lại app khi process **thoát**. Process bị
treo (deadlock) mà chưa chết thì launchd tưởng vẫn khỏe. `scripts/watch-health.sh`
gọi `GET /healthz` định kỳ để phát hiện trường hợp này:

```bash
# cron: kiểm tra mỗi 5 phút
*/5 * * * * ~/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
```

Không kết nối được hoặc `db_ok=false` → ghi 1 dòng log + exit 1. Khi khỏe thì
im lặng (không spam log).

## Xoay log (tự động trong app — Đợt R)

App tự ghi và xoay log tại `~/aicos-data/logs/aicos.log`: mỗi file tối đa
20MB, giữ 7 bản cũ (`aicos.log.1` … `aicos.log.7`). Không cần cấu hình gì thêm.

> Vì sao không dùng newsyslog nữa: launchd mở `StandardOutPath` một lần lúc
> start job và giữ file descriptor — newsyslog rename file nhưng launchd vẫn
> ghi vào inode cũ, file "đã xoay" tiếp tục phình vô hạn. App tự quản lý file
> log của mình thì xoay mới thật sự có tác dụng. Plist mẫu cũng đã bỏ
> `StandardOutPath`/`StandardErrorPath` để tránh ghi đúp.

## Lưu ý

- `GEMINI_API_KEYS` được nhúng vào plist lúc cài (launchd không đọc
  shell profile). Đổi key → chạy lại script cài.
- Watchdog chỉ trông **process app**. Tự chữa job treo / canh đĩa / sao
  lưu vẫn do các tick trong app đảm nhiệm (xem
  `docs/features/daemon-tu-dong.md`).
- Muốn app không chạy nữa: gỡ watchdog (lệnh trên) rồi tắt app —
  nếu chỉ tắt app mà không gỡ, launchd sẽ chạy lại nó (đúng thiết kế).
- Chạy binary tay trong khi watchdog đang chạy: app phát hiện đã có instance
  (khóa `aicos.lock`) và thoát êm — không chạy đúp, không giành database.
