# Kể chuyện (`/stories`)

## Đang hoạt động

- Form tạo job nhận chủ đề hoặc truyện có sẵn. File `.txt` đọc trong browser;
  `.docx` được chuyển thành text qua `POST /stories/upload-text`.
- `POST /stories` xếp job nền trong Studio. Pipeline viết hoặc nhận truyện,
  chia cảnh, tạo ảnh, đọc TTS, ghép MP4 và cập nhật trạng thái job.
- Trang liệt kê job, thời lượng đo từ file, tiến trình/lỗi; job lỗi có thể thử lại.
- Lịch StoryTick lấy một chủ đề mỗi chu kỳ. Công tắc lịch nằm trong bảng Tự động
  hóa trên Trang chủ; mặc định tắt. Cadence mặc định 24 giờ. Master, Dry-run và
  Kill switch vẫn áp dụng.
- Tự refill chủ đề và tự đăng là cài đặt riêng. Tự đăng mặc định tắt, yêu cầu
  bật lịch truyện và chọn kênh; khi chạy, upload ép `privacyStatus=private`,
  không phụ thuộc `YOUTUBE_DEFAULT_PRIVACY` của pipeline khác.

## Giới hạn hiện tại

- U-04 tạm dừng job truyện nhiều cảnh mới: renderer cần ảnh cảnh đầu làm tham
  chiếu cho các cảnh sau để giữ nhân vật đồng nhất, nhưng provider ảnh miễn phí
  hiện tại chỉ nhận prompt; Gemini image không có free tier. `CreateStoryJob`
  từ chối trước khi xếp hàng. Retry dừng trước khi vẽ nếu còn thiếu ảnh ở cảnh
  sau cảnh đầu.
- Nút **Viết truyện & đưa vào hàng đợi** không xuất bản lên YouTube. Có route
  `POST /stories/{id}/publish`, nhưng bảng `/stories` hiện chưa có nút đăng tay.
- Story form thủ công mặc định video dọc 9:16; Growth persona job có thể chọn
  16:9 theo variant `youtube_long`. Story pipeline chưa đạt spec long-form
  30–60 phút: job Growth 150 giây không phải bản long-form đó.
- Thời lượng 30/45/60 phút là độ dài timeline đích. `CreateStoryJob` yêu cầu
  tối thiểu 150 từ/phút, nhưng `writeStory` giới hạn truyện AI ở 1.800 từ.
  Pipeline có thể lặp video ngắn và đệm audio đến thời lượng đích; job `done`
  không đảm bảo thoại kéo dài đủ hoặc không có đoạn lặng/cảnh lặp.
- Có thể nhập truyện dài riêng, nhưng không có kiểm tra độ dài thoại so với
  timeline trước khi job hoàn tất. TTS lỗi hoặc render/QC lỗi thì job thất bại.
- Không có phụ đề trong video. Form tạo tay hiện không có điều khiển nhạc nền;
  một số tuỳ chọn nâng cao chỉ áp dụng cho StoryTick.
- Lưu mặc định chỉ ghi những trường của form. Không đổi lịch StoryTick hoặc
  `music_on`; các công tắc đó phải được quản lý riêng.

## Nguồn chính

- `internal/web/stories_create.go` — form, nạp `.txt/.docx`, tạo job.
- `internal/studio/story.go`, `story_script.go`, `story_pipeline.go` — viết,
  chia cảnh, ảnh/TTS, dựng và QC.
- `internal/automation/story_tick.go` — lịch tạo truyện, refill, auto-publish.
