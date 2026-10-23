# Video 7 — Kể chuyện YouTube (`/stories`)

**Trạng thái:** tạo truyện mới đang tạm dừng theo U-04. Pipeline nhiều cảnh cần ảnh tham chiếu để giữ nhân vật nhất quán; Pollinations Free API Classic chỉ nhận prompt, còn Gemini image cần billing. Không bắt đầu job mới hoặc autofill chủ đề khi chưa có provider phù hợp.

## Kịch bản

**Bước 1 — Giải thích trạng thái.**
[Thao tác: mở `/stories`, chỉ vào thông báo tạm dừng và card hàng đợi chủ đề.]
"Luồng truyện mới đang dừng trước khi lấy chủ đề hoặc gọi LLM. Chủ đề đã xếp vẫn được giữ nguyên; hệ thống không tạo job dở dang hay tiêu quota viết truyện."

**Bước 2 — Xem lịch sử nếu có.**
[Thao tác: mở job truyện cũ đã hoàn tất hoặc log của job lỗi; nếu chưa có job, chỉ demo trạng thái trang.]
"Job lịch sử vẫn xem và quản lý được. Đây là video ảnh minh họa tĩnh + giọng đọc, không phải footage chuyển động."

**Bước 3 — Publish lịch sử đã hoàn tất.**
[Thao tác: chỉ khi có job done, chọn kênh YouTube và đăng private.]
"Chỉ job dựng xong mới được gửi; Story luôn đăng private trước để chủ kênh kiểm tra."

## Lưu ý trung thực

- Gemini API keys không mở lại tạo ảnh Gemini. LLM/TTS Gemini vẫn cần operator xác nhận key set thuộc quota miễn phí.
- Không demo tạo mới hoặc bật autofill chủ đề khi capability ảnh tham chiếu chưa có.
- Chỉ mô tả YouTube publish nếu đã cấu hình OAuth và nhìn thấy trạng thái thực trên YouTube Studio.
