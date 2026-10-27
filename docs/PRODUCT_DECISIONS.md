# Quyết định sản phẩm

Cập nhật: 2026-10-25.

## Phạm vi đã chốt

1. Phạm vi hiện tại gồm video affiliate TikTok và video viral TikTok; truyện audio là roadmap, không phải flow đang triển khai.
2. Radar video TikTok dùng [Web Crawler cục bộ](features/tiktok-web-crawler.md); TikTok Display API chỉ dùng riêng cho counters sau publish khi account cấp scope.
3. Video affiliate và video viral lấy âm thanh từ video nhảy viral do Radar thu thập.
4. Video affiliate và video viral chỉ dùng âm thanh từ video nhảy đã chọn trong Radar.
5. Không khẳng định sales, đánh giá, followers, quyền sử dụng hay trạng thái đăng nếu chưa có bằng chứng từ nguồn tương ứng.
6. Chỉ dùng provider/tài khoản được phép và chi phí bằng 0; không né giới hạn/quota. Vận hành Radar theo điều kiện production trong [crawler contract](features/tiktok-web-crawler.md). Publish TikTok cần consent rõ cho từng nội dung và trạng thái thật từ API.
7. Thiết kế hỗ trợ TikTok Shop Shoppable Video khi partner/creator scope cho phép bind product anchor; Direct Post public chỉ bật khi app audit/intended-use/account scope cho phép. Khi thiếu điều kiện, dùng Draft/manual handoff.

## Điều kiện chưa đạt

- Dance review hiện là thủ công sau khi người dùng xem video; không suy ra từ caption/sound. Auto-classification không thuộc phạm vi sản phẩm đã chốt.
- Nguồn sản phẩm cần dữ liệu hoa hồng, giá, lượt bán theo kỳ và đánh giá có nguồn/thời điểm rõ. Thiếu dữ liệu thì không gọi sản phẩm là bán chạy hoặc đánh giá tốt.
- Flow đăng TikTok cần phản ánh đúng trạng thái Draft/đang đăng/đã public, trạng thái product anchor và tuân thủ consent theo từng video. Direct Post public và Shop anchor chưa được xác minh với credential thật.
- Growth recommendation chỉ dùng số liệu TikTok có nguồn và thời điểm; không có số liệu thì báo thiếu dữ liệu.

## Tài khoản AI tạo video

Đăng nhập Web AI dùng Chromium cô lập; không đọc profile browser cá nhân. Trạng thái đăng nhập không xác nhận gói miễn phí hoặc credits. JSON backup/import chỉ dùng trong màn API Key.
