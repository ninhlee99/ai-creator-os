# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar dùng Web Crawler cục bộ; field metrics và sound metadata có thể thiếu. Phân tích ưu tiên Gemini Web, ChatGPT Web, Claude Web, Grok Web đã kết nối; Gemini API key chỉ dùng khi không có kết quả WebAI hợp lệ. AI đọc 4 frame theo thời gian cùng caption để chấm vũ đạo và độ khớp topic. Chỉ tab mặc định `Xu hướng Việt Nam` yêu cầu bằng chứng vũ đạo; topic khác chỉ cần khớp nội dung. Với tab mặc định, người vận hành vẫn phải xem và xác nhận video trước khi dùng audio. Xem [tài liệu chuẩn về TikTok Web Crawler](tiktok-web-crawler.md). Viral analysis chỉ nêu tín hiệu quan sát và giả thuyết thử nghiệm, không khẳng định nguyên nhân viral. Audio lấy từ video nguồn đã chọn; không tự thay sound khi tải hoặc tách audio thất bại.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Form Tạo nội dung, render Affiliate/Viral, retry/remix và mỗi lần chạy Workflow Studio cần xác nhận có thể dùng credits. Muse workflow dùng Chrome/Edge/Chromium cài trên máy chạy app ở chế độ headless và account Muse đã kết nối. Webhook API kiểm tra graph và subworkflow, kể cả custom video node: gửi `muse_credits_acknowledged: true` cho Muse hoặc `google_flow_credits_acknowledged: true` cho Google Flow. Session không xác nhận gói miễn phí; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Workflow Studio

Preset Affiliate và Fashion tạo ba clip toàn thân/bán thân/cận cảnh, ghép thành video, lồng audio, cân màu, upscale và xuất file. Preset Viral phân tích frame bằng Vision AI, dựng prompt từ mô tả chuyển động, render video, dùng lại audio nguồn và xuất file. Các preset cần ảnh nhân vật, sản phẩm đã duyệt hoặc ảnh sản phẩm thật, audio/video nguồn thật và provider đã cấu hình trước khi chạy.

Node AI custom hỗ trợ text, image và video qua dịch vụ đang cấu hình. Video đầu vào được lấy một frame làm ảnh tham chiếu; phân tích hoặc tạo audio chưa được hỗ trợ. Audio Source, Audio Merge và Audio Output nạp, ghép hoặc xuất file audio, không tạo hay phân tích audio bằng AI. `custom_script_runner` chỉ thay biến trong template văn bản, không chạy code tùy ý. `virtual_tryon` giữ ảnh gốc và chuẩn bị outfit prompt; không thay quần áo trong ảnh. `dance_retarget` tạo prompt từ mô tả Vision AI; không thực hiện motion capture. `image_composite` chỉ overlay ảnh ở vị trí xấp xỉ. `custom_export_publish` và các node output chỉ xuất file; đăng TikTok cần workflow consent riêng trong Studio.

Trong Workflow Studio, product input chọn sản phẩm đã duyệt trong catalog; video source có thể chọn video từ Radar theo chủ đề. Dropdown cài đặt chỉ liệt kê giá trị executor đang hỗ trợ. Fit Canvas đưa toàn bộ workflow vào vùng nhìn khi mở graph.

Workflow chạy thật cần nguồn media, catalog/provider, FFmpeg và cấu hình phù hợp. Preset chưa phải một lần bấm để sinh nội dung nếu các đầu vào còn trống.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. Job Affiliate/Viral đã qua QC có thể gửi vào TikTok Creator Inbox từ Studio sau khi người vận hành xem preview, xác nhận account đích và đồng ý gửi Draft. Consent được ghi vào log job. Trạng thái gửi chống tạo draft trùng; kết quả không rõ phải đối chiếu trong TikTok trước khi gửi lại. Draft cần người dùng kiểm tra và duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
