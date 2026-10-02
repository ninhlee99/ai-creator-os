# Kể chuyện YouTube (pillar 3 post-pivot)

## 1. Mục đích

Trụ kể chuyện sau pivot: viết **truyện ngôi thứ nhất (xưng "tôi")** →
vẽ 1 **ảnh minh họa 16:9** cho mỗi cảnh → đọc **giọng TTS** từng cảnh →
dựng video **16:9** (ảnh tĩnh + Ken Burns nhẹ + phụ đề) → đăng YouTube.
**Không quay video, không cinematic** (đã park).

> **Trung thực:** video là ảnh minh họa tĩnh + giọng đọc — không hứa
> "phim"; không hứa bản quyền (nhạc nền do Ninh upload, file
> `autopilot-music.m4a` licensed).

## 2. Pipeline — `internal/studio/story.go`

Tái dùng tối đa piece hiện có (director/story prompt, mediagen image,
TTS chain, `AssemblePhotoList` Ken Burns, `MuxSubtitles` caption,
`MixMusicBed`, studio_jobs):

| Bước | Hàm | Chi tiết |
|---|---|---|
| Tạo job | `CreateStoryJob(StoryParams{Topic, Genre, Words 200–1200, Scenes 3–12, MusicOn, MusicPath, AccountID})` | validate + clamp, chạy nền với job-slot semaphore; fail-closed khi provider nil |
| Viết truyện | `writeStory` | `directorPrompt("story.txt", ...Orient: "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\")")` → JSON `{title, logline, text}`; parse 2 lớp (strict + trích `{...}`) |
| Chia cảnh | `splitScenes` | LLM → JSON `{scenes:[{text, image_prompt}]}`; ≥3 cảnh hợp lệ |
| Ảnh | mỗi cảnh `mg.GenerateImage(16:9)` + asset `studio_assets` | **fail-closed: vẽ lỗi → dừng, không bỏ cảnh** |
| Giọng | `narrator.Speak(text)` → wav + `storyProbeDuration` (ffprobe; không có → 3s ước tính, ghi rõ) | **fail-closed: TTS lỗi → dừng, không dựng video câm** |
| Dựng | `AssemblePhotoList` (1 ảnh/cảnh, thời lượng = audio thật) → `ConcatClips` → `concatWAVs` → mux audio + `MuxSubtitles` (SRT từ thời lượng audio thật) + `MixMusicBed` (nếu MusicOn) | 16:9 MP4 — trả nợ "true 16:9 long-form renderer" của growth |
| QC | 1920×1080 + có video+audio stream + > 30s + đủ số cảnh | rớt → `failed` + lý do tiếng Việt |

Lời bài hát nhạc nền: chung file `autopilot-music.m4a` Ninh upload
(tránh bắt upload 2 lần) — cùng nguồn như autopilot + reup.

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

- `internal/studio/story_test.go`: parse JSON 2 lớp, build SRT, pipeline
  đầy đủ với fake LLM/image/TTS + FFmpeg thật (36s → done, 1920×1080,
  có audio, subtitle mov_text, 3 assets), QC rớt khi ngắn (<30s),
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
