# Sao lưu & khôi phục

## 1. Mục đích
Chụp dữ liệu vận hành app thành file zip tải về; credential được lược bỏ khỏi archive.
Khôi phục bằng cách upload file — file thật chỉ bị thay khi app chưa mở DB.

## 2. Kích hoạt
- Ở Cài đặt · Hệ thống, khối "Sao lưu & khôi phục":
  - `POST /settings/backup` → `handleBackupCreate` (`internal/web/backup.go:21`).
  - `POST /settings/backup/restore` → `handleBackupRestore` (upload file).

## 3. Luồng vận hành chi tiết
1. **Tạo** (`backup.CreateValidatedToFile`): chụp DB SQLite qua online backup,
   nén vào file tạm, kiểm tra archive trên đĩa rồi stream/tái đặt file — không
   cần giữ toàn bộ ZIP (tới 512 MiB) trong RAM.
   - 4 database: `ledger.db`, `studio.db`, `products.db`, `accesstrade.db`; `webai.db` bị loại vì chứa cookie/session bearer. SQLite online backup nên archive không cần `-wal`/`-shm`.
   - Từ `ledger.db` bị lược `env:*`, API key, notification secret, webhook và password hash; cấu hình provider còn lại được giữ nhưng JSON credential fields bị xóa. Snapshot được `secure_delete` + `VACUUM` trước ZIP.
   - file đơn: `content_jobs.json`
   - input do người dùng tải lên và DB còn tham chiếu: `output/studio/models/` và
     `output/studio/uploads/` (ảnh nhân vật, ảnh listing, clip/audio đầu vào).
   - manifest `aicos-backup.json` (`{"app":"aicos","created":"..."}`)

   File nào thiếu thì bỏ qua (không fail cả bản). Thư mục trống → lỗi
   "thư mục dữ liệu trống — không có gì để sao lưu". File tải về tên
   `aicos-backup-YYYYMMDD-HHMMSS.zip`; ghi decision `human/backup_create`.

   **Cố ý loại trừ**: OAuth token files trong `tokens/`, tài khoản Web AI,
   credential settings, video render và cache trong `output/` ngoài hai thư mục
   input trên, cùng `avatars/` và model AI tải về. Ảnh nhân vật và upload đầu
   vào được giữ vì workflow viral/affiliate cần chúng để retry.
2. **Khôi phục** (`handleBackupRestore`): upload (tối đa 512MB) → `backup.Stage`:
   - `backup.Validate`: manifest đúng app, `ledger.db` có magic `SQLite format 3`,
     entry nằm trong allowlist, không trùng đường dẫn và tổng dữ liệu giải nén
     không quá 512 MiB; mọi DB được kiểm tra toàn vẹn — zip lạ/hỏng/zip-slip →
     từ chối, **dữ liệu thật không bị đụng tới** (fail-closed, `?err=` + toast).
   - Giải nén vào `restore-staging/` + đặt cờ file `RESTORE_PENDING`.
   - Đường dẫn asset trong thư viện nhân vật, job viral/affiliate, shot plan và
     workflow đã lưu được chuyển sang asset của data dir khôi phục.
   - Ghi decision `human/backup_restore` ("đã nhận — chờ restart").
3. **Áp dụng**: `backup.ApplyStaged(dataDir)` chỉ chạy lúc **khởi động app**
   (trước khi mở DB): đổi file staged vào vị trí thật, xoá WAL cũ của DB được
   khôi phục, xoá cờ. UI hiện banner "bản khôi phục đang chờ restart".

## 4. Fail-closed & an toàn
- Mọi lỗi validate/upload → `?err="Khôi phục thất bại: <lý do> — dữ liệu hiện tại
  không bị đụng tới."`
- Áp dụng chỉ khi không có handle DB nào mở (lúc startup) → không corrupt DB.

## 5. Credential recovery
Sau restore, nhập lại API key, env secret, notification credential và đăng nhập
lại Web AI/TikTok. Đây là chủ ý bảo vệ: backup ZIP không chứa bearer credentials.
Archive tạm và backup tự động được tạo với permission riêng tư của hệ điều hành.
