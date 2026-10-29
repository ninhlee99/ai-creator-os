# Tạo Video AI (Universal Workflow Studio)

Trang **Tạo video** (`/workflow/create`, tương thích ngược với `/viral`) là trung tâm sản xuất video AI linh hoạt theo quy trình (Workflow). Người dùng có thể tạo nhiều thể loại video khác nhau (Video nhảy viral, Video review Affiliate, AI Storyteller, v.v.) chỉ bằng việc thay đổi Workflow thực thi.

## Luồng vận hành

1. **Chọn Workflow sản xuất**:
   - `preset_viral_dance`: TikTok Viral Dance (Veo 3.1 & Muse AI) - Thay nhân vật giữ nguyên vũ đạo & âm thanh gốc.
   - `preset_affiliate_daily`: TikTok Affiliate Review (Kling 3.0 & Veo AI) - Video giới thiệu sản phẩm & tính năng.
   - Các Custom Workflow do người dùng tự tạo hoặc nhân bản từ hệ thống.
   - Nút liên kết trực tiếp `🎨 Tùy biến Workflow này` để mở ngay trên Canvas.

2. **Dữ liệu nguồn tự động thích ứng**:
   - Giao diện tự động thay đổi các trường dữ liệu theo Workflow đã chọn:
     - *Viral Dance*: Nhập link TikTok nguồn hoặc chọn từ Xu hướng, tên video, âm thanh nền.
     - *Affiliate*: Nhập link sản phẩm hoặc chọn từ Kệ hàng, góc review, ưu đãi.
     - *Custom*: Nhập chủ đề, kịch bản & prompt hướng dẫn cho AI.
   - Loại bỏ các thành phần rườm rà không cần thiết: Đã xóa nút "Dán", nút "Phân tích Storyboard", và thuật ngữ kỹ thuật "Radar" khỏi giao diện người dùng.

3. **Nhân vật đại diện kênh & Multi-upload**:
   - Chọn kênh TikTok nhận diện (@account).
   - Hiển thị lưới ảnh nhân vật đã có của kênh để người dùng chọn nhiều ảnh góc mặt.
   - Hỗ trợ tải lên cùng lúc **nhiều góc ảnh mới** (chân dung, nghiêng 45°, toàn thân) để AI nhận diện khuôn mặt và trang phục nhất quán.

4. **Thiết lập xuất video**:
   - Độ phân giải: **1080p Full HD** (chuẩn dọc 9:16) hoặc **2K QHD** (upscale sắc nét, giới hạn trần 2K).
   - Chế độ AI: Thay nhân vật giữ nguyên chuyển động hoặc sinh mới theo kịch bản.

5. **Khởi chạy tác vụ**:
   - Bấm **`🚀 Khởi chạy Workflow Tạo Video`**: Job được khởi chạy ngay (hoặc xếp vào hàng đợi nếu hệ thống đang bận render) và chuyển hướng tới màn hình **Lịch sử** (`/studio/jobs`).
