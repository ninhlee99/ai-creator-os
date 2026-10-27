# Quy tắc an toàn

Áp dụng cho video affiliate TikTok và video viral TikTok.

## Tài khoản và chi phí

- Chỉ dùng account do người vận hành cho phép.
- Không tạo account mới hoặc đổi account để né quota/chính sách.
- Không gọi provider có billing hoặc giá không rõ.
- Session Web AI connected không chứng minh free plan hay credits còn lại.

## Dữ liệu TikTok

- Tuân theo [crawler contract](features/tiktok-web-crawler.md) về quyền sử dụng production, không bypass giới hạn nền tảng và cách giữ trường dữ liệu chưa được nguồn xác nhận ở trạng thái `unknown`.

## Dữ liệu sản phẩm

- Lưu provider, offer ID, URL và thời điểm cập nhật.
- Chỉ hiển thị sales, rating, review hoặc commission khi provider trả field tương ứng.
- Không biến số lượng kết quả thành số bán; không bịa claim về chất lượng/sản phẩm.

## Video nguồn và âm thanh

- Nguồn video phải thuộc TikTok Radar.
- Video dùng làm sound source phải là video nhảy được xác nhận; nếu chưa chắc, cần người dùng chọn.
- Lưu video ID/URL và metadata audio trong job.
- Không dùng audio thay thế khi tải hoặc trích âm thanh thất bại.
- Không tuyên bố quyền sử dụng audio chỉ từ việc audio xuất hiện trong TikTok.

## Xuất bản

- Preview và QC trước khi gửi Draft.
- Nêu rõ trạng thái draft, lỗi hoặc chưa xác định; không gọi upload là public publish.
- Không retry mù sau timeout có thể xảy ra sau submit.
- Direct Post cần eligibility, controls và consent TikTok được xác minh trước khi mở.

## Metrics

Lưu nguồn, account/video ID, timestamp và khoảng đo. Thiếu số liệu là unknown; không đưa lời khuyên tăng trưởng như kết luận nếu chưa có mẫu.
