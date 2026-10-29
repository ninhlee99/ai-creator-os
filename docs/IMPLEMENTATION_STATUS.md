# Trạng thái triển khai

Ảnh chụp source: 2026-10-28. Đây là rà soát tĩnh; chưa chứng minh hoạt động với tài khoản/provider thật.

## Hai flow đang làm

| Bước | Trạng thái | Giới hạn hiện tại |
|---|---|---|
| Kết nối TikTok | Một phần | OAuth client dùng Go DNS resolver để tránh lỗi resolver hệ thống trên macOS. Smoke server đã chạy build mới; OAuth với tài khoản TikTok thật, quyền app và callback deployment vẫn chưa xác minh. |
| Radar video TikTok | Một phần | Web Crawler lấy tối đa 32 ứng viên; Gemini Web, ChatGPT Web, Claude Web, Grok Web phân tích 4 frame và caption khi account kết nối; Gemini API là fallback. Tab mặc định lọc dance; topic khác lọc relevance. Refresh ghi position 1–8 và chuyển mapping cũ về 0; non-dance bị gỡ riêng khỏi mapping tab mặc định. Quyền sử dụng production và độ ổn định trên môi trường thật chưa xác minh. Hành vi, fallback, giới hạn và unknown fields: [tài liệu crawler](features/tiktok-web-crawler.md). |
| Chọn video nhảy | AI gợi ý + xác nhận thủ công | Radar lưu dance confidence và bằng chứng; người vận hành vẫn phải mở/xác nhận trước khi dùng làm source audio. Backend yêu cầu xác nhận và freshness dưới 7 ngày. |
| Quản lý sản phẩm Affiliate | Hoàn thành tinh gọn | Nhập sản phẩm trực tiếp từ 1 link affiliate duy nhất (Shopee, TikTok Shop, Lazada...). Hệ thống tự bóc tách ảnh, tên, giá, hoa hồng và lưu vào kệ hàng. Loại bỏ Accesstrade hunter, tab chờ duyệt và loại bỏ. Hỗ trợ tạo video 1-click từ kệ hàng. |
| Tạo video bằng 1 câu lệnh (1-Prompt Studio) | Hoàn thành tinh gọn | Route độc nhất `/`. Tự động phân tích câu lệnh (Auto-Detect) để kích hoạt workflow phù hợp (Affiliate hoặc TikTok Viral). Form kéo thả upload nhiều ảnh (preview trực quan) và video mẫu, giới hạn Upscale tối đa 2K (loại bỏ 4K/8K). |
| Lấy audio Radar | Một phần | Code tải video nguồn, tách audio và kiểm tra track; cần xác minh bằng video thật và đầu ra cuối. |
| Gửi TikTok Draft | Một phần | Studio hiện hiển thị account đích, bắt buộc người vận hành xác nhận đã xem preview và đồng ý gửi Draft, rồi ghi consent vào log job. Trạng thái gửi chống tạo draft trùng và kết quả không rõ cần đối chiếu trước retry. Chưa chứng minh tài khoản/app thật đủ scope. Draft vẫn cần người dùng hoàn tất trong TikTok; public publish và product anchor chưa nối. |
| Analytics/gợi ý | Một phần | TikTok Display API đã nối follower/video totals và current public-video views/likes/comments/shares; Growth so delta follower giữa snapshot cách nhau ít nhất 24 giờ. Job hoàn tất nhận link TikTok công khai; chỉ xem video đã xác minh trong snapshot `video.list` mới của đúng kênh và chỉ sau đó ghép đơn Affiliate theo `sub2`. Engagement baseline so video cùng flow/kênh, hiển thị số mẫu; dưới 3 video đối chiếu thì chưa kết luận cao/thấp. Chưa có retention, demographics, profile visits hoặc follows-per-video. TikTok Shop product card vẫn cần người dùng gắn thủ công trong TikTok. |
| Workflow truyện audio | Có preset viết → TTS → trộn nhạc tùy chọn → WAV | Dùng LLM và TTS chain đang cấu hình; chưa có dựng nhiều vai, âm thanh theo cảnh hoặc workflow sản xuất audio riêng trong Studio. Chưa xác minh end-to-end với provider thật. |

## Acceptance chưa xác minh

1. Xác minh quyền sử dụng Web Crawler và độ ổn định Radar trên môi trường production.
2. Chạy hai flow với video thật để xác nhận audio lấy từ đúng video Radar và còn trong MP4 cuối.
3. Xác minh TikTok OAuth scopes, upload Draft và reconciliation timeout bằng account/app thật.
4. Xác minh Display API scopes và metrics bằng account thật; retention, demographics và order attribution chỉ mở khi có nguồn.
5. Xác minh chi phí/provider credits cho video affiliate end-to-end.

## Giới hạn chung

Provider Web AI có thể hỗ trợ tạo media nhưng trạng thái session không chứng minh gói miễn phí hoặc credits. Flow/Muse không được tự dùng dịch vụ tính phí. Không có đủ nguồn hợp lệ thì dừng job và báo rõ.
