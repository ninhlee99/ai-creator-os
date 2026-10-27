# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar dùng Web Crawler cục bộ; field metrics và sound metadata có thể thiếu. Crawler không phân loại video nhảy: video mới ở trạng thái chưa xác nhận cho tới khi người vận hành xem và xác nhận trên card Radar. Chỉ video đã thu thập và xác nhận thủ công mới dùng làm nguồn audio. Xem [tài liệu chuẩn về TikTok Web Crawler](tiktok-web-crawler.md). Viral analysis chỉ nêu tín hiệu quan sát và giả thuyết thử nghiệm, không khẳng định nguyên nhân viral. Audio lấy từ video nguồn đã chọn; không tự thay sound khi tải hoặc tách audio thất bại.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Session và credits phụ thuộc provider; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Workflow Studio

Preset Affiliate và Fashion tạo ba clip toàn thân/bán thân/cận cảnh, ghép thành video, lồng audio, cân màu, upscale và xuất file. Preset Viral phân tích frame bằng Vision AI, dựng prompt từ mô tả chuyển động, render video, dùng lại audio nguồn và xuất file. Các preset cần ảnh nhân vật, sản phẩm đã duyệt hoặc ảnh sản phẩm thật, audio/video nguồn thật và provider đã cấu hình trước khi chạy.

Node AI custom hỗ trợ text, image và video qua dịch vụ đang cấu hình. Video đầu vào được lấy một frame làm ảnh tham chiếu; phân tích hoặc tạo audio chưa được hỗ trợ. `custom_script_runner` chỉ thay biến trong template văn bản, không chạy code tùy ý. `virtual_tryon` giữ ảnh gốc và chuẩn bị outfit prompt; không thay quần áo trong ảnh. `dance_retarget` tạo prompt từ mô tả Vision AI; không thực hiện motion capture. `image_composite` chỉ overlay ảnh ở vị trí xấp xỉ. `custom_export_publish` và `video_output` chỉ xuất file; đăng TikTok cần workflow consent riêng trong Studio.

Workflow chạy thật cần nguồn media, catalog/provider, FFmpeg và cấu hình phù hợp. Preset chưa phải một lần bấm để sinh nội dung nếu các đầu vào còn trống.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. Job Affiliate/Viral đã qua QC có thể gửi vào TikTok Creator Inbox từ Studio sau khi người vận hành xem preview, xác nhận account đích và đồng ý gửi Draft. Consent được ghi vào log job. Trạng thái gửi chống tạo draft trùng; kết quả không rõ phải đối chiếu trong TikTok trước khi gửi lại. Draft cần người dùng kiểm tra và duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
