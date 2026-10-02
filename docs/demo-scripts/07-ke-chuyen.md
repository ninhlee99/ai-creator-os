# Video 7 — Kể chuyện YouTube (`/stories`)

**Mục tiêu:** tạo 1 truyện ngôi thứ nhất → xem pipeline (viết → chia cảnh → vẽ ảnh → đọc giọng → dựng) → duyệt và đăng private.

## Chuẩn bị

- **Cần `GEMINI_API_KEYS` thật** (vẽ ảnh + viết truyện + TTS). Không có: demo tới bước job fail "media-gen chưa sẵn sàng".
- File nhạc `autopilot-music.m4a` trong data dir (nếu muốn có nhạc nền).
- Dry-run TẮT nếu muốn job chạy thật.

## Kịch bản

**Bước 1 — Tạo truyện.**
[Thao tác: trang Kể chuyện, card "Kể chuyện mới": nhập chủ đề demo "Chuyến đi Đà Lạt nhớ đời", thể loại "hồi ký", 6 cảnh, 600 từ, bật nhạc nền, bấm "Viết & dựng".]
"Tôi nhập chủ đề, chọn 6 cảnh, 600 từ — bấm Viết và dựng. Từ đây máy tự làm hết."

**Bước 2 — Xem pipeline chạy.**
[Thao tác: bảng truyện, xem badge trạng thái chuyển dần; mở log cuối.]
"Máy viết truyện ngôi thứ nhất xưng 'tôi', chia thành từng cảnh, vẽ một ảnh minh họa 16:9 cho mỗi cảnh, đọc giọng từng cảnh, rồi dựng thành video 16:9 có phụ đề."
[Thao tác: chỉ vào log nếu có lỗi fail-closed.]
"Vẽ lỗi hay đọc giọng lỗi thì job dừng và ghi rõ lý do — không bao giờ ra video câm hay thiếu cảnh."

**Bước 3 — Xem trước.**
[Thao tác: job done → bấm Xem, mở video.]
"Xong — video 16:9, ảnh minh họa chạy Ken Burns nhẹ theo giọng đọc, có phụ đề tiếng Việt. Thời lượng đo thật từ file, chưa xong thì hiện dấu gạch ngang chứ không bịa."

**Bước 4 — Duyệt và đăng.**
[Thao tác: form Đăng, chọn kênh YouTube, bấm Đăng (confirm "RIÊNG TƯ").]
"Mặc định máy CHỜ bạn duyệt mới đăng — đây là luật. Đăng lên ở chế độ riêng tư trước, bạn xem lại rồi mới public. Muốn máy tự đăng private sau QC thì bật công tắc ở card Mặc định."

**Bước 5 — Tự động theo lịch.**
[Thao tác: card Mặc định: hàng đợi chủ đề (mỗi dòng 1 chủ đề), "Tự tạo theo lịch" BẬT, chu kỳ 24h.]
"Bạn ném sẵn chủ đề vào hàng đợi, mỗi 24 giờ máy lấy một cái làm video mới. Hết chủ đề thì máy ghi chú nhắc bạn thêm — không tự bịa chủ đề."

## Lưu ý trung thực

- **Cần key Gemini thật + YouTube OAuth thật trên máy Ninh mới demo được đăng thật**; chưa có OAuth thì dừng ở "chờ kết nối".
- Nói rõ: video là ảnh minh họa tĩnh + giọng đọc — không phải phim quay.
- Mô tả video đăng lên có dòng disclosure nội dung AI — app tự gắn.
