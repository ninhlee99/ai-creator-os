# Brand DNA & AI Mission Control

## 1. Mục đích
Xây dựng hệ thống tự hành cho phép người chủ doanh nghiệp giao mục tiêu bằng ngôn ngữ tự nhiên (AI Mission Control) và sản xuất nội dung nguyên bản chất lượng cao theo từng hồ sơ thương hiệu (Brand DNA), triệt tiêu sự phụ thuộc vào việc reup video của bên thứ ba.

## 2. Kích hoạt & Route
- `POST /team/dispatch` → `handleTeamDispatchGoal` (`internal/web/agent_workforce.go`): Giao mục tiêu chiến lược 1-chạm từ Trang chủ hoặc Tổng hành dinh AI.
- `GET /accounts/{id}?tab=brand-dna` → `handleAccountDetail` (`internal/web/accounts.go`): Xem và tùy biến hồ sơ Brand DNA cho kênh.
- `POST /accounts/{id}/brand-dna` → `handleAccountBrandDNASave` (`internal/web/accounts.go`): Lưu hồ sơ Brand DNA.
- `POST /accounts/{id}/brand-dna/reset` → `handleAccountBrandDNAReset` (`internal/web/accounts.go`): Khôi phục Brand DNA chuẩn AI theo ngành hàng.
- `GET /api/missions/{id}` → `handleMissionDetailJSON` (`internal/web/mission_engine.go`): Truy xuất trạng thái và tiến độ từng bước của nhiệm vụ.
- `POST /missions/{id}/cancel` → `handleMissionCancel` (`internal/web/mission_engine.go`): Hủy nhiệm vụ khẩn cấp.

## 3. Luồng vận hành chi tiết

### A. AI Mission Control Engine (`internal/web/mission_engine.go`)
1. **Tiếp nhận & Phân loại mục tiêu**:
   - `affiliate_campaign`: Tự động tìm sản phẩm hoa hồng cao, soạn 4 Hook 3s theo Brand DNA, tạo job video 9:16 trong Studio và gắn tracking link Accesstrade.
   - `story_campaign`: Lập đề cương truyện ngôi thứ nhất, phân 5 cảnh kịch tính, tạo job kể chuyện Studio (hình ảnh 16:9 + giọng đọc TTS).
   - `trend_campaign`: Thu thập âm thanh thịnh hành từ Kworb TikTok VN, phân tích cơ hội qua Trend Intelligence, đề xuất 3 góc khai thác nguyên bản.
   - `growth_campaign`: Đánh giá số liệu 14 ngày, loại bỏ format kém, tự động lập lại ma trận nội dung 30 ngày cho kênh.
2. **Theo dõi tiến độ theo thời gian thực**:
   - Lưu trữ trong bảng `ai_missions` (SQLite WAL).
   - Tự động ghi nhận quyết định vào `decisions` audit ledger (`agent: "AI Mission Control"`).
   - Hiển thị trực quan tại thẻ "Chiến Dịch AI Vận Hành Tự Động" trên Trang chủ.

### B. Brand DNA & Tiêu Chuẩn Nguyên Bản (`internal/growth/brand_dna.go`)
Mỗi kênh sở hữu một hồ sơ thương hiệu toàn diện:
- **Định vị (Positioning)**: Đại diện cho giá trị cốt lõi nào của ngành hàng.
- **Chân dung khán giả (Audience Persona)**: Nỗi đau, độ tuổi, tâm lý mua hàng.
- **Lời hứa giá trị (Value Promise)**: Lợi ích nhận được sau mỗi video.
- **Tông giọng (Tone of Voice)**: Phong cách phát ngôn đặc trưng.
- **Nguyên tắc Hook 3s (Hook Rules)**: 4 công thức mở đầu giữ chân người xem.
- **Trụ cột nội dung (Content Pillars)**: Ma trận 60-30-10 tránh trùng lặp.
- **Thẩm mỹ (Visual & Audio Style)**: Ánh sáng, bảng màu, nhạc nền và tốc độ nói.
- **Ranh giới đỏ (Red Lines)**: Danh sách từ cấm nền tảng và điều cấm kỵ của thương hiệu.
- **Checklist QC nguyên bản (10 tiêu chí)**: Gác cổng trước khi xuất bản.

### C. Trend Intelligence Engine (`internal/growth/trend_intelligence.go`)
- Bóc tách tín hiệu xu hướng từ Kworb TikTok VN và radar Douyin.
- Tính toán điểm vận tốc (Velocity Score 0–100) và điểm tương thích Brand DNA (Brand-Fit Score 0–100).
- Tổng hợp thành Điểm Cơ Hội (Opportunity Score) và đề xuất 3 góc quay nguyên bản thay vì reup copy nguyên trạng.
