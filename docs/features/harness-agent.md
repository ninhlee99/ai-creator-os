# Harness và vòng học chất lượng video

## Trạng thái hiện tại

Harness hiện là vòng quan sát/ghi chú có giới hạn, chưa phải bộ điều khiển zero-touch.

- `Harness.RunCycle` gọi `OODAEngine.ExecuteCycle`, ghi quan sát, diagnosis và decisions vào `harness_cycles`.
- `OODAEngine.Observe` đọc số người theo dõi từ account và thống kê views theo video từ `growth.video_metric_snapshots`. Mỗi channel snapshot ghi số video có view reading mới trong 7 ngày, trung bình views/video và thời điểm đo gần nhất. Đây là số video được đồng bộ metrics, không phải số video mới đăng.
- OODA đọc thêm các verdict format đã lưu (`testing`/`winner`/`killed`), cỡ mẫu, views, lý do và `updated_at`. Đây là projection read-only; không tự tính lại verdict và không coi verdict cũ là mới.
- Không có reading mới thì `metrics_status` là `missing`; store/query lỗi thành `error`; chưa nối Growth store thành `unavailable`. Không thay thiếu dữ liệu bằng 0 để kết luận hiệu quả. Hiện nguồn metrics theo video là YouTube Analytics; TikTok chưa nối.
- OODA lưu readiness thật do Studio báo: affiliate tự động và thay toàn thân viral. Hiện cả hai đều `false`; trạng thái kèm lý do được lưu vào snapshot và hiện trong cycle inspector.
- LLM chỉ tóm tắt metrics và capability evidence; OODA không biến views trung bình thành task tạo video. Viral cần video nguồn cụ thể, affiliate cần sản phẩm đã xác minh và footage khả dụng. Hai nguồn candidate này chưa được nối nên hiện không sinh production task.
- OODA chưa chạy job video hoặc đăng bài. Growth automation riêng chạy `Evaluate` khi nhận snapshot thật; OODA chỉ báo verdict đã lưu để người vận hành đọc.
- Level 2 và 3 không render video hoặc đăng bài. OODA không dispatch production action khi thiếu capability hoặc input gắn nguồn; `ActionCount` hiện bằng 0.
- Memory lưu bài học nhập qua UI. OODA chưa tự ghi bài học hiệu quả video vì không có action nào được thực thi.
- `Studio.PromptEnhancer` lấy memory theo niche và channel. Affiliate Director dùng nội dung đó như gợi ý yếu cho nhịp kể/cảm xúc; prompt cấm memory ghi đè bằng chứng sản phẩm, cách mặc/dùng, red lines hoặc claim. Viral recreation không nhận gợi ý phong cách vì phải giữ nguyên hành động và nhịp của nguồn.
- Prompt chỉ nhận memory của đúng channel hoặc memory dùng chung; memory riêng của channel khác không bị trộn vào dù cùng niche. Context hiển thị channel, số mẫu và confidence. Mẫu ít là giả thuyết, không phải kết quả đã chứng minh.

## Luồng affiliate hiện có

1. `POST /studio/affiliate/plan` khóa kênh, ảnh listing, ảnh nhân vật, Product Analyst brief và Director shot list.
2. Director đọc Brand DNA, channel learning, Product Analyst brief và ảnh listing. Cách dùng thật và bằng chứng sản phẩm luôn ưu tiên.
3. Producer nhận một clip chuyển động do người vận hành cung cấp cho mỗi shot; app chuẩn hóa và ghép clip.
4. Temporal QC chặn footage gần đứng yên. Visual QC kiểm tra mẫu frame theo từng shot. QC trên frame mẫu không chứng minh mọi thời điểm trong video.
5. Autopilot affiliate đang dừng vì chưa có provider tạo footage chuyển động khả dụng theo chính sách free-only.

## Luồng viral hiện có

- Viral Radar chỉ lưu nguồn tham khảo.
- Phân tích nguồn có thể tạo storyboard từ frame mẫu, nhưng không tạo footage.
- `EnqueueViral` từ chối mọi job; adapter thay toàn thân hiện chưa triển khai. Face swap không đạt yêu cầu thay toàn nhân vật.
- Vì vậy hiện không có video viral đầu ra để QC hoặc đăng.

## Thiết kế đích cho agent dùng được

1. **Observer** đọc metrics thật theo channel và video ID, kèm nguồn, thời điểm, mẫu số và khoảng dữ liệu. Thiếu dữ liệu thì không suy luận.
2. **Growth Analyst** so sánh format theo cỡ mẫu tối thiểu; tách views, retention, share/save, follower gain và affiliate orders. Không kết luận chuyển đổi khi chưa nối attribution.
3. **Creative Director** dùng bài học channel-scoped làm giả thuyết về hook, cảm xúc và nhịp dựng. Product truth và viral source fidelity là ràng buộc cao hơn memory.
4. **Producer/Editor** chỉ nhận việc khi provider thật hỗ trợ đúng capability: affiliate tạo footage nhân vật mặc/dùng sản phẩm; viral giữ cảnh, choreography, camera và audio nguồn trong khi thay toàn thân.
5. **QC** đối chiếu sản phẩm/nhân vật, hành động, framing, continuity và chất lượng ở toàn timeline. Chỉ worker đã chạy mới được ghi stage pass; QC model là screening, không phải chứng minh tuyệt đối.
6. **Learning loop** ghi kết quả có nguồn và cỡ mẫu, cập nhật memory theo channel/format, rồi cấp lesson phù hợp vào prompt kế tiếp. Không tự học từ dự đoán, job chưa đăng hoặc số liệu không gắn video.

## Điều kiện mở sản xuất tự động

- Có quyết định policy rõ cho provider tạo/chỉnh video. `docs/STANDARDS.md` hiện cấm billing, Veo và model runtime cục bộ; chính sách này chặn cả hai capability video thật.
- Provider phải báo capability, readiness và giới hạn chi phí trước khi cho phép enqueue. Không gửi request video trả phí khi policy chưa được đổi.
- Affiliate phải tạo được chuyển động mặc/dùng sản phẩm thật; viral phải thay toàn thân và giữ timeline nguồn. Nếu không có provider đáp ứng, UI tiếp tục báo chưa hỗ trợ.

## Mã chính

- `internal/harness/ooda.go`, `internal/harness/harness.go`, `internal/harness/memory.go`
- `internal/studio/affiliate_plan.go`, `internal/studio/director.go`, `internal/studio/prompts/affiliate_shotlist.txt`
- `internal/studio/affiliate.go`, `internal/studio/viral.go`, `internal/studio/gemini_video_edit.go`
- `internal/growth/metrics.go`, `internal/growth/youtube_analytics.go`, `internal/growth/formats.go`

Cycle inspector escape dữ liệu động trước khi đưa vào HTML; nội dung diagnosis từ LLM và các trường account/product không được diễn giải như markup.
