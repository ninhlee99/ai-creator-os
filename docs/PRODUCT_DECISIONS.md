# Quyết định sản phẩm

Cập nhật: 2026-10-25.

## Phạm vi đã chốt

1. Sản phẩm có ba flow: video affiliate TikTok, video viral TikTok, truyện audio.
2. Thứ tự giao hàng hiện tại: hoàn thiện hai flow TikTok trước; truyện audio để phase sau.
3. Video affiliate và video viral lấy âm thanh từ video nhảy viral do Radar thu thập.
4. Video affiliate và video viral chỉ dùng âm thanh từ video nhảy đã chọn trong Radar.
5. Không khẳng định sales, đánh giá, followers, quyền sử dụng hay trạng thái đăng nếu chưa có bằng chứng từ nguồn tương ứng.
6. Chỉ dùng provider/tài khoản được phép và chi phí bằng 0; không né giới hạn/quota. Publish TikTok cần consent rõ cho từng nội dung và trạng thái thật từ API.
7. Thiết kế hỗ trợ TikTok Shop Shoppable Video khi partner/creator scope cho phép bind product anchor; Direct Post public chỉ bật khi app audit/intended-use/account scope cho phép. Khi thiếu điều kiện, dùng Draft/manual handoff.

## Điều kiện chưa đạt

- Radar cần nhận diện video nhảy chính xác hơn. Khi chưa chắc, người dùng chọn video trực tiếp từ kết quả Radar.
- Nguồn sản phẩm cần dữ liệu hoa hồng, giá, lượt bán theo kỳ và đánh giá có nguồn/thời điểm rõ. Thiếu dữ liệu thì không gọi sản phẩm là bán chạy hoặc đánh giá tốt.
- Flow đăng TikTok cần phản ánh đúng trạng thái Draft/đang đăng/đã public, trạng thái product anchor và tuân thủ consent theo từng video. Direct Post public và Shop anchor chưa được xác minh với credential thật.
- Growth recommendation chỉ dùng số liệu TikTok có nguồn và thời điểm; không có số liệu thì báo thiếu dữ liệu.

## Tài khoản AI tạo video

Đăng nhập Web AI dùng Chromium cô lập; không đọc profile browser cá nhân. Trạng thái đăng nhập không xác nhận gói miễn phí hoặc credits. JSON backup/import chỉ dùng trong màn API Key.
