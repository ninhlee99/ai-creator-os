# Rà soát dự án

Cập nhật: 2026-10-28. Rà source, tài liệu và test cục bộ; chưa xác minh bằng tài khoản/provider production thật.

## Đã xử lý trong source

- Radar dùng Web Crawler làm nguồn TikTok. Crawler contract là nguồn chi tiết chuẩn; video cần người vận hành xác nhận trước khi dùng.
- Chọn video dance và lấy audio bị chặn nếu nguồn chưa được xác nhận; track audio được kiểm tra trong file đầu ra.
- Màn hình Sản phẩm tách khám phá/duyệt khỏi vận hành Affiliate. Growth tách Tổng quan khỏi Vận hành.
- Workflow có input/output contract, gọi workflow con theo loại Affiliate, TikTok Viral, Thời trang hoặc Custom; contract sai, thiếu hoặc thay đổi sẽ báo lỗi khi chạy.
- Scaffold 3D/VFX giả đã gỡ. Loại workflow và kiểu cổng không được backend hỗ trợ cũng đã gỡ khỏi UI.
- Backup dùng SQLite online snapshots, archive/staging có giới hạn và kiểm tra; secret bị loại khỏi ledger snapshot, `webai.db`/token file nhạy cảm không xuất vào ZIP plaintext.
- Studio giới hạn hàng đợi và concurrent render; output social mặc định 1080p; job dọn output/cache cũ theo policy.
- WebAI/CDP dùng profile cô lập, port debug động, một browser automation slot toàn app và cleanup process tree; Edge macOS chạy trực tiếp với profile tạm, không chia sẻ profile/process với Edge người dùng. Output browser đi thẳng null device để child process không giữ pipe của `Cmd.Wait`.
- Growth không còn suy diễn hook/retention/giá/“healthy” khi thiếu provenance, kỳ đo, baseline; tuning áp dụng qua override allowlist theo account, bounded 15%, evidence-backed audit và rollback chống ghi đè setting mới.

## Còn cần xác minh ngoài source

1. **Radar:** quyền crawl production chưa được xác nhận. Hành vi và giới hạn hiện tại: [TikTok Web Crawler](features/tiktok-web-crawler.md).
2. **Video/audio:** kiểm tra end-to-end với video thật để chứng minh audio đúng nguồn đã chọn và còn trong MP4 cuối.
3. **Affiliate evidence:** AccessTrade feed hiện tại không chứng minh doanh số theo kỳ, rating hoặc review từng sản phẩm. UI phải giữ các trường này là unknown khi thiếu nguồn.
4. **TikTok Draft:** xác minh OAuth scope, submit, trạng thái Draft và xử lý timeout với tài khoản/app thật. Kết quả không rõ cần đối chiếu trước retry.
5. **Growth:** xác minh Display API scopes và metrics bằng tài khoản thật; không suy ra retention/demographics khi API không trả.

## Feedback UI/UX đã xử lý trong source

- **Workflow inputs:** product input có picker chỉ liệt kê sản phẩm đã duyệt, đủ dữ liệu affiliate. Video source có picker Radar theo chủ đề, hiển thị trạng thái xác nhận vũ đạo; URL gõ tay vẫn qua kiểm tra của executor.
- **Media upload:** upload node nhận ảnh, video và audio theo định dạng executor hỗ trợ. Link picker gửi/đọc đúng contract `type` của endpoint.
- **Workflow settings:** provider, output type, operation, aspect ratio, location, lighting time, codec, resolution và HTTP method dùng dropdown theo giá trị executor hỗ trợ.
- **Custom nodes:** inspector và detail modal nêu rõ trường hợp dùng Universal AI, Web AI, AI Prompt; nhắc giới hạn frame video, audio, script và publish.
- **Preset workflows:** có Fit Canvas để xem toàn graph khi mở preset hoặc đổi workflow.
- **Audio output:** có node xuất audio riêng; run panel phát và tải file audio.
- **TikTok Draft:** UI phân biệt Draft đã tải lên nhưng chưa công khai với bước kiểm tra/duyệt tiếp theo trong TikTok.

## Giới hạn năng lực node còn lại

- Chưa có AI node phân tích hoặc tạo audio. Audio Source, Audio Merge và Audio Output chỉ nạp, ghép hoặc xuất file audio.
- Vision Analyzer chỉ phân tích ảnh hoặc một frame video; Dance Retarget tạo prompt từ kết quả Vision, không thực hiện motion capture.
- Virtual Try-On giữ ảnh gốc và tạo mô tả outfit; không thay trang phục trong ảnh tĩnh. Image Composite overlay ảnh ở vị trí xấp xỉ.
- Custom Script Runner chỉ thay biến trong template; không chạy mã tùy ý. Export nodes chỉ xuất file; đăng TikTok cần luồng consent riêng.
- Preset graph có đầu ra và đường dây hoàn chỉnh, nhưng cần input thật, FFmpeg và provider sẵn sàng. Chưa xác minh end-to-end với tài khoản/provider production.

## Phase tiếp

Truyện audio chỉ mở sau khi hai flow TikTok đạt acceptance trong PRODUCT_SPEC.
