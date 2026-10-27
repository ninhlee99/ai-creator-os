# Trạng thái triển khai

Ảnh chụp source: 2026-10-28. Đây là rà soát tĩnh; chưa chứng minh hoạt động với tài khoản/provider thật.

## Hai flow đang làm

| Bước | Trạng thái | Giới hạn hiện tại |
|---|---|---|
| Kết nối TikTok | Một phần | OAuth client dùng Go DNS resolver để tránh lỗi resolver hệ thống trên macOS. Smoke server đã chạy build mới; OAuth với tài khoản TikTok thật, quyền app và callback deployment vẫn chưa xác minh. |
| Radar video TikTok | Một phần | Web Crawler là nguồn hiện tại; quyền sử dụng production và độ ổn định trên môi trường thật chưa xác minh. Hành vi, fallback, giới hạn và unknown fields: [tài liệu crawler](features/tiktok-web-crawler.md). |
| Chọn video nhảy | Xác nhận thủ công | Mỗi video cần được mở và xác nhận trước khi dùng làm source audio; backend yêu cầu xác nhận và freshness dưới 7 ngày. Crawler contract ghi hành vi và reset dữ liệu cũ; chưa có classifier. |
| Radar sản phẩm và duyệt | Một phần | TikTok Shop feed qua AccessTrade tìm theo category keyword, sort commission; khi có `commission.amount`, lưu và hiển thị amount/currency nguồn; chỉ ước tính rate × giá khi thiếu amount. `units_sold` hiển thị là tổng từ feed; feed không nêu kỳ đo. Khi có hai snapshot app cách nhau 6 giờ–14 ngày, UI tính chênh lệch quan sát được và ghi rõ khả năng trễ từ nguồn. Offer thiếu giá không bị bỏ khỏi kho. Currency/provider-updated-at/sales hiển thị unknown nếu nguồn không trả; rating/review không có trong feed hiện tại. Duyệt chặn offer thiếu giá, currency, commission hoặc link listing. Có thao tác loại kèm lý do và danh sách xem lại; tín hiệu xu hướng tách match tên sản phẩm/ngành hàng từ Radar nhưng chưa chấm độ phù hợp hoặc tệp tuổi. Sản xuất tự động cần offer đã duyệt và demand evidence còn hạn. |
| Tạo video affiliate | Một phần | Shot plan/QC có; một số nhánh vẫn cần clip chuyển động do người dùng cung cấp hoặc provider cần credits. Chưa chứng minh tạo miễn phí end-to-end. |
| Tạo video viral | Một phần | Nguồn video được ràng buộc vào Radar; phân tích trả tối đa 4 yếu tố cấu trúc quan sát có mốc beat và 3 thử nghiệm một biến cho video sau. Mốc được xác thực lại với beat nguồn; giả thuyết không chứng minh nguyên nhân viral. Metrics/retention/audience chỉ hiện khi có nguồn; tạo bản video hoàn chỉnh và giữ đúng audio cần xác minh end-to-end. |
| Lấy audio Radar | Một phần | Code tải video nguồn, tách audio và kiểm tra track; cần xác minh bằng video thật và đầu ra cuối. |
| Gửi TikTok Draft | Một phần | Studio hiện hiển thị account đích, bắt buộc người vận hành xác nhận đã xem preview và đồng ý gửi Draft, rồi ghi consent vào log job. Trạng thái gửi chống tạo draft trùng và kết quả không rõ cần đối chiếu trước retry. Chưa chứng minh tài khoản/app thật đủ scope. Draft vẫn cần người dùng hoàn tất trong TikTok; public publish và product anchor chưa nối. |
| Analytics/gợi ý | Một phần | TikTok Display API đã nối follower/video totals và current public-video views/likes/comments/shares; Growth so delta follower giữa snapshot cách nhau ít nhất 24 giờ. Job hoàn tất nhận link TikTok công khai; chỉ xem video đã xác minh trong snapshot `video.list` mới của đúng kênh và chỉ sau đó ghép đơn AccessTrade theo `sub2`. Engagement baseline so video cùng flow/kênh, hiển thị số mẫu; dưới 3 video đối chiếu thì chưa kết luận cao/thấp. Chưa có retention, demographics, profile visits hoặc follows-per-video. TikTok Shop product card vẫn cần người dùng gắn thủ công trong TikTok. |
| Workflow truyện audio | Có preset viết → TTS → trộn nhạc tùy chọn → WAV | Dùng LLM và TTS chain đang cấu hình; chưa có dựng nhiều vai, âm thanh theo cảnh hoặc workflow sản xuất audio riêng trong Studio. Chưa xác minh end-to-end với provider thật. |

## Acceptance chưa xác minh

1. Xác minh quyền sử dụng Web Crawler và độ ổn định Radar trên môi trường production.
2. Chạy hai flow với video thật để xác nhận audio lấy từ đúng video Radar và còn trong MP4 cuối.
3. Xác minh TikTok OAuth scopes, upload Draft và reconciliation timeout bằng account/app thật.
4. Xác minh Display API scopes và metrics bằng account thật; retention, demographics và order attribution chỉ mở khi có nguồn.
5. Xác minh chi phí/provider credits cho video affiliate end-to-end.

## Giới hạn chung

Provider Web AI có thể hỗ trợ tạo media nhưng trạng thái session không chứng minh gói miễn phí hoặc credits. Flow/Muse không được tự dùng dịch vụ tính phí. Không có đủ nguồn hợp lệ thì dừng job và báo rõ.
