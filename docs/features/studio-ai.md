# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar chỉ gọi TikTok Business Discovery sau khi cấu hình Access Token và Advertiser ID trong Cài đặt → Kênh mạng xã hội. API chính thức trả hashtag phổ biến và video xếp hạng theo hashtag tại Việt Nam, không trả counters. Topic search khớp hashtag đang thịnh hành, không phải tìm toàn bộ TikTok. Radar không gọi scraper bên thứ ba, không tự động quét bằng Playwright và không reverse-engineer endpoint TikTok. Nếu chưa cấp quyền Discovery, Radar yêu cầu cấu hình nguồn; không dùng TikWM. Metadata và lịch sử chỉ số lưu trong SQLite. Tích hợp Discovery chưa được xác minh với token thật. Radar không tự phân loại video nhảy; người dùng xác nhận video nhảy làm nguồn. Viral analysis hiển thị tín hiệu cấu trúc có mốc frame và giả thuyết thử nghiệm, không khẳng định nguyên nhân viral. Mọi audio dùng trong hai flow phải lấy từ video nhảy đã xác nhận, được Radar thu thập trong 7 ngày; không dùng sound thay thế tự động.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Session và credits phụ thuộc provider; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. Job Affiliate/Viral đã qua QC có thể gửi vào TikTok Creator Inbox từ Studio sau khi người vận hành xem preview, xác nhận account đích và đồng ý gửi Draft. Consent được ghi vào log job. Trạng thái gửi chống tạo draft trùng; kết quả không rõ phải đối chiếu trong TikTok trước khi gửi lại. Draft cần người dùng kiểm tra và duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
