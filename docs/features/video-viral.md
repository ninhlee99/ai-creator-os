# Tạo video dọc (`/viral`)

Trang `/viral` tạo job video 9:16 từ chủ đề do người dùng nhập. Có thể thêm link video TikTok/Douyin làm tham chiếu chuyển động, chọn ảnh nhân vật trong thư viện kênh hoặc tải ảnh mới, rồi chọn engine và thời lượng. Job chạy trong Studio và theo dõi tại `/studio/jobs`.

Trang hiện không có radar video realtime, danh sách chủ đề mẫu hay thư viện clip để chọn. Link tham chiếu được xử lý trong lúc tạo job; endpoint import clip riêng và các nhóm video preset đã bỏ vì không có UI sử dụng và không có dữ liệu thật.

Luồng chính nằm trong `internal/web/viral.go` và `internal/studio/viral.go`. TTS dùng chuỗi đang cấu hình; nhạc nền là tùy chọn. Video tham chiếu không bắt buộc.
