# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar ưu tiên Apify TikTok Scraper khi có `APIFY_API_TOKEN`: feed mặc định tìm theo ba cụm từ xu hướng Việt Nam, topic search nhận tối đa ba cụm từ, mỗi cụm tối đa 20 video; Radar xếp top 10 theo view rồi tym. Apify tính phí theo kết quả. Nếu chưa có Apify token, Radar dùng TikTok Business Discovery khi cấu hình access token và advertiser ID; API chính thức lấy hashtag phổ biến và video theo hashtag tại VN nhưng không trả counters. Nếu chưa cấu hình một trong hai nguồn, Radar yêu cầu cấu hình trong Cài đặt → Kênh mạng xã hội; không dùng TikWM. Metadata và lịch sử chỉ số lưu trong SQLite. Tích hợp Discovery và Apify chưa được xác minh với token thật. Radar không tự phân loại video nhảy; người dùng xác nhận video nhảy làm nguồn. Viral analysis hiển thị tín hiệu cấu trúc có mốc frame và giả thuyết thử nghiệm, không khẳng định nguyên nhân viral. Mọi audio dùng trong hai flow phải lấy từ video nhảy đã xác nhận, được Radar thu thập trong 7 ngày; không dùng sound thay thế tự động.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Session và credits phụ thuộc provider; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. Job Affiliate/Viral đã qua QC có thể gửi vào TikTok Creator Inbox từ Studio sau khi người vận hành xem preview, xác nhận account đích và đồng ý gửi Draft. Consent được ghi vào log job. Trạng thái gửi chống tạo draft trùng; kết quả không rõ phải đối chiếu trong TikTok trước khi gửi lại. Draft cần người dùng kiểm tra và duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
