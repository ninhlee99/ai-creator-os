# Video viral TikTok

## Flow

1. TikTok Radar giữ tối đa 2.000 nguồn gần nhất cùng snapshot chỉ số trong SQLite; giao diện chỉ hiện tối đa 10 video.
2. Feed mặc định và topic search dùng [TikTok Web Crawler](tiktok-web-crawler.md) chạy bằng browser headless cục bộ; không cần Access Token TikTok Business. Metrics và sound metadata chỉ có khi phản hồi trang cung cấp.
3. Với kết quả thu thập, mở preview TikTok chính thức ngay trên thẻ Radar. Tự xác nhận nội dung chính thực sự là người nhảy/vũ đạo; caption hoặc tên sound không đủ.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Phân tích storyboard từ các frame mẫu; nêu tín hiệu cấu trúc và giả thuyết có mốc nguồn, kèm thử nghiệm cho video sau. Hiển thị metrics và tốc độ tăng view chỉ khi nguồn cung cấp dữ liệu và Radar có đủ snapshot.
6. Tải video, kiểm tra và lấy audio đúng từ nguồn.
7. Tạo output, QC file/hình/audio và gửi TikTok Draft.
8. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Radar chưa có dance classifier. Hiện backend tự đặt `dance_verified` cho mọi kết quả và thẻ hiển thị xác minh; cờ này không chứng minh vũ đạo. Đây là gap cần xử lý trước automation chọn audio. Crawler có thể thiếu counters/sound metadata; không có field thì giữ unknown. Mỗi topic search chỉ trả đúng topic hoặc lỗi; không dùng kết quả của topic chung thay thế. Các giới hạn nguồn, profile tạm, timeout, lưu trữ và quyền sử dụng ghi trong [tài liệu crawler](tiktok-web-crawler.md). Phân tích Viral đưa yếu tố hình ảnh quan sát được, mốc beat và giả thuyết thử nghiệm; đây không chứng minh nguyên nhân viral. Retention, demographics, profile visits và follows-per-video vẫn chưa có dữ liệu. Nếu không lấy được audio từ đúng video Radar đã chọn, flow phải dừng.

Server yêu cầu cờ video nhảy và Radar thu thập trong 7 ngày trước khi phân tích hoặc tạo job. Nhưng crawler tự bật cờ cho mọi kết quả; hiện gate này không chứng minh người dùng đã xác nhận video.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các giả thuyết chỉ để thiết kế thử nghiệm, không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
