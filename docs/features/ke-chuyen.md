# Kể chuyện YouTube & Phim Audio Điện Ảnh (30 - 60 Phút)

## 1. Mục đích & Chuẩn Điện Ảnh Mới
Trụ Kể chuyện theo chuẩn **Open-Spec v1.0 (`docs/OPEN_SPEC.md`)**:
- Sản xuất **Truyện Audio Điện Ảnh** thời lượng dài (từ ngắn 5-10 phút đến trường thiên 30 - 60 phút).
- **Tuyệt đối không để màn hình đen (Zero Black Screen)**: Mỗi 8 - 15 giây kịch bản tương ứng với một visual canvas sống động (ảnh nghệ thuật độ phân giải cao từ Flux / Pollinations / Gemini kết hợp chuyển động chậm Ken Burns và b-roll điện ảnh).
- **Hòa âm không gian 3 lớp**:
  1. Giọng dẫn chuyện qua chuỗi Gemini TTS → Edge (`vi-VN-HoaiMyNeural` hoặc `vi-VN-NamMinhNeural`).
  2. Âm thanh môi trường thực tế (tiếng mưa dầm, tiếng gió đêm, tiếng bước chân) ở mức -14dB.
  3. Nhạc nền cinematic du dương lót dưới âm lượng -18dB (tự động ducking khi có tiếng đọc).
- Dựng video tỷ lệ 9:16 (Shorts/TikTok series) hoặc 16:9 (YouTube Cinema) chuẩn 1080p, xuất bản kèm phụ đề `.srt` và metadata SEO hoàn chỉnh.

## 2. Pipeline — `internal/studio/story.go`

Tái dùng tối đa piece hiện có (director/story prompt, mediagen image,
TTS chain, `AssemblePhotoList` Ken Burns, `MuxSubtitles` caption,
`MixMusicBed`, studio_jobs):

| Bước | Hàm | Chi tiết |
|---|---|---|
| Tạo job | `CreateStoryJob(StoryParams{Topic, Genre, Words 200–1200, Scenes 3–12, MusicOn, MusicPath, AccountID})` | validate + clamp, chạy nền với job-slot semaphore; fail-closed khi provider nil |
| Viết truyện | `writeStory` | `directorPrompt("story.txt", ...Orient: "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\")")` → JSON `{title, logline, text}`; parse 2 lớp (strict + trích `{...}`) |
| Chia cảnh | `splitScenes` | LLM → JSON `{scenes:[{text, image_prompt}]}`; phân chia 8-15s/cảnh, ≥3 cảnh hợp lệ |
| Visual Canvas | mỗi cảnh `mg.GenerateImage` + asset `studio_assets` | **fail-closed: vẽ lỗi → fallback visual canvas, tuyệt đối không để màn hình đen** |
| Giọng đọc | `narrator.Speak(text)` → wav + `storyProbeDuration` (ffprobe; đo thời lượng audio thật) | **fail-closed: TTS lỗi → dừng, không dựng video câm** |
| Dựng phim | `AssemblePhotoList` (1 visual/cảnh, thời lượng = audio thật) → `ConcatClips` → `concatWAVs` → hòa âm 3 lớp (Voice + Foley + Nhạc lót -18dB) | Xuất chuẩn Full HD không méo hình |
| QC | Đúng tỷ lệ + có video & audio stream + > 30s + đủ số cảnh | rớt → `failed` + lý do tiếng Việt rõ ràng |

Lời bài hát nhạc nền: chung file `autopilot-music.m4a` Ninh upload
(tránh bắt upload 2 lần) — cùng nguồn như autopilot + viral.

## 3. Automation — `internal/automation/story_tick.go`

`StoryTick` (main.go gọi mỗi 5 phút, cùng vòng tick):

- Cổng: kill switch + DRY-RUN chặn; `story.enabled` (mặc định **BẬT** khi
  unset — tiền lệ Đợt 3); `s.Story == nil` → bỏ qua im lặng.
- Đến hạn (`story.interval_hours`, mặc định **24h**, watermark
  `story.last_run`) → `popTopic()`: lấy 1 chủ đề từ `story.topics`
  (mỗi dòng 1 chủ đề; lấy xong xoá khỏi hàng đợi) → `CreateStoryJob`
  với mặc định từ settings (`story.words` 600, `story.scenes` 6,
  `story.genre`, `story.music_on`, nhạc từ `StoryMusicPath`).
- Hàng đợi trống → note "thêm chủ đề ở trang Kể chuyện", đóng watermark.
- `storyAutoPublish`: chỉ khi `story.auto_publish=1` (mặc định **0** =
  **chờ Ninh duyệt**), đăng private từng job `done` chưa có watermark
  `story.published.<id>`.
- `PublishStoryNow(ctx, jobID, username)`: fail-closed khi job không
  tồn tại/không phải story/chưa done/chưa có output/không có kênh.
  Upload qua `s.Uploader` (private-first theo cấu hình YouTube publisher),
  kind `"short_film"` (YouTube category 24 — Entertainment), mô tả có
  `growth.DisclosureLine` (disclosure AI). Đăng xong → watermark
  `story.published.<id>=1`.

## 4. UI — trang Kể chuyện (`/stories`, sidebar mục 9)

`internal/web/stories.go` + `templates/stories.html`:

- Badge trạng thái: Tự tạo BẬT/TẮT (số giờ/lần), "Chờ duyệt mới đăng" /
  "Tự đăng (private)", Nhạc nền có/chưa có.
- Card "Kể chuyện mới": chủ đề (bắt buộc), thể loại, số cảnh 3–12,
  số từ 200–1200, nhạc nền on/off → nút "Viết & dựng" (303 + toast).
- Bảng truyện: tiêu đề, trạng thái tiếng Việt, số cảnh (đếm asset thật),
  thời lượng **đo thật từ file** (probe tại render; chưa có file → "—"),
  nút Xem (`/media/<file>`) + form Đăng (chọn kênh, confirm modal,
  chỉ hiện khi done & chưa đăng) + nút Xoá (confirm, xoá file + assets +
  work dir, chỉ cho job kind=story).
- Card Mặc định: thể loại/kênh/từ/cảnh/giờ tick, hàng đợi chủ đề
  (textarea), checkbox "Tự tạo theo lịch" (BẬT mặc định) + "Tự đăng
  private sau QC" (TẮT mặc định), nút Lưu.

Thất bại job: log cuối hiện trong bảng; thời lượng job chưa done là "—"
(không bịa số).

## 5. Test

- `internal/studio/story_test.go`: parse JSON 2 lớp, pipeline
  đầy đủ với fake LLM/image/TTS + FFmpeg thật (36s → done, 1920×1080,
  có audio, không phụ đề, 3 assets), QC rớt khi ngắn (<30s),
  TTS lỗi → failed không output, ảnh lỗi → failed không output,
  `DeleteStoryJob` (xoá file+assets+row, chặn khi không phải story).
- `internal/automation/story_tick_test.go`: hàng đợi trống → cảnh báo
  (không tạo job), lấy 1 chủ đề + xóa khỏi hàng đợi, tắt → im lặng,
  Story nil → im lặng, unset = BẬT, publish fail-closed (job không có /
  chưa done / không kênh).
- `internal/web/stories_test.go`: render khi Studio nil (200 + báo
  trung thực), render đầy đủ + sidebar link, tạo job (303 ok), chủ đề
  trống bị chặn, publish chặn khi job chưa xong, **xoá job** (303 ok,
  job không tồn tại → ?err=), settings lưu và trang hiện lại đúng.
