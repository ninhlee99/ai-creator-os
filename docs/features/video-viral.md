# Video viral TikTok

## Flow

1. TikTok Radar giữ tối đa 2.000 nguồn gần nhất cùng snapshot chỉ số trong SQLite; giao diện chỉ hiện tối đa 10 video.
2. Tìm theo chủ đề: nếu có `APIFY_API_TOKEN`, Radar gửi tối đa 3 cụm từ tới Apify TikTok Scraper, nhận tối đa 20 kết quả mỗi cụm, rồi xếp theo view giảm dần và tym giảm dần khi bằng view. Apify tính phí theo dữ liệu trả về. Nếu chưa có token, dùng TikTok Business Discovery khi đã cấu hình; API chính thức này trả video theo hashtag nhưng không có bộ đếm view/tym. Cuối cùng mới dùng TikWM, vốn có thể bị Cloudflare chặn HTTP 403.
3. Với kết quả Discovery thiếu ảnh bìa, mở preview TikTok chính thức ngay trên thẻ Radar. Tự xác nhận nội dung chính thực sự là người nhảy/vũ đạo; caption hoặc tên sound không đủ.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Phân tích storyboard từ các frame mẫu; nêu tín hiệu cấu trúc và giả thuyết có mốc nguồn, kèm thử nghiệm cho video sau. Hiển thị metrics và tốc độ tăng view chỉ khi nguồn cung cấp dữ liệu và Radar có đủ snapshot.
6. Tải video, kiểm tra và lấy audio đúng từ nguồn.
7. Tạo output, QC file/hình/audio và gửi TikTok Draft.
8. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Radar không phân loại dance đáng tin; người dùng cần xác nhận video nhảy. Apify TikTok Scraper nhận từ khóa và trả view/tym để Radar xếp top 10, nhưng là nguồn bên thứ ba có tính phí và chưa xác minh với token thật. TikTok Business Discovery là nguồn chính thức dự phòng; endpoint trả top 20 video theo hashtag, xếp theo tổ hợp views/comments/likes/shares nhưng không trả counters trong danh sách. Nếu nguồn không có đủ counters, giao diện ghi rõ và giữ thứ tự nguồn cho các video thiếu số liệu. Phân tích Viral đưa yếu tố hình ảnh quan sát được, mốc beat và giả thuyết thử nghiệm; đây không chứng minh nguyên nhân viral. Retention, demographics, profile visits và follows-per-video vẫn chưa có dữ liệu. TikWM topic search có thể bị Cloudflare challenge HTTP 403; app báo rõ và không giả vờ lọc caption để thay thế tìm kiếm trên TikTok. Metadata đã thu thập giữ trong SQLite để nguồn không mất khi cache hết hạn. Nếu chưa chọn đúng video từ Radar hoặc không lấy được audio, flow phải dừng. Không dùng URL ngoài Radar hoặc sound thay thế tự động.

Server xác minh nguồn đã được đánh dấu video nhảy và Radar đã thu thập trong 7 ngày trước khi phân tích hoặc tạo job; bỏ qua nút UI không thể dùng nguồn chưa xác nhận hoặc đã cũ.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các giả thuyết chỉ để thiết kế thử nghiệm, không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
