# TikTok Web Crawler

Cập nhật: 2026-10-28. Đây là tài liệu chuẩn cho nguồn thu thập TikTok Radar.

## Nguồn hiện tại

Radar dùng Web Crawler chạy trên Edge, Chrome, Brave hoặc Chromium cục bộ. Crawler mở một browser headless với profile tạm, điều khiển qua Chrome DevTools Protocol, đọc phản hồi mạng của trang tìm kiếm TikTok rồi dùng DOM làm dự phòng. Đây không phải TikTok Business Discovery API hay API công khai được TikTok cam kết ổn định; repo không ghi nhận phê duyệt/quyền sử dụng cho cách thu thập này.

Crawler không dùng Access Token TikTok Business hoặc Advertiser ID. TikTok Display API ở Growth là nguồn riêng, dùng lấy counters của video công khai sau khi người dùng đăng và cấp scope; không dùng làm nguồn Radar.

## Hành vi trong code

- Feed mặc định tìm `xu hướng việt nam`; topic được chuẩn hóa và thêm `việt nam` khi thiếu. Topic tối đa 120 ký tự.
- Nếu topic người dùng không có kết quả, trả lỗi cho topic đó; không thay bằng video của tìm kiếm tổng quát. Chỉ feed mặc định được thử query xu hướng chung.
- Browser có timeout tổng 15 giây. Sau tối đa 7 giây không có phản hồi phù hợp, crawler thử đọc danh sách video đang có trong DOM.
- Network response có thể cung cấp ID, caption, tác giả, ảnh bìa, sound metadata và view/like/comment/share counts. Code trả tối đa 8 kết quả; lượt xem đã biết xếp giảm dần, lượt thích phá hòa, sau đó dùng thứ hạng từ nguồn.
- Radar hiển thị tối đa 8 video mỗi chủ đề: 4 cột × 2 hàng trên desktop, ít cột hơn trên màn hình nhỏ. Thumbnail tải lazy; thiếu video trong danh sách lưu được bổ sung nền, tối đa một crawler headless chạy cùng lúc.
- DOM fallback chỉ cung cấp ID, caption, tác giả và ảnh bìa; metrics và sound metadata không có thì giữ unknown. UI hiển thị gạch cho metrics thiếu và `Chưa có metadata sound` khi thiếu tên sound.
- Nguồn lưu trong SQLite: tối đa 2.000 video, metric snapshots tối đa 30 ngày. Video mới mặc định chưa xác nhận dance; người vận hành phải đánh dấu rõ sau khi xem video. Chưa có classifier.

Âm thanh dùng cho job được trích từ video nguồn đã chọn. Tên sound chỉ là metadata, không chứng minh audio track, quyền sử dụng hay nguồn gốc bản quyền.

## Trạng thái và giới hạn

Cài đặt chỉ báo browser binary đã được tìm thấy; điều đó không bảo đảm trang tải được hoặc crawler thu thập thành công. DOM TikTok, phản hồi mạng và field có thể đổi; kết quả không đảm bảo đủ coverage, cập nhật hay luôn đúng.

Việc crawler chạy được không xác nhận TikTok đã cho phép truy cập tự động. Trước khi dùng production, chủ sản phẩm cần xác minh quyền/phạm vi sử dụng. Nếu chưa xác minh được, không tuyên bố tuân thủ; cần tắt crawl tự động hoặc chuyển sang nguồn được phép. Không bypass đăng nhập, CAPTCHA, access control hay giới hạn nền tảng.

Dance verification là xác nhận thủ công, không phải kết quả crawler hay classifier. Lịch sử cờ auto-mark từ phiên bản cũ được reset một lần khi nâng cấp; cần xem lại video trước khi xác nhận. Chỉ nguồn đã xác nhận mới dùng được cho audio.
