# Tăng trưởng TikTok

Growth hỗ trợ cải thiện hai flow video TikTok; hiện chưa phải flow giao hàng độc lập.

## Luồng dữ liệu

Metric cần account/video ID, provider, thời điểm và kỳ đo. Không có nguồn đọc được thì trạng thái là chưa có dữ liệu. Job chưa đăng không được tính là hiệu quả.

## Gợi ý

Chỉ so sánh video có dữ liệu cùng loại/khoảng đo. Hiển thị cỡ mẫu và nguồn. Gợi ý là thử nghiệm, không phải cam kết view, follower hay doanh thu.

## Trạng thái

TikTok Display API source đã nối cho follower/video totals và current views/likes/comments/shares của public videos khi account cấp `user.info.stats` + `video.list`. Scope và app approval cần được duyệt; account đã kết nối cần OAuth lại.

Retention, watch-time, demographics, profile visits, follows-per-video và video-level order attribution vẫn chưa có source. Những trường này giữ `Chưa có dữ liệu`; không suy ra từ view count.
