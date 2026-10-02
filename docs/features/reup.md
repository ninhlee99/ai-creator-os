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
- Transform/đăng/kill rule: xem §4c–§4e (Đợt E).

## 4. UI — trang Reup (`/reup`, sidebar mục 8)

`internal/web/reup.go` + `templates/reup.html`:

| Card | UI | Route |
|---|---|---|
| Nguồn Douyin | Bảng nguồn (tên, loại user/hashtag, bật/tắt, xoá — confirm modal); form thêm (loại + giá trị + tên). Ghi chú: "nhập 1 lần, tự quét 6 giờ/lần"; hashtag chưa tự quét được | `POST /reup/sources`, `/reup/sources/{id}/toggle`, `/reup/sources/{id}/delete` (form → redirect `?ok=`/`?err=` → toast) |
| yt-dlp | Badge version / "chưa tải" / "đang tải…"; nút **Tải yt-dlp** / **Cập nhật** (AJAX → chạy nền, poll `GET /reup/ytdlp/status` mỗi 5s) | `POST /reup/ytdlp/ensure`, `POST /reup/ytdlp/update`, `GET /reup/ytdlp/status` |
| Hàng đợi tải | Nút **Quét ngay** (AJAX → toast "Tìm X video mới → Y đã tải…"); form **Thêm URL trực tiếp**; bảng video (tiêu đề, tác giả, lượt xem, độ dài, nhãn watermark trung thực, badge trạng thái, lý do lỗi, nút **Tải lại** khi failed) | `POST /reup/scan`, `POST /reup/videos/add`, `POST /reup/videos/{id}/retry` |
| Tự động | Bật/tắt tự quét, chu kỳ (giờ), số video/nguồn (1–10); bật/tắt transform, mức mặc định (1/2), voiceover, tự đăng, video/ngày, kill rule + ngưỡng, warm-up (AJAX → toast) | `POST /reup/settings` (JSON, lưu ledger settings `reup.*`) |
| Transform | Nút **Transform** + chọn mức 1/2 cho video downloaded (option Mức 2 chỉ hiện khi nguồn có ≥3 clip; chặn tạo trùng khi đã có bài còn hiệu lực; AJAX → chạy nền, poll `GET /reup/posts/{id}/status` mỗi 5s) | `POST /reup/videos/{id}/transform` (JSON `{"level":1|2}` → `{"ok":true,"post_id"}`; trùng → 412) |
| Bài đăng (transform) | Bảng reup_posts: badge mức, trạng thái (chờ transform/đang transform/chờ đăng/đã đăng/lỗi + lý do), view ("chờ số liệu" khi chưa có), **before/after 2 video cạnh nhau**, form đăng (chọn kênh + nút **Đăng**); cảnh báo "giảm rủi ro, không đảm bảo an toàn 100%" | `GET /reup/posts/{id}/status`, `GET /reup/file/{id}`, `GET /reup/videos/{id}/file`, `POST /reup/posts/{id}/publish` (form → redirect `?ok=`/`?err=`) |

Fail-closed: kho chưa mở → trang báo "Kho Reup chưa sẵn sàng" (200, không
sập); `POST /reup/scan` thiếu kho → 412 JSON `{"error": …}`.

## 4b. UI — Cài đặt · Reup (`/settings/reup`, tab Reup trong Cài đặt)

`internal/web/reup_transform.go` (handler `handleSettingsReup` /
`handleSettingsReupSave`) + `templates/settings/reup.html`: card trạng thái
(kho, transform, nhạc nền licensed, kill rule); form lưu (POST → 303):
transform on/off, mức mặc định 1/2, voiceover on/off, tự đăng on/off, kênh
đăng tự động, video/ngày (1–20), kill rule on/off + ngưỡng 0-view (2–20),
warm-up on/off. Nhạc nền: badge "có/không" (đọc file
`<dataDir>/autopilot-music.m4a` — Ninh upload 1 lần).

## 4c. Transform 2 mức (`internal/reup/transform.go`, Đợt E)

Chain filter (pure Go, gọi FFmpeg có sẵn; `VideoEncoderArgs`/`FFmpegRun`
tái dùng từ `internal/studio/assemble.go`):

- **Mức 1** (mọi video): T5 zoom động — `zoompan` d=1 trên frame video,
  `z='min(1+0.12*on/<frames>,1.12)'` (scale 2160×3840 cover trước) + T3 tốc
  độ ±5% (`setpts=0.952381*PTS` nhanh / `1.052632*PTS` chậm, seed chẵn/lẻ)
  + T4 grade nhẹ (`eq=contrast=1.06:saturation=1.12:brightness=0.01`) →
  1080×1920, fps=30.
- T8 voiceover tiếng Việt: TTS VieNeu local (không tốn Gemini key); nội
  dung do LLM bình luận ngắn từ title/metadata, hoặc **template trung
  thực** khi không có LLM. Delay 2.0s, lồng với nhạc nền (volume 0.35,
  fade in/out) qua `amix` + `loudnorm` −14 LUFS.
- T7: audio gốc bị thay bằng nhạc licensed; intro/outro card 1.2s mỗi đầu
  (drawtext tiêu đề, nền `#2b2f4a`, fallback card trơn khi thiếu font);
  caption tiếng Việt (SRT burn-in, fallback mov_text khi thiếu font).
- **Mức 2** (video hot): mức 1 + T9 compilation 3 clip cùng nguồn/chủ đề
  (seed offset mỗi clip, `xfade` 0.5s chuyển cảnh).
- Fail-closed: transform lỗi → không xuất file đăng được; voiceover bật mà
  TTS chết → lỗi (không tạo video câm); thiếu cả voiceover lẫn nhạc → lỗi.
- QC sau transform: file tồn tại + non-empty, ffprobe có video **và**
  audio stream, độ dài > 3s; rớt → failed + lý do.

Kết quả ghi `reup_posts` (schema v2): `video_ids` (JSON), `level`,
`file_path`, `status` (pending/transforming/transformed/failed/posted),
`fail_reason`, `views` (mặc định −1 = "chờ số liệu"), `metrics_at`,
`remote_id`, `remote_url`.

**Trung thực tuyệt đối:** code/UI/doc chỉ dùng "giảm rủi ro bản quyền",
không nơi nào hứa "an toàn bản quyền".

## 4d. Kill rule 0-view (`internal/growth/reup_kill.go`, Đợt E)

- `EvaluateReupZeroView(posts, n)`: sắp xếp theo PostedAt; bỏ qua bài
  `views<0` (không đếm, không reset streak); reset khi views>0; kill khi
  streak ≥ n (mặc định 5, cấu hình `reup.killrule_zeroview_n`).
- `FetchYouTubeVideoViews` qua YouTube Data API v3 `videos.list`
  (`statistics.viewCount`); không có key → `ErrNotConnected` (rule ở chế
  độ "chờ số liệu", không bịa). TikTok không có metrics → bài luôn ở
  "chờ số liệu".
- Tick `ReupKillTick` (1 giờ/lần, trong goroutine reup của `cmd/aicos`):
  đồng bộ view YouTube → đánh giá → kích hoạt: `reup.post_enabled=0`
  (dừng đăng), alert `growth` kind `reup_kill` + ledger decide + đề xuất
  đổi nguồn/phong cách. **Không bao giờ xoá video đã đăng.**

## 4e. Automation tick (Đợt E)

`internal/automation/reup_transform.go`: `ReupTransformTick` (15 phút/lần,
≤2 bài/lần, level 2 tự compile 3 clip cùng nguồn), `ReupPostTick` (2 giờ/lần:
kiểm tra công tắc + kênh + giới hạn/ngày với warm-up 1/ngày × 2 ngày đầu →
2/ngày → full), export `PublishReupPostNow` cho nút Đăng UI. Tất cả sau
kill switch + dry-run + công tắc riêng mặc định BẬT. Nối trong goroutine
reup `cmd/aicos/main.go`; `ReupTTS`/`ReupLLM`/`ReupMusicPath` nối từ web
`automation()` (TTS chain, LLM service, file `autopilot-music.m4a` nếu có).

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

**Đợt E** — `internal/reup/transform_test.go`: FFmpeg thật (testsrc 720×1280
+ sine): mức 1 ra 1080×1920 có audio, độ dài ≈ gốc + 2.4s intro/outro;
compilation mức 2 nối 3 clip + xfade ≈ 15s; QC rớt khi file rỗng/thiếu
audio/không tồn tại; voiceover fail-closed (TTS nil); filter/text/SRT unit.
`internal/reup/posts_test.go`: CRUD reup_posts, metrics, CountPostedSince,
VideosNeedingTransform (failed được transform lại), compilation ordering.
`internal/growth/reup_kill_test.go`: kill đúng 5 bài 0-view liên tiếp;
streak reset khi có view; bài "chờ số liệu" (views<0) không đếm; no-data
fail-closed; ngưỡng n tuỳ chỉnh; YouTube views mock qua
`youtubeAPIBase` inject.
`internal/automation/reup_transform_test.go`: tick gates (kill/dry-run/tắt
công tắc/thiếu store); transform tick thật (video giả + nhạc fake);
kill tick: no-data → "chờ số liệu" không tắt đăng; kill trigger →
`post_enabled=0` + alert + bài đã đăng giữ nguyên; post tick fail-closed
(chưa có kênh, thiếu file transform).
`internal/web/reup_transform_test.go`: transform thiếu kho/chưa downloaded
→ 412; tạo bài + poll status (file giả → failed, kiểm tra trong 10s);
file lạ → 404; publish thiếu kênh → redirect err; settings mở rộng
(kill_n cắt trần 20); tab `/settings/reup` render + lưu form (303).
