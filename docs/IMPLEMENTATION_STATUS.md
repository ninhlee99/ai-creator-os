# Trạng thái triển khai

Ảnh chụp source: 2026-10-25. Đây là rà soát tĩnh; chưa chứng minh hoạt động với tài khoản/provider thật.

## Hai flow đang làm

| Bước | Trạng thái | Giới hạn hiện tại |
|---|---|---|
| Kết nối TikTok | Một phần | OAuth client dùng Go DNS resolver để tránh lỗi resolver hệ thống trên macOS. Smoke server đã chạy build mới; OAuth với tài khoản TikTok thật, quyền app và callback deployment vẫn chưa xác minh. |
| Radar video TikTok | Một phần | Feed mặc định qua TikWM; topic search có adapter chính thức TikTok Business Discovery khi cấu hình access token + advertiser ID: lấy hashtag phổ biến khu vực VN rồi top videos theo hashtag. Chưa xác minh với credential thật; endpoint trả ID/URL, không trả counters. Nếu chưa cấu hình, TikWM `feed/search` hiện bị Cloudflare challenge HTTP 403; không né challenge và không lọc caption cục bộ thay thế. Kết quả lưu tối đa 2.000 nguồn và snapshot metrics 30 ngày, xếp theo tốc độ view khi có hai mốc trong 7 ngày. Dance classification vẫn cần người xác nhận. |
| Chọn video nhảy | Chọn thủ công | Người dùng phải xác nhận đã xem clip và nội dung thực sự là vũ đạo; server chặn xác nhận thiếu dấu này, đồng thời chặn job nếu nguồn chưa xác nhận hoặc Radar chưa thu thập lại trong 7 ngày. Chưa có classifier dance tự động. |
| Radar sản phẩm và duyệt | Một phần | TikTok Shop feed qua AccessTrade tìm theo category keyword, sort commission; khi có `commission.amount`, lưu và hiển thị amount/currency nguồn; chỉ ước tính rate × giá khi thiếu amount. `units_sold` hiển thị là tổng từ feed; feed không nêu kỳ đo. Khi có hai snapshot app cách nhau 6 giờ–14 ngày, UI tính chênh lệch quan sát được và ghi rõ khả năng trễ từ nguồn. Offer thiếu giá không bị bỏ khỏi kho. Currency/provider-updated-at/sales hiển thị unknown nếu nguồn không trả; rating/review không có trong feed hiện tại. Duyệt chặn offer thiếu giá, currency, commission hoặc link listing. Có thao tác loại kèm lý do và danh sách xem lại; tín hiệu xu hướng tách match tên sản phẩm/ngành hàng từ Radar nhưng chưa chấm độ phù hợp hoặc tệp tuổi. Sản xuất tự động cần offer đã duyệt và demand evidence còn hạn. |
| Tạo video affiliate | Một phần | Shot plan/QC có; một số nhánh vẫn cần clip chuyển động do người dùng cung cấp hoặc provider cần credits. Chưa chứng minh tạo miễn phí end-to-end. |
| Tạo video viral | Một phần | Nguồn video được ràng buộc vào Radar; phân tích trả tối đa 4 yếu tố cấu trúc quan sát có mốc beat và 3 thử nghiệm một biến cho video sau. Mốc được xác thực lại với beat nguồn; giả thuyết không chứng minh nguyên nhân viral. Metrics/retention/audience chỉ hiện khi có nguồn; tạo bản video hoàn chỉnh và giữ đúng audio cần xác minh end-to-end. |
| Lấy audio Radar | Một phần | Code tải video nguồn, tách audio và kiểm tra track; cần xác minh bằng video thật và đầu ra cuối. |
| Gửi TikTok Draft | Một phần | Job Affiliate/Viral qua QC có thể gửi thủ công từ Studio; trạng thái dùng chung chống gửi trùng và kết quả không rõ cần đối chiếu trước retry. Chưa chứng minh tài khoản/app thật đủ scope. Draft phải được người dùng hoàn tất trong TikTok. |
| Analytics/gợi ý | Một phần | TikTok Display API đã nối follower/video totals và current public-video views/likes/comments/shares; Growth so delta follower giữa snapshot cách nhau ít nhất 24 giờ. Job hoàn tất nhận link TikTok công khai; chỉ xem video đã xác minh trong snapshot `video.list` mới của đúng kênh và chỉ sau đó ghép đơn AccessTrade theo `sub2`. Engagement baseline so video cùng flow/kênh, hiển thị số mẫu; dưới 3 video đối chiếu thì chưa kết luận cao/thấp. Chưa có retention, demographics, profile visits hoặc follows-per-video. TikTok Shop product card vẫn cần người dùng gắn thủ công trong TikTok. |
| Truyện audio | Để phase sau | Không nằm trong acceptance hiện tại. |

## Ưu tiên sửa

1. Dance detection/select phải không lấy nhầm video viral thường.
2. Hai luồng affiliate/viral phải dùng audio từ cùng video Radar được chọn.
3. Job lưu URL/video ID/audio provenance; lỗi tải video Radar giờ hiển thị stderr `yt-dlp`. Còn xác minh tách/ghép trên video thật.
4. Offer hiển thị chính xác commission; sales/rating chỉ hiện nếu có nguồn thật.
5. Video QC và gửi Draft phải giữ trạng thái thật, tránh retry gây job trùng.
6. Xác minh Display API scopes bằng account thật; metrics retention, demographics và order attribution chỉ mở khi có nguồn thật.

## Giới hạn chung

Provider Web AI có thể hỗ trợ tạo media nhưng trạng thái session không chứng minh gói miễn phí hoặc credits. Flow/Muse không được tự dùng dịch vụ tính phí. Không có đủ nguồn hợp lệ thì dừng job và báo rõ.
