# Video 3 — Phát triển kênh (`/growth`)

**Mục tiêu:** kế hoạch 30 ngày tự sinh, ngưỡng kill/double-down chỉnh được, sản xuất tự động trong quota.

## Chuẩn bị

- Đã có 1 kênh (video 2). Dry-run BẬT.

## Kịch bản

**Bước 1 — Tổng quan.**
[Thao tác: mở Phát triển kênh.]
"Đây là máy tăng trưởng: nó lo kế hoạch nội dung, lo sản xuất, lo đăng, và lo dừng đúng lúc."

**Bước 2 — Kế hoạch 30 ngày.**
[Thao tác: chọn kênh, bấm "Sinh kế hoạch".]
"Tôi bấm sinh kế hoạch 30 ngày. Máy chia theo giai đoạn: kênh mới thì giai đoạn khởi động lạnh — ít video, thăm dò; kênh có số liệu thì scale."
[Thao tác: chỉ vào danh sách plan items (topic, loại video, ngày dự kiến).]
"Mỗi dòng là một ý tưởng đã lên lịch. Một ý tưởng tự tách thành 3 bản: TikTok 60 giây, Shorts 45 giây, bản dài 16:9 — mỗi bản là file riêng, không trùng nội dung."

**Bước 3 — Ngưỡng governance.**
[Thao tác: mở khối ngưỡng, chỉ vào các số mặc định.]
"Luật chơi nằm ở đây và bạn chỉnh được: ví dụ 8 video mà dưới 35% người xem hết thì máy tự tạm dừng kênh để không đốt tài nguyên. Gặp video gấp 5 lần trung bình thì nhân bản mô-típ đó."
[Thao tác: sửa 1 ngưỡng, lưu, rồi bấm "Reset về mặc định".]
"Đổi xong lưu là có hiệu lực ngay. Muốn về chuẩn thì reset một nút."

**Bước 4 — Sản xuất tự động.**
[Thao tác: chỉ vào nút bật/tắt "Sản xuất tự động" và nút "Chạy ngay".]
"Sản xuất tự động mặc định BẬT — bạn không phải bật gì cả. Mỗi 5 phút máy kiểm tra một lần, mỗi lần tối đa 3 video một kênh."
[Thao tác: bấm "Chạy ngay" (dry-run).]
"Tôi bấm chạy ngay — dry-run nên máy chỉ ghi log 'đáng lẽ đã sản xuất', không tốn key."

**Bước 5 — Quota YouTube.**
[Thao tác: chỉ vào chỗ hiện quota đã dùng / trần ngày.]
"YouTube cho 10.000 đơn vị quota một ngày, một lượt đăng tốn 1.600. Hết quota thì video xếp hàng 'chờ quota', mai đăng tiếp — không bao giờ đăng lố."

**Bước 6 — Trạng thái item trung thực.**
[Thao tác: chỉ vào các badge trạng thái: chờ sản xuất, đang sản xuất, chờ kết nối, chờ quota, đã đăng.]
"Video nào thiếu OAuth thì đứng ở 'chờ kết nối' kèm lý do thiếu cái gì. Không có chuyện app báo đăng thành công trong khi chưa đăng."

## Lưu ý trung thực

- Cần key Gemini thật mới demo được sản xuất video thật; không có thì dừng ở dry-run.
- Nói rõ: TikTok mặc định chỉ làm tới bản nháp (draft) — bạn gắn giỏ hàng và nhạc trong app TikTok rồi mới public, vì API TikTok không cho tự gắn giỏ.
- Không hứa số follower hay view.
