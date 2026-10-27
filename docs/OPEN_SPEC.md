# TikTok Radar audio source spec

Cập nhật: 2026-10-24. Áp dụng cho video affiliate TikTok và video viral TikTok.

Radar source contract và giới hạn crawler: [TikTok Web Crawler](features/tiktok-web-crawler.md).

## Mục tiêu

Chọn âm thanh từ video nhảy viral mà Radar đã thu thập.

## Nguồn hợp lệ

- Video phải có trong kết quả TikTok Radar hiện tại.
- Lưu TikTok video ID, URL nguồn, tác giả, thời điểm thu thập và metadata audio.
- Chỉ dùng video nhảy. Caption hoặc tên sound có từ khóa dance chưa đủ bằng chứng để tự coi là video nhảy.
- Radar hiện không có classifier; người vận hành phải xem và xác nhận video đúng trong Radar.

## Dùng trong hai flow

- Affiliate: người vận hành chọn video nhảy Radar; Studio tách audio từ đúng URL đó rồi ghép vào video sản phẩm.
- Viral: video Radar đã chọn là reference; giữ liên kết tới nguồn và audio tương ứng.
- Hai flow không được tự đổi sang sound từ một video viral khác nếu tải nguồn thất bại.

## Kiểm tra bắt buộc

1. Xác minh URL là video TikTok và video thuộc kết quả Radar.
2. Tải video nguồn; kiểm tra file và có audio track.
3. Trích audio, ghép vào output; kiểm tra output phát được và audio không rỗng.
4. Lưu source video ID/URL, audio metadata và trạng thái từng bước vào job.
5. Khi bất kỳ bước nào thất bại, dừng và báo lý do. Không fallback sang danh sách nhạc, sound bất kỳ hoặc audio giả.

## Acceptance

Với một video Radar được chọn, đầu ra cả hai flow phải mang audio từ đúng video đó. Job detail phải cho biết nguồn dùng; log không chứa credential/session. Khi feed không có video nhảy hợp lệ, UI phải yêu cầu chọn/làm mới Radar thay vì tự chọn video khác.
