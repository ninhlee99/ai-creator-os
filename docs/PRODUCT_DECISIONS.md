# PRODUCT_DECISIONS — Quyết định và giả định

> Cập nhật: 2026-10-23. Tài liệu tách lựa chọn người dùng khỏi đề xuất/giả định
> của nhóm thiết kế.

## Đã được người dùng nói rõ

| ID | Quyết định | Tác động lên spec |
|---|---|---|
| U-01 | Sản phẩm phải tự tìm cơ hội affiliate, tự tạo video affiliate và video viral để phát triển kênh trên TikTok, Facebook, Threads, Instagram, YouTube. | Cả hai lane nằm trong một vòng tự hành; không dừng ở product catalog hay radar. |
| U-02 | Người dùng muốn quan sát hệ thống; agent tự phân tích, chọn sản phẩm và quyết định nội dung. Người dùng chỉ muốn đăng nhập các tài khoản social cần thiết; nếu chọn account AI web miễn phí, login một lần trên desktop khi provision theo U-07. | Không yêu cầu chọn sản phẩm, tải clip hoặc duyệt nội dung định kỳ. Affiliate identity/payout cần thiết nhưng chưa có mô hình không buộc người dùng self-hosted nhập thêm credential; đây là blocker O-01, chưa được tính là đạt. |
| U-03 | Chủ sản phẩm yêu cầu một chạm xác nhận cho từng bài TikTok. | Yêu cầu đã ghi nhận; phạm vi “một chạm” chưa thể chốt trước UX bắt buộc của TikTok Direct Post. Xem O-07. |
| U-04 | Chỉ dùng tài nguyên miễn phí. Khi một tài khoản/provider hết quota, thử tài khoản miễn phí khác; khi mọi nguồn phù hợp đều hết, dừng và báo. | Cấm fallback trả phí; quota exhaustion là trạng thái chờ có giải thích. |
| U-05 | Tên hiển thị hệ thống là **MiuNova**; gợi mèo dễ thương và không gian; không dùng “OS” làm tên sản phẩm. | Tên và mascot mèo đã được người dùng chốt. Giữ `ai-creator-os` và `aicos` làm định danh kỹ thuật tương thích. |
| U-06 | Khi app chạy trên VPS, người dùng cần đăng nhập provider AI từ điện thoại; Playwright có thể thay CDP. | Superseded by U-07 on 2026-10-23. |
| U-07 | Web AI login chỉ hỗ trợ trên desktop; bỏ khả năng login từ điện thoại. | Ẩn login action trên thiết bị mobile và chặn mobile request ở server. Google Flow login mở Google Account page để tránh tải Flow canvas; login thật vẫn cần live verification. |

## Suy ra từ yêu cầu; chưa cần cấu hình người dùng hằng ngày

| ID | Giả định thiết kế | Vì sao | Trạng thái |
|---|---|---|---|
| A-01 | Agent tự thay đổi tỷ lệ nội dung organic/affiliate theo kết quả; không dùng tỷ lệ cố định toàn cục. | Người dùng giao agent tự phân tích và quyết định. | Giả định hợp lý; cần hiệu chỉnh sau khi có metrics thật. |
| A-02 | Affiliate connector được provision ở cấp VPS/instance trước onboarding. | Có thể tránh login affiliate lặp lại trong vận hành. | Chưa chứng minh đạt U-02: self-hosted VPS owner thường là chính người dùng, nên việc nhập key/payout vẫn là bước thêm ngoài social login. |
| A-03 | Việt Nam là locale khởi đầu. | Integration hiện tại là AccessTrade Vietnam; nội dung yêu cầu bằng tiếng Việt. | Giả định; chưa được xác nhận. |
| A-04 | Kết nối social qua cơ chế chính thức; không điều khiển web UI để né quyền/quota nền tảng. | OAuth/API là flow hiện có và trạng thái cần truy vết được. | Khuyến nghị kỹ thuật, chưa có ADR riêng. |
| A-05 | Dịch vụ cần billing không chạy; mức chi tiêu mặc định bằng 0. | Người dùng yêu cầu chỉ miễn phí. | Đã ràng buộc trong U-04. |
| A-06 | Cổng sản phẩm khởi điểm: rating chuẩn hóa ≥4,5/5 và ≥20 reviews; sales 30 ngày cao hơn trung vị offer cùng ngành/thị trường; trong nhóm đạt cổng, ưu tiên hoa hồng ròng kỳ vọng thuộc top 25%. | Biến “đánh giá tốt, nhiều người mua, hoa hồng cao” thành điều kiện có thể kiểm tra; khi thiếu feed hoặc nhóm so sánh, chỉ watchlist. | Đề xuất mặc định của spec; hiệu chỉnh theo kết quả thật, không hạ cổng để lấp lịch. |
| A-07 | Sau hard gate, điểm xếp hạng khởi điểm: 35% hoa hồng ròng kỳ vọng (hoặc ghi rõ ước tính gộp), 30% sales 30 ngày, 20% phù hợp khán giả, 15% độ tin cậy review; chiều dữ liệu thiếu không được bù bằng 0/suy đoán. | Làm rõ cách agent so offer cùng ngành/thị trường; tách cổng đủ điều kiện khỏi điểm ưu tiên. | Đề xuất thiết kế, chưa được người dùng duyệt; chỉ dùng để xếp tương đối và cần hiệu chỉnh theo kết quả thật. Chi tiết tại [PRODUCT_SPEC](PRODUCT_SPEC.md). |

## Mục còn mở

| ID | Cần quyết định/kiểm chứng | Khuyến nghị |
|---|---|---|
| O-01 | Làm sao có publisher identity, credential, campaign approval và payout route mà người dùng chỉ cần đăng nhập social? AccessTrade yêu cầu publisher tự hoàn tất CCCD/MST, e-contract và tài khoản ngân hàng để nhận payout; UTM/sub-ID chỉ là tracking. Rentracks công bố API cho subnet nhưng chưa công bố payout split/delegation. Với self-hosted app, nếu VPS owner phải đăng nhập/configure affiliate thì U-02 không đạt. | Chốt mô hình payout trước. Kiểm chứng bằng điều khoản/thoả thuận provider xem publisher identity do MiuNova/operator quản lý có được phép và có settlement hợp lệ tới từng người dùng không; không coi tracking tags hay subnet API là payout routing. Nếu không có mô hình đó, affiliate setup một lần của người dùng là ngoại lệ cần họ chấp thuận rõ. Xem [RESEARCH](RESEARCH.md). |
| O-02 | Nguồn affiliate nào cung cấp sales gần đây, rating/review và refund signals có provenance? AccessTrade `/v1/datafeeds` không có sales/review; `/v1/top_products.total` được docs mô tả là số lượng sản phẩm, không phải số bán. Shopee API, Rentracks, ADPIA và Awin là ứng viên nhưng docs đã rà chưa chứng minh đủ sales theo kỳ, review/rating, thị trường và truy cập miễn phí. TikTok Shop feed có `units_sold`/commission nhưng đã bị loại khỏi scope; cũng thiếu kỳ bán gần đây và review. | Giữ TikTok Shop API ngoài scope theo quyết định trước. Chỉ bật nguồn khác sau khi xác minh schema, sales window, review sample/freshness, quyền publisher và chi phí $0. Nếu chưa có nguồn qua gate, affiliate lane hold; organic tiếp tục. Chỉ mở lại TikTok Shop bằng quyết định mới; xem [RESEARCH](RESEARCH.md). |
| O-03 | Những tài khoản AI miễn phí nào hợp lệ để dùng; provider cho phép nhiều account/rotation tới đâu? | Chỉ xoay giữa account người dùng được quyền dùng, theo quota/điều khoản provider; không né quota. |
| O-04 | App tự host cho một người dùng có đủ điều kiện dùng TikTok Content Posting API không? Hướng dẫn hiện hành yêu cầu app nhắm tới creator audience rộng, không chỉ nhóm nội bộ/riêng tư; đồng thời nêu công cụ upload lên tài khoản do developer/team quản lý là không chấp nhận. Điều khoản repo mô tả app là personal/self-hosted, không cung cấp công khai. | **Suy luận:** cấu hình sản phẩm hiện tại có xung đột intended-use và có thể không qua audit Direct Post. Khuyên giữ app riêng tư, dùng xuất file/chuyển sang TikTok app để đăng; chỉ mở rộng thành sản phẩm công khai nếu chủ sản phẩm chấp nhận scope và TikTok xác nhận eligibility. Upload-to-Draft cần kiểm tra riêng, không giả định được miễn trừ. |
| O-05 | Threads API video được Meta document trong Postman chính thức; app MiuNova có lấy được scope/app review, media URL và quyền metrics cần thiết trên account thật không? Canonical docs/change log cần đối chiếu lại. | Thực hiện live verification với credential thật; media cần URL HTTPS công khai tạm thời theo artifact, trạng thái Meta fetch/process và kết quả publish riêng. Ưu tiên cấp từ VPS qua Media Delivery Gateway; không mở `/media` hay dashboard. Chưa xác minh thì adapter video vẫn tắt/chờ. Xem [RESEARCH](RESEARCH.md). |
| O-06 | Có giữ storytelling dài YouTube như một lane riêng không? | Giữ code/tài liệu hiện có như năng lực phụ, nhưng không để nó thay thế mục tiêu affiliate + channel growth. |
| O-07 | “Một chạm” TikTok nghĩa là một nút xác nhận cuối sau khi người dùng chọn thông tin bắt buộc cho từng bài, hay một thao tác duy nhất để gửi vào Draft rồi tự hoàn tất trong TikTok? Direct Post hiện yêu cầu preview, metadata chỉnh được, privacy/interactions được chọn thủ công và consent rõ ràng. | Khuyên dùng Direct Post với một nút xác nhận cuối sau các lựa chọn bắt buộc. Nếu bắt buộc chỉ một thao tác tổng, dùng luồng upload Draft và chấp nhận bước đăng cuối trong ứng dụng TikTok. Không bỏ qua UX bắt buộc. |

## Thứ tự mở khóa theo lane

| Lane | Điều kiện cần trước khi chạy thật | Nếu chưa đạt |
|---|---|---|
| Nội dung organic | Có ít nhất một nguồn tạo media miễn phí, được phép theo U-04; social account và quyền publish phù hợp cho từng nền tảng. | Chỉ nguồn/job phụ thuộc mới chờ; không gọi dịch vụ trả phí. Nếu không còn nguồn free hợp lệ, dừng job và báo. |
| Sản phẩm affiliate | O-01 publisher identity/payout hợp lệ **và** O-02 có sales + review evidence qua gate; campaign được duyệt và tracking link hợp lệ. | Đưa offer vào watchlist; không tạo affiliate video. Organic vẫn chạy độc lập. |
| TikTok publish | O-04 app eligibility/scope **trước** O-07 UX xác nhận theo bài; OAuth và quyền creator hợp lệ. | Xuất file/handoff sang TikTok; không gọi API hay browser automation để lách eligibility. |
| Threads video | O-05 app scope/live verification, HTTPS Media Delivery Gateway theo artifact, Meta fetch/process/publish status thật. | Video chờ/không hỗ trợ; không tính text post là video. |
| YouTube long-form | O-06 xác nhận có lane storytelling riêng, cùng với quota và quyền publish/analytics đã xác minh. | Có thể hoãn; không chặn short-form organic hoặc affiliate lanes. |
| Web AI provider | Chỉ cần khi instance chọn Web AI provider; login một lần trên desktop. Login điện thoại không hỗ trợ. | Dùng provider API miễn phí khác nếu được phép; nếu không còn nguồn thì dừng job và báo. |

Dependency chính: **O-03** mở sản xuất media cho lane cần AI; **O-01 và O-02**
cùng mở affiliate; **O-04 phải có trước O-07** cho TikTok. Kiểm chứng O-05 riêng
cho Threads. O-06 là tùy chọn. Đây là dependency của thiết kế, không phải kết
luận rằng các lane đã được triển khai.

## Ràng buộc nền tảng đã kiểm tra

- TikTok Content Sharing Guidelines (cập nhật 2026-08-04) yêu cầu API client
  nhắm tới audience creator rộng, không chỉ nội bộ/riêng tư; nêu công cụ chỉ
  upload lên account do developer/team quản lý là không chấp nhận. Điều khoản
  repo hiện mô tả MiuNova là app cá nhân tự host, không cung cấp công khai:
  [tiktok-app-site/terms.md](tiktok-app-site/terms.md). **Suy luận:** đây là
  xung đột intended-use đáng kể, không phải xác nhận chính thức TikTok sẽ từ
  chối; không được giả định audit sẽ qua.
- Với Direct Post, TikTok yêu cầu creator info mới nhất, preview, metadata có
  thể sửa, lựa chọn privacy/interactions theo thông tin creator, khai báo
  commercial content và consent rõ ràng cho từng bài. Người dùng tự chọn
  privacy/interactions, không dùng mặc định. Client chưa audit bị giới hạn ở
  private. Xem
  [Direct Post](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)
  và [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines).
- O-04 phải được giải quyết trước O-07: nếu app giữ private/self-hosted, TikTok
  có thể chỉ hỗ trợ handoff thủ công nếu API eligibility không được cấp. Audit
  không xóa các lựa chọn và consent theo bài.
- AccessTrade Publisher FAQ hiện yêu cầu publisher hoàn thành định danh CCCD,
  mã số thuế, hợp đồng điện tử và thông tin ngân hàng để nhận hoa hồng; publisher
  đủ điều kiện được trả về tài khoản ngân hàng mặc định đã đăng ký. API tạo link
  gọi UTM/sub1–sub4 là tham số tracking, không mô tả chia payout hay ủy quyền
  publisher. **Suy luận:** operator-managed credential không tự chuyển tiền
  hợp lệ tới người dùng; cần xác nhận mô hình với AccessTrade trước khi chốt O-01.
  Xem [RESEARCH](RESEARCH.md).

Mục mở không được trình bày như đã được người dùng duyệt. Khi được chốt, ghi
ngày và sửa PRODUCT_SPEC/IMPLEMENTATION_STATUS nếu cần.
