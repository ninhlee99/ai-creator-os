# Video viral TikTok

## Flow

1. TikTok Radar thu thập feed video Việt Nam qua TikWM hoặc tìm video TikTok theo keyword; lưu tối đa 2.000 nguồn gần nhất và snapshot chỉ số trong SQLite.
2. Có thể lọc feed đã thu thập theo từ khóa trong caption TikTok; mọi từ trong chủ đề phải xuất hiện trong caption, không phân biệt dấu. Đây là lọc cục bộ, không phải tìm kiếm từ khóa trực tiếp trên TikTok; không có kết quả chỉ có nghĩa feed hiện tại không khớp.
3. Chọn và tự xác nhận một video nhảy từ kết quả Radar.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Phân tích storyboard từ các frame mẫu; hiển thị lượt xem, lượt thích, bình luận, chia sẻ, tốc độ tăng view và tương tác/view mà Radar có cho video đó.
6. Tải video, kiểm tra và lấy audio đúng từ nguồn.
7. Tạo output, QC file/hình/audio và gửi TikTok Draft.
8. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

Job đã qua QC có nút gửi thủ công vào TikTok Creator Inbox. App lưu trạng thái để tránh gửi trùng; nếu API không xác nhận kết quả, cần kiểm tra Creator Inbox trước khi thử lại. Draft chưa công khai cho tới khi người dùng duyệt trong TikTok.

## Giới hạn hiện tại

Radar không phân loại dance đáng tin; người dùng cần xác nhận video nhảy. Feed và tìm kiếm keyword dùng TikWM, cần kiểm tra độ phủ/freshness khi vận hành. Metadata đã thu thập giữ trong SQLite để nguồn không mất khi cache hết hạn. Nếu chưa chọn đúng video từ Radar hoặc không lấy được audio, flow phải dừng. Không dùng URL ngoài Radar hoặc sound thay thế tự động.

Server xác minh nguồn đã được đánh dấu video nhảy và Radar đã thu thập trong 7 ngày trước khi phân tích hoặc tạo job; bỏ qua nút UI không thể dùng nguồn chưa xác nhận hoặc đã cũ.

Storyboard mô tả bằng chứng hình ảnh theo mốc thời gian và số liệu Radar hiện có. Các số đo không chứng minh nguyên nhân viral; retention, watch-time, demographics, profile visits và follows riêng theo video vẫn chưa có dữ liệu. Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
