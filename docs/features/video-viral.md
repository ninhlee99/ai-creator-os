# Video Viral AI 100% Gốc (Trendjack Spotlight Shorts 9:16)

> **Mô hình thay thế vượt trội cho Reup**: Chuyển đổi toàn diện từ việc cào/tải lại video của bên thứ ba (vốn chịu rủi ro bản quyền, anti-bot Douyin, thuật toán phạt 0-view shadowban của TikTok/Shorts) sang **Xưởng Sáng Tạo Video Viral Nguyên Bản 100%** (100% Original Assets).

## 1. Triết lý & Giá trị kinh doanh

- **Zero Platform Risk (0% rủi ro vi phạm nền tảng)**:
  - 100% kịch bản do AI viết dựa trên tâm lý học hành vi và công thức giữ chân người xem (Retention Hooks).
  - 100% hình ảnh/video phân cảnh do AI MediaGen (Gemini/Imagen) sinh độc quyền, chuẩn tỉ lệ 9:16.
  - 100% giọng đọc lồng tiếng bản ngữ truyền cảm qua VieNeu TTS / Gemini Audio.
  - Âm nhạc thịnh hành an toàn bản quyền từ bảng xếp hạng Kworb VN.
- **Tiết kiệm thời gian cho Chủ doanh nghiệp**:
  - Không cần ngồi tìm trend Douyin, không cần lo proxy/anti-captcha vượt rào cản IP Trung Quốc.
  - Chế độ **1-Chạm (1-Click Trendjack Launcher)**: Bấm 1 nút từ các chủ đề hot trong ngày, hệ thống tự động sinh kịch bản, vẽ cảnh, lồng tiếng và dựng video hoàn chỉnh.
  - Dây chuyền tự động hóa: Tự quét xu hướng và lên lịch sản xuất hàng ngày theo Brand DNA của từng kênh.

## 2. Cấu trúc kịch bản Triệu View (Khoa học giữ chân)

Mỗi video viral 9:16 được thiết kế chặt chẽ theo 4 phân đoạn vàng:

1. **Phân cảnh 1: Hook 3s Giật Gân (Shock/Curiosity Hook - 0 đến 3s)**
   - Câu thoại ngắn gọn, đánh trúng tâm lý tò mò hoặc nỗi sợ bị bỏ lỡ (FOMO).
   - Chữ giật tít hiển thị lớn trên màn hình.
   - Đảm bảo tỷ lệ giữ chân >60% ở 3 giây đầu tiên để thuật toán TikTok/Reels/Shorts đẩy video vào luồng phân phối mạnh.
2. **Phân cảnh 2: Nỗi Đau & Vấn Đề Nhức Nhối (Problem & Tension - 3 đến 12s)**
   - Trực tiếp chỉ ra sai lầm phổ biến hoặc khúc mắc mà đại đa số người xem đang gặp phải.
3. **Phân cảnh 3: Góc Nhìn Đột Phá & Giải Pháp (Insight & Solution - 12 đến 24s)**
   - Cung cấp giá trị thực chất, bí mật ít người biết, hoặc phương pháp giải quyết nhanh gọn.
4. **Phân cảnh 4: Kêu Gọi Hành Động Tự Nhiên (CTA & Follow - 24 đến 30s)**
   - Đúc kết cảm xúc tích cực, kêu gọi lưu video và follow kênh để đón xem tập tiếp theo.

## 3. Giao diện điều hành (`/viral`)

Trang `/viral` bao gồm 3 khu vực chính:

1. **Bảng chỉ số & Tự động hóa**:
   - Công tắc `viral.auto_produce_enabled`: Bật/tắt chế độ tự động sáng tạo mỗi ngày.
   - Số lượng video đang render AI (kèm thanh tiến độ và log thời gian thực).
   - Kho lưu trữ video đã hoàn thành sẵn sàng xuất bản.
2. **Khởi tạo 1-Chạm (1-Click Fast Launcher)**:
   - Các thẻ gợi ý theo xu hướng thực tế: Bí ẩn & Giật gân, Kinh nghiệm & Đời sống, Công nghệ mới, Đánh giá KOC.
   - Thể hiện tiềm năng lượt xem và câu Hook mở đầu mẫu.
3. **Tùy chỉnh kịch bản theo yêu cầu**:
   - Nhập chủ đề bất kỳ.
   - Lựa chọn thể loại, phong cách hình ảnh (Điện ảnh, 3D nghệ thuật, Thực tế đời sống, Cyber neon).
   - Chọn kênh đại diện để đồng bộ phong cách Brand DNA.
   - Chọn thời lượng mục tiêu (15s, 30s, 45s, 60s).
4. **Trình phát video xem trước & Tải về**:
   - Trình phát video dọc 9:16 HTML5 tích hợp trực tiếp trên dashboard.
   - Nút tải file `.mp4` hoặc đẩy sang Cổng Xuất Bản (`/publishers`).

## 4. Kiến trúc kỹ thuật & Lưu trữ

- **Job Kind**: `KindViral = "viral"` trong cơ sở dữ liệu `studio.db` (bảng `studio_jobs` và `studio_assets`).
- **Engine**: Đóng gói trong package `internal/studio` (`viral.go`).
- **Render Engine**:
  - Biên dịch chuyển động Ken Burns chuẩn tỉ lệ dọc 9:16 (1080x1920) qua FFmpeg.
  - Hòa âm giọng đọc voiceover và nhạc nền trending tự động cân bằng âm lượng (Audio Normalization).
- **Phục hồi & Fail-closed**:
  - Toàn bộ pipeline chạy nền được bảo vệ bằng `recover()` bắt panic, timeout tối đa 20 phút.
  - Không block luồng web; dashboard liên tục cập nhật tiến độ qua API poll.
