# TikTok Radar video and audio source spec

Cập nhật: 2026-10-28. Áp dụng cho nguồn video viral và nguồn audio affiliate TikTok.

Radar source contract và giới hạn crawler: [TikTok Web Crawler](features/tiktok-web-crawler.md).

## Mục tiêu

Dùng video đã thu thập trong Radar làm reference viral; dùng sound từ video nhảy đã xác nhận cho affiliate.

## Nguồn hợp lệ

- Video phải được TikTok Radar thu thập và lưu; audio affiliate còn cần xác nhận dance trong thời hạn cho phép.
- Lưu TikTok video ID, URL nguồn, tác giả, thời điểm thu thập và metadata audio.
- Chỉ tab mặc định `Xu hướng Việt Nam` lọc video nhảy. Topic người dùng tự thêm chỉ lọc độ khớp chủ đề; không cần xác nhận dance để dùng video làm reference viral.
- Dùng audio affiliate cần video từ tab mặc định, bằng chứng frame do AI và xác nhận thủ công. Caption hoặc tên sound có từ khóa dance chưa đủ bằng chứng.

## Dùng trong hai flow

- Affiliate: người vận hành chọn video nhảy đã xác nhận trong Radar; Studio tách audio từ đúng URL đó rồi ghép vào video sản phẩm.
- Viral: video Radar đã chọn là reference; topic tùy chỉnh không cần xác nhận dance. Giữ liên kết tới video nguồn và audio tương ứng.
- Hai flow không được tự đổi sang sound từ một video viral khác nếu tải nguồn thất bại.

## Kiểm tra bắt buộc

1. Xác minh URL là video TikTok đã thu thập trong Radar. Audio affiliate cần thêm dance confirmation còn mới.
2. Tải video nguồn; kiểm tra file và có audio track.
3. Trích audio, ghép vào output; kiểm tra output phát được và audio không rỗng.
4. Lưu source video ID/URL, audio metadata và trạng thái từng bước vào job.
5. Khi bất kỳ bước nào thất bại, dừng và báo lý do. Không fallback sang danh sách nhạc, sound bất kỳ hoặc audio giả.

## Acceptance

Viral flow nhận video từ topic tùy chỉnh làm reference mà không yêu cầu xác nhận dance. Affiliate chỉ dùng audio từ video nhảy đã xác nhận. Đầu ra phải giữ nguồn/audio tương ứng; job detail cho biết nguồn dùng; log không chứa credential/session. Khi tab mặc định không có video nhảy hợp lệ, UI yêu cầu chọn/làm mới Radar thay vì tự chọn video khác.
