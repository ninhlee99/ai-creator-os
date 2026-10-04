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

## Xoay log (khỏi đầy đĩa)

launchd ghi mọi log vào `~/aicos-data/logs/aicos.log` và không tự xoay.
Tạo file `/etc/newsyslog.d/aicos.conf` (cần sudo, 1 lần):

```
# logfile                    [owner:group] mode count size when  flags
/Users/NINH/aicos-data/logs/aicos.log  NINH:staff   644  7     20480  *     Z
```

Thay `NINH` bằng username thật. Giữ 7 bản, mỗi bản 20MB.

## Lưu ý

- `GEMINI_API_KEYS` được nhúng vào plist lúc cài (launchd không đọc
  shell profile). Đổi key → chạy lại script cài.
- Watchdog chỉ trông **process app**. Tự chữa job treo / canh đĩa / sao
  lưu vẫn do các tick trong app đảm nhiệm (xem
  `docs/features/daemon-tu-dong.md`).
- Muốn app không chạy nữa: gỡ watchdog (lệnh trên) rồi tắt app —
  nếu chỉ tắt app mà không gỡ, launchd sẽ chạy lại nó (đúng thiết kế).
