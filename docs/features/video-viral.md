# Video viral TikTok

## Flow

1. TikTok Radar giữ tối đa 2.000 nguồn gần nhất cùng snapshot chỉ số trong SQLite; giao diện chỉ hiện tối đa 10 video.
2. Feed mặc định và topic search dùng TikTok Business Discovery. API trả hashtag phổ biến và video xếp hạng theo hashtag tại VN; topic search chỉ khớp hashtag đang thịnh hành, không tìm toàn bộ TikTok. Nguồn không trả bộ đếm view/tym nên giao diện giữ các chỉ số này là unknown. Radar không tự động quét TikTok bằng Playwright, không reverse-engineer endpoint riêng và không gọi scraper bên thứ ba. Nếu chưa cấp quyền Discovery, Radar yêu cầu cấu hình nguồn.
3. Với kết quả Discovery thiếu ảnh bìa, mở preview TikTok chính thức ngay trên thẻ Radar. Tự xác nhận nội dung chính thực sự là người nhảy/vũ đạo; caption hoặc tên sound không đủ.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Phân tích storyboard từ các frame mẫu; nêu tín hiệu cấu trúc và giả thuyết có mốc nguồn, kèm thử nghiệm cho video sau. Hiển thị metrics và tốc độ tăng view chỉ khi nguồn cung cấp dữ liệu và Radar có đủ snapshot.
6. Tải video, kiểm tra và lấy audio đúng từ nguồn.
7. Tạo output, QC file/hình/audio và gửi TikTok Draft.
8. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Radar không phân loại dance đáng tin; người dùng cần xác nhận video nhảy. TikTok Business Discovery là nguồn duy nhất: endpoint trả video theo hashtag phổ biến, xếp hạng theo tín hiệu tổng hợp nhưng không trả counters. Giao diện ghi rõ giới hạn và giữ view/tym là unknown. Discovery cần quyền API for Business; nếu chưa cấu hình, Radar không tải video. Phân tích Viral đưa yếu tố hình ảnh quan sát được, mốc beat và giả thuyết thử nghiệm; đây không chứng minh nguyên nhân viral. Retention, demographics, profile visits và follows-per-video vẫn chưa có dữ liệu. Metadata đã thu thập giữ trong SQLite để nguồn không mất khi cache hết hạn. Nếu chưa chọn đúng video từ Radar hoặc không lấy được audio, flow phải dừng. Không dùng URL ngoài Radar hoặc sound thay thế tự động.

Server xác minh nguồn đã được đánh dấu video nhảy và Radar đã thu thập trong 7 ngày trước khi phân tích hoặc tạo job; bỏ qua nút UI không thể dùng nguồn chưa xác nhận hoặc đã cũ.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các giả thuyết chỉ để thiết kế thử nghiệm, không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
