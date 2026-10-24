# Tác vụ nền hỗ trợ flow TikTok

## Tác vụ liên quan

- AccessTrade: product hunt lưu offer chờ duyệt; chỉ offer được duyệt và có sales/rating evidence còn hạn mới vào hàng video tự động; campaign check và order sync theo cấu hình.
- Growth: đồng bộ/lập kế hoạch theo dữ liệu hiện có; không tự tạo video TikTok nếu producer chưa hỗ trợ input.
- Studio maintenance: đánh dấu job treo, dọn work lỗi và sao lưu dữ liệu.
- Notification: gửi cảnh báo theo cấu hình.

## Ranh giới

Tick tồn tại không chứng minh từng provider call thành công. Trạng thái scheduler, heartbeat, job và API result phải được hiển thị riêng. Flow affiliate không được claim sản xuất tự động khi thiếu offer evidence, clip/provider hợp lệ hoặc source audio Radar. Dữ liệu sản phẩm hiện chưa có phân tích xu hướng/tệp tuổi tự động. TikTok Display API source đọc follower và counters video công khai khi được cấp scope; retention, demographics và orders theo video vẫn chưa có source.
