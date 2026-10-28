# TikTok Web Crawler

Cập nhật: 2026-10-28. Đây là tài liệu chuẩn cho nguồn thu thập TikTok Radar.

## Nguồn hiện tại

Radar dùng Web Crawler chạy trên Edge, Chrome, Brave hoặc Chromium cục bộ. Crawler mở một browser headless với profile tạm, điều khiển qua Chrome DevTools Protocol, đọc phản hồi mạng của trang tìm kiếm TikTok rồi dùng DOM làm dự phòng. Đây không phải TikTok Business Discovery API hay API công khai được TikTok cam kết ổn định; repo không ghi nhận phê duyệt/quyền sử dụng cho cách thu thập này.

Crawler không dùng Access Token TikTok Business hoặc Advertiser ID. TikTok Display API ở Growth là nguồn riêng, dùng lấy counters của video công khai sau khi người dùng đăng và cấp scope; không dùng làm nguồn Radar.

## Hành vi trong code

- Feed mặc định tìm `xu hướng việt nam`; topic được chuẩn hóa và thêm `việt nam` khi thiếu. Topic tối đa 120 ký tự.
- Nếu topic người dùng không có kết quả, trả lỗi cho topic đó; không thay bằng video của tìm kiếm tổng quát. Chỉ feed mặc định được thử query xu hướng chung.
- Browser có timeout tổng 15 giây. Sau tối đa 7 giây không có phản hồi phù hợp, crawler thử đọc danh sách video đang có trong DOM.
- Network response có thể cung cấp ID, caption, tác giả, ảnh bìa, sound metadata và view/like/comment/share counts. Crawler giữ tối đa 32 ứng viên để bù các video không khớp chủ đề hoặc không có bằng chứng vũ đạo.
- Radar tải video nguồn qua media downloader hiện có, trích 4 frame theo thứ tự thời gian và gửi frame cùng caption/sound để phân tích. Ưu tiên mọi account đã kết nối theo thứ tự Gemini Web, ChatGPT Web, Claude Web, Grok Web; kết quả hợp lệ được gộp theo đồng thuận. Chỉ khi không có phản hồi WebAI dùng được mới gọi Gemini API key. Caption và sound không đủ để xác nhận vũ đạo. Chỉ tab mặc định `Xu hướng Việt Nam` (tên alias `Việt Nam trending`) lọc dance confidence từ 0.72; chủ đề khác chỉ lọc relevance từ 0.55, không yêu cầu video nhảy để xuất hiện.
- Top 8 xếp theo tốc độ tăng view đã quan sát trong snapshot (tối đa 7 ngày), rồi engagement và tổng lượt xem. Khi chưa có hai snapshot hợp lệ, fallback engagement rồi tổng lượt xem; số thiếu giữ unknown. Đây là tín hiệu xếp hạng, không phải dự báo hay bằng chứng nguyên nhân viral.
- Radar hiển thị tối đa 8 video mỗi chủ đề: 4 cột × 2 hàng trên desktop, ít cột hơn trên màn hình nhỏ. Refresh là job Studio lưu bền trong SQLite; khi trang mở, topic refresh tự chạy mỗi 15 phút, UI poll mỗi 4 giây lúc job đang chạy. Nếu app khởi động lại, job queued/running được resume sau khi nối WebAI; tối đa một crawler headless chạy cùng lúc.
- DOM fallback chỉ cung cấp ID, caption, tác giả và ảnh bìa; metrics và sound metadata không có thì giữ unknown. UI hiển thị gạch cho metrics thiếu và `Chưa có metadata sound` khi thiếu tên sound.
- Nguồn lưu trong SQLite theo TikTok video ID: video trùng ID cập nhật metadata hiện có; phân tích lưu riêng theo topic. Mỗi lần refresh ghi TOP 1–8 vào `rank`; video cũ được giữ với `rank=0`. Video được phân loại không nhảy với confidence từ 0.72 bị xóa khỏi mapping của tab mặc định; video gốc và mapping topic khác vẫn được giữ. Chỉ topic tùy chỉnh được xóa; tab dance mặc định được bảo vệ. Kho giữ tối đa 2.000 video, metric snapshots tối đa 30 ngày. Ở tab mặc định, người vận hành vẫn phải mở và xác nhận video trước khi dùng audio.

Âm thanh dùng cho job được trích từ video nguồn đã chọn. Tên sound chỉ là metadata, không chứng minh audio track, quyền sử dụng hay nguồn gốc bản quyền.

## Trạng thái và giới hạn

Cài đặt chỉ báo browser binary đã được tìm thấy; điều đó không bảo đảm trang tải được hoặc crawler thu thập thành công. DOM TikTok, phản hồi mạng và field có thể đổi; kết quả không đảm bảo đủ coverage, cập nhật hay luôn đúng.

Việc crawler chạy được không xác nhận TikTok đã cho phép truy cập tự động. Trước khi dùng production, chủ sản phẩm cần xác minh quyền/phạm vi sử dụng. Nếu chưa xác minh được, không tuyên bố tuân thủ; cần tắt crawl tự động hoặc chuyển sang nguồn được phép. Không bypass đăng nhập, CAPTCHA, access control hay giới hạn nền tảng.

Dance verification là xác nhận thủ công tách biệt với AI classification. Lịch sử cờ auto-mark từ phiên bản cũ được reset một lần khi nâng cấp; cần xem lại video trước khi xác nhận. Chỉ nguồn đã xác nhận mới dùng được cho audio.
