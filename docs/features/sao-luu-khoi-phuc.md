# Sao lưu & khôi phục

## 1. Mục đích
Chụp ảnh toàn bộ dữ liệu app thành file zip tải về; khôi phục bằng cách upload file —
an toàn vì file thật chỉ bị thay khi app chưa mở DB.

## 2. Kích hoạt
- Ở Cài đặt · Hệ thống, khối "Sao lưu & khôi phục":
  - `POST /settings/backup` → `handleBackupCreate` (`internal/web/backup.go:21`).
  - `POST /settings/backup/restore` → `handleBackupRestore` (upload file).

## 3. Luồng vận hành chi tiết
1. **Tạo** (`backup.Create`, `internal/backup/backup.go:43`): zip
   - 3 database (+ `-wal`/`-shm` nếu có): `ledger.db`, `studio.db`, `products.db`
   - file đơn: `content_jobs.json`
   - thư mục: `tokens/` (đệ quy 1 cấp, bỏ thư mục con)
   - manifest `aicos-backup.json` (`{"app":"aicos","created":"..."}`)

   File nào thiếu thì bỏ qua (không fail cả bản). Thư mục trống → lỗi
   "thư mục dữ liệu trống — không có gì để sao lưu". File tải về tên
   `aicos-backup-YYYYMMDD-HHMMSS.zip`; ghi decision `human/backup_create`.

   **Cố ý loại trừ**: `output/` (video render), `avatars/`, `models/` — nặng,
   tái tạo được.
2. **Khôi phục** (`handleBackupRestore`): upload (tối đa 512MB) → `backup.Stage`:
   - `backup.Validate`: phải có manifest đúng app + `ledger.db` có magic
     `SQLite format 3` — zip lạ/hỏng/zip-slip → từ chối, **dữ liệu thật không
     bị đụng tới** (fail-closed, `?err=` + toast).
   - Giải nén vào `restore-staging/` + đặt cờ file `RESTORE_PENDING`.
   - Ghi decision `human/backup_restore` ("đã nhận — chờ restart").
3. **Áp dụng**: `backup.ApplyStaged(dataDir)` chỉ chạy lúc **khởi động app**
   (trước khi mở DB): đổi file staged vào vị trí thật, xoá WAL cũ của DB được
   khôi phục, xoá cờ. UI hiện banner "bản khôi phục đang chờ restart".

## 4. Fail-closed & an toàn
- Mọi lỗi validate/upload → `?err="Khôi phục thất bại: <lý do> — dữ liệu hiện tại
  không bị đụng tới."`
- Áp dụng chỉ khi không có handle DB nào mở (lúc startup) → không corrupt DB.

## 5. API key rotation
Không dùng key cloud. (Token OAuth nằm trong `tokens/` nên được sao lưu cùng.)
