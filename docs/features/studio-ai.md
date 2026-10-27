# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar dùng Web Crawler cục bộ; field metrics và sound metadata có thể thiếu. Crawler không phân loại video nhảy: video mới ở trạng thái chưa xác nhận cho tới khi người vận hành xem và xác nhận trên card Radar. Chỉ video đã thu thập và xác nhận thủ công mới dùng làm nguồn audio. Xem [tài liệu chuẩn về TikTok Web Crawler](tiktok-web-crawler.md). Viral analysis chỉ nêu tín hiệu quan sát và giả thuyết thử nghiệm, không khẳng định nguyên nhân viral. Audio lấy từ video nguồn đã chọn; không tự thay sound khi tải hoặc tách audio thất bại.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Session và credits phụ thuộc provider; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. Job Affiliate/Viral đã qua QC có thể gửi vào TikTok Creator Inbox từ Studio sau khi người vận hành xem preview, xác nhận account đích và đồng ý gửi Draft. Consent được ghi vào log job. Trạng thái gửi chống tạo draft trùng; kết quả không rõ phải đối chiếu trong TikTok trước khi gửi lại. Draft cần người dùng kiểm tra và duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
