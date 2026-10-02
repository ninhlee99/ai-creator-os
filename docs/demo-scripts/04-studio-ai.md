# Video 4 — Studio AI (`/studio`)

**Mục tiêu:** tạo 1 video affiliate (ảnh + nhạc, không chữ không voiceover — format đã chốt) và 1 video chữ động; xem Jobs và bảng nhạc trending.

## Chuẩn bị

- Cần `GEMINI_API_KEYS` thật (free tier) + 1 ảnh sản phẩm + 1 ảnh người mẫu + 1 file nhạc licensed. Dry-run TẮT cho video này (muốn job chạy thật) — hoặc để dry-run và demo tới bước "media-gen sẵn sàng".

## Kịch bản

**Bước 1 — Kiểm tra media-gen.**
[Thao tác: mở Studio AI, chỉ vào trạng thái media-gen.]
"Trước khi làm, app báo media-gen sẵn sàng hay chưa. Chưa có key thì job sẽ fail ngay với lý do rõ ràng, không render bừa tốn tiền."

**Bước 2 — Tạo video affiliate.**
[Thao tác: tab Video affiliate, upload ảnh sản phẩm + ảnh người mẫu, upload/chọn nhạc, bấm tạo.]
"Tôi tạo video affiliate: tải ảnh sản phẩm thật, ảnh người mẫu để khóa mặt, chọn nhạc bản quyền. Format đã chốt: không chữ, không thuyết minh — chỉ video và nhạc."
[Thao tác: sang trang Jobs, xem tiến trình.]
"Máy viết shot list, chụp từng ảnh cùng một địa điểm, dựng Ken Burns rồi lồng nhạc. Tối đa 2 job render cùng lúc, job thừa xếp hàng 'queued' trung thực."

**Bước 3 — Xem kết quả.**
[Thao tác: job xong → bấm xem video, bấm nút Copy caption.]
"Xong. Caption và hashtag tiếng Việt do AI viết sẵn, bấm copy là đăng."

**Bước 4 — Video chữ động.**
[Thao tác: chuyển tab Video chữ động, nhập tiêu đề demo, bấm tạo, xem ở Jobs.]
"Tab này làm video chữ động nhanh — hợp làm teaser, thông báo."

**Bước 5 — Nhạc trending.**
[Thao tác: mở bảng nhạc trending VN, bấm "Làm mới".]
"Bảng nhạc trending TikTok Việt Nam — app ghi rõ đây là chart bên thứ ba, không phải số liệu chính thức của TikTok. Cache 6 giờ, bấm làm mới thì fetch lại."

## Lưu ý trung thực

- **Cần key Gemini thật trên máy Ninh mới quay được job chạy thật**; không có key thì chỉ demo tới bước fail-closed.
- Nói rõ mỗi ảnh upscale 4K; upscale lỗi thì dùng bản gốc chứ không fail job.
- Không hứa video "giống quay thật 100%".
