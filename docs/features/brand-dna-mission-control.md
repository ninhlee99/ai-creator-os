# Brand DNA, Trend Intelligence và Mission Engine

## Trạng thái hiện tại

**Một phần.** Brand DNA lưu được theo kênh và được dùng ở một số luồng. Mission Engine có form tại `/team`, nhưng chỉ phân loại mục tiêu bằng từ khóa và chạy đồng bộ; không có tiến độ trực tiếp, lịch sử mission trong UI hoặc chức năng hủy.

## Brand DNA

- Mở `/accounts/{id}?tab=brand-dna` để xem/sửa hồ sơ; `POST /accounts/{id}/brand-dna` lưu vào Growth SQLite.
- Nếu chưa có bản ghi, form nạp mẫu gợi ý theo ngách nhưng chưa lưu; trạng thái đọc được hiển thị trên trang. Nút “Dùng lại mẫu theo ngách” ghi đè toàn bộ hồ sơ đang lưu; lỗi đọc/ghi được báo lại.
- Affiliate Director nhận `positioning`, `audience_persona`, `tone_of_voice`, `visual_style` và `red_lines` làm ngữ cảnh shot plan.
- Trend heuristic dùng `positioning`, `content_pillars` và `hook_rules` để tính tương thích/gợi ý góc nội dung.
- `value_promise`, `audio_style` và `qc_checklist` được lưu nhưng hiện chưa được các job Studio áp dụng. Checklist QC không tự kiểm tra nội dung hoặc chặn đăng. Red lines đi vào prompt Affiliate Director, chưa phải deterministic gate.
- `ValidateContentAgainstDNA` chỉ dò chuỗi cấm tổng quát và một dạng red line; không đánh giá checklist và chưa được gọi từ production flow.

## Mission Engine

### Mở và gửi mục tiêu

- Vào `/team`, nhập mục tiêu và gửi form `POST /team/dispatch`.
- Bộ phân loại dùng `strings.Contains` với nhóm từ khóa như “kể chuyện”, “trend”, “30 ngày”, “affiliate” hoặc “thay nhân vật”; đây không phải chatbot hiểu mục tiêu tổng quát.
- Tên kênh được khớp nếu mục tiêu có username. Nếu chỉ có một kênh, engine tự chọn kênh đó; với nhiều kênh cần ghi đúng username.
- Tùy chọn “xác nhận Google Flow credits” chỉ cho phép tiếp tục nhánh Affiliate có thể dùng credits. Engine không kiểm tra quota hoặc giá thực tế.
- Dispatch chạy ngay trong request. `completed` nghĩa là bước mission xong hoặc job được xếp, không có nghĩa video đã render, qua QC hay đăng.

### Hành vi theo loại mục tiêu

| Nhóm | Hành vi hiện có | Giới hạn |
|---|---|---|
| Affiliate | Chờ xác nhận credits; tìm mục phù hợp trong kệ, cần demand evidence, ảnh listing, kênh và ảnh nhân vật; lập Product Analyst/Director plan rồi xếp Studio job nếu readiness cho phép. | Không tự đăng. Video chưa hoàn tất khi mission báo xếp job. Datafeed hiện chưa cung cấp đủ demand evidence. |
| Kể chuyện | Xếp job truyện chuẩn vào Studio. | Mission kết thúc khi job được xếp; không chờ render xong. |
| Trend | Đọc chart âm thanh TikTok Việt Nam của Kworb và lưu gợi ý heuristic từ tín hiệu hiện tại. | Không tạo job/video. Không có nguồn radar Douyin; điểm cơ hội không phải dự báo lượt xem. |
| Tăng trưởng | Tạo lịch 30 ngày từ ngách/giai đoạn kênh và verdict format đã đánh dấu dừng. | Snapshot chỉ báo có hay chưa; chưa được dùng để tính lại lịch hoặc dự báo kết quả. |
| Thay nhân vật viral | Chuyển người vận hành sang `/viral` để chọn video nguồn, kênh và ảnh nhân vật. | Chưa tạo job thay nhân vật. |
| Không phân loại | Yêu cầu nêu rõ loại mục tiêu. | Chưa tạo job. |

### Trạng thái, lưu trữ và hủy

- Engine thử ghi kết quả vào bảng `ai_missions` sau khi nhánh xử lý trả về nếu SQLite khả dụng. Hiện lỗi tạo bảng/ghi dòng bị bỏ qua; API `GET /api/missions/{id}` chỉ đọc kết quả đã lưu và trả 404 nếu không có dòng, không phải polling tiến độ trực tiếp.
- Audit ledger nhận sự kiện dispatch nếu ledger khả dụng.
- `POST /missions/{id}/cancel` trả HTTP 501. Engine chưa hỗ trợ hủy; xử lý đồng bộ và chỉ lưu kết quả sau khi hoàn tất.
- Form `/team` hiển thị tóm tắt theo trạng thái. Yêu cầu lỗi hiện bằng thông báo lỗi; yêu cầu chờ xác nhận/đầu vào không được gọi là chiến dịch đã tạo.

## Trend heuristic

- Nguồn đang dùng: `studio.FetchVNTrending` → Kworb TikTok Trending Songs Vietnam.
- `AnalyzeTrendSignals` suy điểm từ movement/rank và so khớp từ khóa với Brand DNA. Một số câu về “nhu cầu khán giả” là mẫu heuristic tĩnh.
- Mission chỉ lưu gợi ý góc đầu tiên ở bước 3; thông báo sau dispatch chỉ báo số tín hiệu. Chưa có đánh giá bằng views, retention, sales hay kết quả publish.

## Mã chính

- `internal/growth/brand_dna.go`
- `internal/growth/trend_intelligence.go`
- `internal/web/account_brand.go`
- `internal/web/mission_engine.go`, `internal/web/mission_campaigns.go`, `internal/web/mission_affiliate.go`, `internal/web/mission_http.go`
- `internal/web/templates/account_detail.html`, `internal/web/templates/team.html`
