# Video viral TikTok

## Flow

1. Nguồn, cách thu thập, lưu trữ và field Radar theo [TikTok Web Crawler](tiktok-web-crawler.md).
2. Với kết quả thu thập, mở preview TikTok chính thức ngay trên thẻ Radar. Xác nhận nội dung chính thực sự có vũ đạo; caption hoặc tên sound không đủ.
3. Dùng video đã chọn làm nguồn/reference cho job.
4. Phân tích storyboard từ các frame mẫu; nêu tín hiệu cấu trúc và giả thuyết có mốc nguồn, kèm thử nghiệm cho video sau. Hiển thị metrics và tốc độ tăng view chỉ khi nguồn cung cấp dữ liệu và Radar có đủ snapshot.
5. Tải video, kiểm tra và lấy audio đúng từ nguồn.
6. Tạo output, QC file/hình/audio và gửi TikTok Draft.
7. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Crawler không phân loại dance; video chưa được xác nhận không dùng làm nguồn. Các giới hạn crawl, unknown fields và quyền sử dụng ghi trong [tài liệu crawler](tiktok-web-crawler.md). Phân tích Viral đưa yếu tố hình ảnh quan sát được, mốc beat và giả thuyết thử nghiệm; đây không chứng minh nguyên nhân viral. Retention, demographics, profile visits và follows-per-video vẫn chưa có dữ liệu. Nếu không lấy được audio từ đúng video Radar đã chọn, flow phải dừng.

Server yêu cầu cờ xác nhận thủ công và Radar thu thập trong 7 ngày trước khi phân tích hoặc tạo job. Cờ cũ không có provenance xác nhận được reset khi nâng cấp.

Renderer hiện hỗ trợ nguồn dài 3–200 giây: tối đa 20 shot Muse, mỗi shot 3–10 giây. Nguồn ngoài giới hạn bị từ chối trước khi xếp job.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các giả thuyết chỉ để thiết kế thử nghiệm, không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
