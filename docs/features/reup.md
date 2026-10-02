# Reup Douyin (tải video viral)

## 1. Mục đích
Trụ reup sau pivot: tự tìm video viral Douyin (thần tiên/tỉ tỉ…) từ nguồn
Ninh nhập 1 lần → tải về kho → chờ transform + đăng (Đợt E).

> **Trung thực:** reup vi phạm bản quyền nguyên tắc dù có transform; nguy
> cơ lớn nhất 2026 là video **0-view/de-boost** chứ không chỉ strike.
> Không chỗ nào trong pipeline hứa "an toàn bản quyền".

## 2. Package — `internal/reup/`

| File | Vai trò |
|---|---|
| `doc.go` | Ghi chú trung thực: yt-dlp gãy định kỳ, TikWM không SLA, không flag `--no-watermark` trong yt-dlp gốc |
| `store.go` | SQLite `reup.db` (WAL, `PRAGMA user_version` v1): `reup_sources` (kind=user/hashtag, value, enabled), `reup_videos` (douyin_id, url, file_path, sha256, play_count, via, watermark_free, status=queued/downloading/downloaded/failed, qc_method), `reup_posts` (Đợt E dùng) |
| `ytdlp.go` | `Manager`: binary **do app tự quản** trong `<dataDir>/bin` (KHÔNG bundle vào binary, không thêm runtime/cgo — đúng mẫu sidecar models). `Ensure()` tải từ GitHub release theo OS/arch (`yt-dlp_macos` / `yt-dlp` / `yt-dlp.exe`); `Check()` = `--version`; `IsExtractorBroken()` nhận diện stderr extractor gãy (signature/cookie) → `Update()` 1 lần rồi thử lại. `Download()` chạy binary với `--no-playlist --print after_move:filepath/id/title/duration` |
| `tikwm.go` | `TikWM`: fallback HTTP thuần Go tới `https://www.tikwm.com` (**bên thứ ba, không SLA, có thể die**). `Lookup(url)` → link `play`/`hdplay` (no-watermark) + metadata; `UserPosts(unique_id)` → danh sách video (`/api/user/posts`). Link CDN có chữ ký, hết hạn vài phút — luôn tải ngay |
| `download.go` | `Downloader`: yt-dlp trước → gãy thì TikWM. **Dedupe 2 lớp:** douyin_id (trước tải — không tải lại video đã có/đang tải) và sha256 (sau tải — URL khác nhưng cùng nội dung thì bỏ). **QC sau tải:** có `ffprobe` → kiểm tra video stream thật; không có → chỉ size + đuôi file, ghi `qc_method="size-only"` (không đoán "đạt chuẩn"). Rớt QC → `failed` + lý do |
| `discover.go` | `Discoverer`: với mỗi nguồn đang bật loại `user` → `UserPosts` → bỏ video đã biết → **auto-pick theo `play_count` cao nhất** → queue vào kho. Nguồn `hashtag`: TikWM không có endpoint list ổn định → **không bịa danh sách**, trả note "nhập URL trực tiếp" |

Về watermark — đã kiểm chứng: yt-dlp gốc **không có flag `--no-watermark`**,
nên không truyền (truyền flag không tồn tại sẽ làm mọi lượt tải lỗi).
Ghi trung thực từng video: `via=tikwm` → "không watermark" (link `play`);
`via=ytdlp` → "có thể có watermark".

## 3. Automation — `internal/automation/reup_tick.go`

`ReupTick` (main.go gọi mỗi 5 phút, cùng vòng với `ATTick`):
- Cổng: kill switch + DRY-RUN chặn; `reup.discover_enabled` (mặc định **BẬT**
  khi unset — tiền lệ Đợt 3); `s.Reup == nil` → bỏ qua im lặng.
- Đến hạn 6 giờ (`reup.discover_interval_hours`, watermark
  `reup.discover_last_run`) → `Discover(per_source)` →
  `DownloadCandidate` từng video → note "X video mới → Y đã tải, Z trùng, W lỗi".
- Chưa nối transform/đăng (Đợt E).

## 4. UI — trang Reup (`/reup`, sidebar mục 8)

`internal/web/reup.go` + `templates/reup.html`:

| Card | UI | Route |
|---|---|---|
| Nguồn Douyin | Bảng nguồn (tên, loại user/hashtag, bật/tắt, xoá — confirm modal); form thêm (loại + giá trị + tên). Ghi chú: "nhập 1 lần, tự quét 6 giờ/lần"; hashtag chưa tự quét được | `POST /reup/sources`, `/reup/sources/{id}/toggle`, `/reup/sources/{id}/delete` (form → redirect `?ok=`/`?err=` → toast) |
| yt-dlp | Badge version / "chưa tải" / "đang tải…"; nút **Tải yt-dlp** / **Cập nhật** (AJAX → chạy nền, poll `GET /reup/ytdlp/status` mỗi 5s) | `POST /reup/ytdlp/ensure`, `POST /reup/ytdlp/update`, `GET /reup/ytdlp/status` |
| Hàng đợi tải | Nút **Quét ngay** (AJAX → toast "Tìm X video mới → Y đã tải…"); form **Thêm URL trực tiếp**; bảng video (tiêu đề, tác giả, lượt xem, độ dài, nhãn watermark trung thực, badge trạng thái, lý do lỗi, nút **Tải lại** khi failed) | `POST /reup/scan`, `POST /reup/videos/add`, `POST /reup/videos/{id}/retry` |
| Tự động | Bật/tắt tự quét, chu kỳ (giờ), số video/nguồn (1–10, AJAX → toast) | `POST /reup/settings` (JSON, lưu ledger settings `reup.*`) |

Fail-closed: kho chưa mở → trang báo "Kho Reup chưa sẵn sàng" (200, không
sập); `POST /reup/scan` thiếu kho → 412 JSON `{"error": …}`.

## 5. Test (không tải thật)

`internal/reup/reup_test.go`: store round-trip + dedupe; yt-dlp giả bằng
script shell (version/download/extractor-broken); `Update()` từ release
giả (httptest, `ReleaseBase` inject được); TikWM mock (`BaseURL` inject);
dedupe sha256 (2 URL → cùng bytes → video 2 failed "trùng nội dung"); QC
rớt với file rỗng; discover pick đúng top `play_count`, bỏ video đã có;
hashtag → không bịa candidate, note trung thực.

`internal/automation/reup_tick_test.go`: kill/dry-run/tắt công tắc/thiếu
store → bỏ qua; chưa có nguồn → note rõ; happy path (TikWM mock, yt-dlp
ép gãy bằng release 404) → 2 video `downloaded`, `via=tikwm`,
`watermark_free=true`, `qc_method` ghi đúng.

`internal/web/reup_test.go`: render `/reup` khi thiếu kho (fail-closed);
thêm/bật-tắt/xoá nguồn (303); lưu settings (cắt trần per_source=10);
scan thiếu kho → 412.
