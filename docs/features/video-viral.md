# Video viral TikTok

## Flow

1. TikTok Radar thu thập feed video Việt Nam qua TikWM hoặc tìm video TikTok theo keyword; lưu tối đa 2.000 nguồn gần nhất và snapshot chỉ số trong SQLite.
2. Có thể lọc feed đã thu thập theo từ khóa trong caption TikTok; mọi từ trong chủ đề phải xuất hiện trong caption, không phân biệt dấu. Đây là lọc cục bộ, không phải tìm kiếm từ khóa trực tiếp trên TikTok; không có kết quả chỉ có nghĩa feed hiện tại không khớp.
3. Chọn và tự xác nhận một video nhảy từ kết quả Radar.
4. Dùng video đã chọn làm nguồn/reference cho job.
5. Tải video, kiểm tra và lấy audio đúng từ nguồn.
6. Tạo output, QC file/hình/audio và gửi TikTok Draft.
7. Sau khi tự đăng công khai, dán URL video TikTok vào job đã hoàn tất để Growth ghép counters theo account/video ID.

## Giới hạn hiện tại

Radar không phân loại dance đáng tin; người dùng cần xác nhận video nhảy. Feed và tìm kiếm keyword dùng TikWM, cần kiểm tra độ phủ/freshness khi vận hành. Metadata đã thu thập giữ trong SQLite để nguồn không mất khi cache hết hạn. Nếu chưa chọn đúng video từ Radar hoặc không lấy được audio, flow phải dừng. Không dùng URL ngoài Radar hoặc sound thay thế tự động.

Tạo video viral hoàn chỉnh và giữ đồng nhất với reference chưa được chứng minh end-to-end.

Counters lấy từ TikTok Display API sau khi account cấp quyền và đồng bộ. Retention, watch-time, demographics, profile visits và follows riêng theo video chưa có dữ liệu; chưa có snapshot thì hiển thị unknown.
