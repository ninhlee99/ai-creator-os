# Khoảng cách hiện tại

Cập nhật: 2026-10-27. Phạm vi hiện tại gồm hai flow TikTok; truyện audio là roadmap.

## Flow video affiliate TikTok

- Cần nguồn offer có commission và dữ liệu sản phẩm chính xác.
- AccessTrade hiện có offer/link/order; sales theo kỳ và rating/review per-product chưa được xác minh.
- Cần chọn video nhảy từ Radar, lấy audio đúng nguồn và giữ product truth trong video.
- Tạo video còn phụ thuộc footage/provider; chưa chứng minh chạy miễn phí end-to-end.
- TikTok Draft cần phản ánh đúng kết quả và người dùng phải hoàn tất review trong TikTok.

## Flow video viral TikTok

- Radar hiện dùng Web Crawler cục bộ; cần xác minh quyền production, hiện đúng topic search và không giả metrics/sound khi thiếu. Dance xác nhận thủ công sau khi xem video; chưa có classifier tự động.
- Caption/sound keyword chưa đủ bằng chứng để tự chọn dance clip.
- Cần bảo toàn video/audio nguồn tới artifact cuối và lưu provenance trên job.
- Tạo output viral hoàn chỉnh cần xác minh trên video nguồn thật.

## Thành phần dùng chung

- Kết nối TikTok OAuth và kiểm tra quyền trên deployment thật.
- Web AI login/session không xác nhận gói miễn phí hoặc credits.
- TikTok Display API đã được nối cho follower/video totals và current public-video counters khi user cấp scopes; retention, demographics và video-level orders vẫn thiếu.

## Thứ tự

1. Radar dance và chọn đúng nguồn.
2. Audio extraction/mux cho cả hai flow.
3. Product evidence, link attribution và video QC cho affiliate.
4. TikTok Draft submit, reconciliation và status.
5. TikTok metrics sau khi hai flow ổn định.
6. Mở truyện audio sau khi hai flow đạt acceptance.
