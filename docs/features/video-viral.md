# Video viral TikTok

## Flow

1. TikTok Radar lấy feed video Việt Nam và lưu tối đa 2.000 nguồn gần nhất cùng snapshot chỉ số trong SQLite.
2. Khi cấu hình TikTok Business Discovery Access Token + Advertiser ID, Radar lấy hashtag phổ biến tại Việt Nam rồi tìm video top theo hashtag/chủ đề. Nếu chưa cấu hình, Radar dùng TikWM; endpoint tìm theo chủ đề hiện bị Cloudflare 403 và không thay bằng lọc caption cục bộ.
3. Mở video và tự xác nhận nội dung chính thực sự là người nhảy/vũ đạo; caption hoặc tên sound không đủ.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Phân tích storyboard từ các frame mẫu; hiển thị metrics và tốc độ tăng view chỉ khi nguồn cung cấp dữ liệu và Radar có đủ snapshot.
6. Tải video, kiểm tra và lấy audio đúng từ nguồn.
7. Tạo output, QC file/hình/audio và gửi TikTok Draft.
8. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Radar không phân loại dance đáng tin; người dùng cần xác nhận video nhảy. TikTok Business Discovery có endpoint chính thức trả top 20 video cho hashtag, xếp hạng theo tổ hợp views/comments/likes/shares; tích hợp cần access token và advertiser ID, hiện chưa xác minh với credential thật. API trả video ID/URL, không trả counters trong danh sách này nên metrics vẫn unknown nếu không có nguồn khác. TikWM topic search hiện bị Cloudflare challenge HTTP 403; app báo rõ và không giả vờ lọc caption để thay thế tìm kiếm trên TikTok. Metadata đã thu thập giữ trong SQLite để nguồn không mất khi cache hết hạn. Nếu chưa chọn đúng video từ Radar hoặc không lấy được audio, flow phải dừng. Không dùng URL ngoài Radar hoặc sound thay thế tự động.

Server xác minh nguồn đã được đánh dấu video nhảy và Radar đã thu thập trong 7 ngày trước khi phân tích hoặc tạo job; bỏ qua nút UI không thể dùng nguồn chưa xác nhận hoặc đã cũ.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các số đo không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
