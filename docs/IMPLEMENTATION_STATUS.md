# Trạng thái triển khai

Ảnh chụp source: 2026-10-24. Đây là rà soát tĩnh; chưa chứng minh hoạt động với tài khoản/provider thật.

## Hai flow đang làm

| Bước | Trạng thái | Giới hạn hiện tại |
|---|---|---|
| Kết nối TikTok | Một phần | OAuth client dùng Go DNS resolver để tránh lỗi resolver hệ thống trên macOS. Smoke server đã chạy build mới; OAuth với tài khoản TikTok thật, quyền app và callback deployment vẫn chưa xác minh. |
| Radar video TikTok | Một phần | Feed mặc định lấy video Việt Nam; khi nhập chủ đề, gọi TikWM `feed/search` thay vì lọc caption cục bộ. Kết quả được lưu tối đa 2.000 nguồn và snapshot metrics 30 ngày trong SQLite, xếp theo tốc độ tăng view khi có hai mốc trong 7 ngày; video thiếu lịch sử xếp theo view. Comments/shares chỉ hiện khi provider trả. Chưa xác minh endpoint topic search trực tiếp trong môi trường hiện tại do DNS không phân giải được TikWM. Dance classification vẫn cần người xác nhận. |
| Chọn video nhảy | Chọn thủ công | Xác nhận dance được lưu bền trong SQLite; server chặn cả job affiliate và viral nếu nguồn chưa xác nhận hoặc Radar chưa thu thập lại trong 7 ngày. Chưa có classifier dance tự động. |
| Radar sản phẩm và duyệt | Một phần | TikTok Shop feed qua AccessTrade tìm theo category keyword, sort commission; offer thiếu giá không bị bỏ khỏi kho. Currency/provider-updated-at/sales hiển thị unknown nếu nguồn không trả; rating/review không có trong feed hiện tại. Duyệt chặn offer thiếu giá, currency, commission hoặc link listing. Có thao tác loại kèm lý do và danh sách xem lại; đánh giá xu hướng/tệp tuổi chưa tự động. Sản xuất tự động cần offer đã duyệt và demand evidence còn hạn. |
| Tạo video affiliate | Một phần | Shot plan/QC có; một số nhánh vẫn cần clip chuyển động do người dùng cung cấp hoặc provider cần credits. Chưa chứng minh tạo miễn phí end-to-end. |
| Tạo video viral | Một phần | Nguồn video được ràng buộc vào Radar; khả năng tạo bản video hoàn chỉnh và giữ đúng audio cần xác minh end-to-end. |
| Lấy audio Radar | Một phần | Code tải video nguồn, tách audio và kiểm tra track; cần xác minh bằng video thật và đầu ra cuối. |
| Gửi TikTok Draft | Một phần | Adapter draft-first có; chưa chứng minh tài khoản/app thật đủ scope. Draft phải được người dùng hoàn tất trong TikTok. |
| Analytics/gợi ý | Một phần | TikTok Display API đã nối follower/video totals và current public-video views/likes/comments/shares; Growth so delta follower giữa snapshot cách nhau ít nhất 24 giờ. Job hoàn tất nhận link TikTok công khai để Growth ghép theo account + video ID. Affiliate Growth cũng tổng hợp đơn AccessTrade theo đúng `sub2` của job, tách trạng thái và hoa hồng đã duyệt; số liệu chỉ phản ánh các đơn đã đồng bộ, chưa xác minh với đơn thật. Chưa có retention, demographics, profile visits hoặc follows-per-video. TikTok Shop product card vẫn cần người dùng gắn thủ công trong TikTok. |
| Truyện audio | Để phase sau | Không nằm trong acceptance hiện tại. |

## Ưu tiên sửa

1. Dance detection/select phải không lấy nhầm video viral thường.
2. Hai luồng affiliate/viral phải dùng audio từ cùng video Radar được chọn.
3. Job lưu URL/video ID/audio provenance và báo đúng lỗi tải/tách/ghép audio.
4. Offer hiển thị chính xác commission; sales/rating chỉ hiện nếu có nguồn thật.
5. Video QC và gửi Draft phải giữ trạng thái thật, tránh retry gây job trùng.
6. Xác minh Display API scopes bằng account thật; metrics retention, demographics và order attribution chỉ mở khi có nguồn thật.

## Giới hạn chung

Provider Web AI có thể hỗ trợ tạo media nhưng trạng thái session không chứng minh gói miễn phí hoặc credits. Flow/Muse không được tự dùng dịch vụ tính phí. Không có đủ nguồn hợp lệ thì dừng job và báo rõ.
