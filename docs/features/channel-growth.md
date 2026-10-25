# Tăng trưởng TikTok

Growth hỗ trợ cải thiện hai flow video TikTok; hiện chưa phải flow giao hàng độc lập.

## Luồng dữ liệu

Metric cần account/video ID, provider, thời điểm và kỳ đo. Không có nguồn đọc được thì trạng thái là chưa có dữ liệu. Job chưa đăng không được tính là hiệu quả. Video thủ công gắn vào job chỉ được dùng để ghép đơn khi video ID xuất hiện trong snapshot `video.list` mới của đúng kênh; trước xác minh, đơn giữ trạng thái chưa ghép.

## Gợi ý

Chỉ so sánh video có dữ liệu cùng flow trên cùng kênh. Hiển thị số video baseline; dưới 3 video đối chiếu thì chỉ báo mẫu nhỏ, chưa kết luận cao/thấp. Gợi ý là thử nghiệm, không phải cam kết view, follower hay doanh thu.

## Trạng thái

TikTok Display API source đã nối cho follower/video totals và current views/likes/comments/shares của public videos khi account cấp `user.info.stats` + `video.list`. Scope và app approval cần được duyệt; account đã kết nối cần OAuth lại.

Trang Growth so sánh follower giữa hai TikTok snapshot mới nhất cách nhau ít nhất 24 giờ, trong cửa sổ 14 ngày. Thiếu snapshot hoặc dữ liệu cũ thì hiện trạng thái chưa đủ dữ liệu; gợi ý chỉ đề xuất thử nghiệm, không khẳng định nguyên nhân tăng/giảm.

Retention, watch-time, demographics, profile visits, follows-per-video và video-level order attribution vẫn chưa có source. Những trường này giữ `Chưa có dữ liệu`; không suy ra từ view count.
