# PRODUCT_SPEC — MiuNova

> Trạng thái: đặc tả mục tiêu sản phẩm. Không phải bằng chứng các tính năng đã
> chạy.
> Phiên bản: 0.1
> Cập nhật: 2026-10-23
> Trạng thái code thật: [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md)
> Bản vẽ kiến trúc và từng job: [SYSTEM_BLUEPRINT.md](SYSTEM_BLUEPRINT.md)

## 1. Tầm nhìn

MiuNova là hệ thống tự vận hành một danh mục kênh nội dung và affiliate.
Hệ thống nghiên cứu người xem, tìm sản phẩm phù hợp có khả năng bán, tạo nội
dung video gốc, xuất bản theo từng nền tảng, đo kết quả rồi tự thay đổi kế
hoạch.

Người dùng không chọn sản phẩm, viết kịch bản, tải clip, xếp lịch hay duyệt
từng video. Người dùng kết nối tài khoản mạng xã hội, theo dõi kết quả và nhận
cảnh báo. Chủ sản phẩm yêu cầu xác nhận từng bài TikTok; “một chạm” đang chờ
làm rõ vì Direct Post cần lựa chọn và consent riêng cho từng bài (O-07).

## 2. Người dùng và kết quả cần đạt

### Người dùng chính

Một nhà sáng tạo/đơn vị vận hành dùng app trên VPS, thường theo dõi bằng điện
thoại. Người dùng muốn hệ thống làm việc liên tục và chỉ cần can thiệp khi phải
kết nối lại tài khoản, xác nhận bài TikTok, hoặc xử lý cảnh báo mà hệ thống
không thể tự khắc phục.

### Kết quả người dùng mong đợi

- Sản phẩm được chọn dựa trên hoa hồng, bằng chứng nhu cầu, đánh giá và độ phù
  hợp với khán giả của từng kênh.
- Video affiliate có liên kết tracking hợp lệ, nêu đúng thuộc tính sản phẩm và
  có công bố nội dung thương mại phù hợp.
- Video gốc không bán hàng trực tiếp vẫn được tạo để tăng reach, thời lượng xem
  và người theo dõi.
- Nội dung được chuyển thể theo TikTok, Facebook, Threads, Instagram và
  YouTube; một file không mặc định đăng nguyên xi lên mọi nơi.
- Hệ thống tự đo lượt xem, giữ chân, theo dõi, click, đơn và hoa hồng khi
  nguồn dữ liệu cho phép.
- Dashboard cho thấy việc đang chạy, lý do agent chọn/bỏ sản phẩm, QC, bài đã
  đăng, lỗi, chi phí và kết quả thật.

Không cam kết video sẽ viral, sản phẩm sẽ bán, hay tài khoản sẽ tăng trưởng
theo một con số cố định.

## 3. Nguyên tắc sản phẩm

1. **Tự hành theo mặc định:** agent tự nghiên cứu, quyết định và thực thi trong
   ngân sách, quyền API và chính sách đã cấp.
2. **Người dùng quan sát:** dashboard và thông báo là bề mặt chính. Không có
   cổng duyệt từng video; TikTok có cổng theo từng bài. Cách hiểu “một chạm”
   phải tuân theo UX bắt buộc và đang chờ quyết định O-07.
3. **Bằng chứng trước lời khẳng định:** không bịa lượt bán, đánh giá, view, đơn,
   hoa hồng hay trạng thái xuất bản. Số liệu thiếu phải hiện “chưa có dữ liệu”.
4. **Sản phẩm phù hợp trước hoa hồng danh nghĩa:** tỷ lệ cao không đủ để chọn
   sản phẩm nếu nhu cầu, chất lượng nguồn, độ phù hợp hoặc khả năng chuyển đổi
   yếu.
5. **Đo tiền đã duyệt:** tách hoa hồng dự kiến, đang chờ, bị từ chối và đã
   duyệt. Tối ưu theo kết quả ròng khi có dữ liệu hoàn/hủy.
6. **Nội dung gốc và đúng sự thật:** không reup, giả trải nghiệm, dựng đánh giá
   giả hoặc thêm tuyên bố không có bằng chứng.
7. **Adapter theo năng lực nền tảng:** mỗi kênh có trạng thái riêng: sẵn sàng,
   cần xác nhận, chờ quyền, chờ quota, lỗi hoặc chưa được hỗ trợ.
8. **Chi phí video bằng 0:** chỉ dùng dịch vụ/tài khoản miễn phí được cấu hình
   hợp lệ. Khi một tài khoản hết quota, thử tài khoản miễn phí khác đã kết nối.
   Khi mọi nguồn miễn phí phù hợp đều hết quota/lỗi, dừng job và báo người
   dùng. Không tự chuyển sang dịch vụ trả phí.

## 4. Ranh giới onboarding và vận hành

### Việc người dùng làm

- Kết nối tài khoản mạng xã hội cần dùng và cấp quyền theo yêu cầu nền tảng.
- Nếu instance cần Web AI account, khôi phục account qua JSON backup một lần
  khi provision. MiuNova không đọc profile browser người dùng; account này là
  tùy chọn và không chặn job dùng provider API.
- Xác nhận từng bài TikTok; số thao tác và UX cuối chờ quyết định O-07.
- Theo dõi dashboard/cảnh báo; dừng hệ thống khi có sự cố nghiêm trọng.
- Kết nối lại tài khoản nếu token hết hạn hoặc bị thu hồi.

### Việc hệ thống tự làm

- Phân tích ngách, người xem, nội dung đã đăng và tín hiệu tăng trưởng.
- Khám phá sản phẩm, xác minh campaign/link và xếp hạng bằng dữ liệu có nguồn.
- Viết chiến lược, kịch bản, shot plan, caption, tiêu đề, hashtag và disclosure.
- Tạo video từ listing/media được phép và các provider miễn phí đang khả dụng.
- Chạy kiểm tra sự thật, sản phẩm, bản quyền/tài nguyên, chất lượng hình, âm
  thanh và thông số từng nền tảng.
- Lên lịch, xuất bản, kiểm tra trạng thái, đồng bộ số liệu và tự điều chỉnh.
- Thử lại lỗi tạm thời có giới hạn; dừng và báo khi nguồn đã cạn hoặc lỗi không
  thể tự khắc phục.

Luồng chính vẫn là người dùng kết nối social rồi quan sát. Web AI account là
nguồn tùy chọn và chỉ được khôi phục bằng JSON backup. MiuNova không đọc profile
browser người dùng và không có interactive browser login. Thiếu Web AI account
chỉ chặn job phụ thuộc nguồn đó; các luồng provider API vẫn hoạt động. Account
không cần xác nhận Free thủ công; Flow vẫn cần consent credits trước khi tạo
video.

### Điều kiện triển khai cấp hệ thống

Mục tiêu onboarding là người dùng chỉ kết nối social; không tự chọn hàng, tạo
video, nhập publisher key hay duyệt campaign. Tuy vậy affiliate network cần
publisher identity, credential, campaign approval và payout owner. Trong app
self-hosted, VPS owner thường chính là người dùng; yêu cầu họ nhập AccessTrade
key hoặc tự cấu hình payout sẽ vượt quá ranh giới “chỉ login social”.
Với AccessTrade hiện tại, payout còn yêu cầu publisher tự hoàn tất định danh,
mã số thuế, hợp đồng điện tử và thông tin ngân hàng. UTM/sub-ID trên link chỉ
phục vụ tracking; không phải cơ chế chuyển hoa hồng sang người dùng. Xem
[nghiên cứu payout](RESEARCH.md).

Vì vậy đây là blocker sản phẩm, không phải chi tiết vận hành đã giải quyết.
Trước khi gọi onboarding zero-touch hoàn tất, phải xác định ai provision
publisher identity và bằng cách nào commission tới đúng người dùng. Không dùng
credential của bên khác nếu chưa có quyền/thoả thuận payout; không tự đăng ký
hoặc đồng ý điều khoản thay người dùng. Nếu mô hình cuối vẫn yêu cầu người dùng
thực hiện affiliate setup một lần, phải ghi thành ngoại lệ được họ chấp thuận;
không âm thầm xem nó là “chỉ login social”. Xem
[PRODUCT_DECISIONS.md](PRODUCT_DECISIONS.md), O-01.

## 5. Vòng tự hành end-to-end

~~~text
OBSERVE CHANNELS
  → DISCOVER PRODUCTS + TRENDS
  → VERIFY EVIDENCE + ELIGIBILITY
  → RANK FOR CHANNEL/AUDIENCE
  → CREATE TRACKED AFFILIATE LINK (affiliate lane only)
  → PLAN ORGANIC + AFFILIATE CONTENT
  → GENERATE PLATFORM-SPECIFIC ASSETS
  → QC + CLAIM CHECK
  → PUBLISH (TikTok: per-post controls + explicit consent; O-07 pending)
  → SYNC PERFORMANCE + COMMISSION
  → LEARN + REPLAN
~~~

Mỗi job lưu trạng thái và bằng chứng theo từng bước. Tiến trình hiển thị phải
đọc từ event/job thật, không dựng hoạt động agent giả.

Các trạng thái chờ cần phân biệt: thiếu connector, campaign chưa duyệt, token
hết hạn, API không hỗ trợ, quota miễn phí cạn, thiếu dữ liệu, QC thất bại, chờ
xác nhận TikTok và đang xử lý trên nền tảng.

## 6. Khám phá và chọn sản phẩm

### 6.1 Nguồn dữ liệu

Mỗi offer phải có provider/source, campaign, thời gian cập nhật, giá/tiền tệ,
link tracking, điều kiện hoa hồng và provenance cho từng thuộc tính quan trọng.
Ưu tiên API/feed affiliate chính thức. Không crawl dữ liệu riêng tư hoặc giả
lập field không có trong feed.

Với AccessTrade, adapter hiện dùng `/v1/datafeeds`, vốn có product/price/
commission/link/update nhưng không có sales hay review. `/v1/top_products`
nhận date filter nhưng docs mô tả `total` là “số lượng sản phẩm”; không định
nghĩa đây là số đơn/units sold theo offer hay cửa sổ bán, nên không dùng làm
sales evidence. TikTok Shop product feed có `units_sold` và commission trong
docs, nhưng API này đã bị loại khỏi scope trước đây; field có thể vắng và docs
không nêu kỳ bán gần đây hoặc rating/review. Không tự bật lại integration đó.
Không cho offer từ nguồn hiện được phép vào affiliate production nếu chưa nối
sales evidence gần đây và review source đạt quality gate. Xem [RESEARCH.md](RESEARCH.md)
và [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

### 6.2 Cổng đủ điều kiện

Agent chỉ được đưa sản phẩm vào sản xuất khi:

- Offer thuộc campaign mà publisher account được quyền quảng bá.
- Link tracking tạo/kiểm chứng thành công.
- Giá, ảnh, trạng thái còn hàng và thông tin cơ bản đủ mới theo chính sách
  provider.
- Không có cảnh báo rủi ro/ngành hàng bị cấm.
- Sản phẩm phù hợp với chủ đề, brand DNA và khán giả của kênh.
- Các tuyên bố trong kịch bản có bằng chứng từ nguồn tin cậy.
- Có bằng chứng nhu cầu bán hàng gần đây và bằng chứng đánh giá từ nguồn có
  provenance, thời điểm cập nhật và phạm vi thị trường rõ ràng. Thiếu một trong
  hai thì chỉ được đưa vào watchlist; không tạo video affiliate cho offer đó.
- Chính sách khởi điểm: điểm đánh giá chuẩn hóa tối thiểu 4,5/5 với ít nhất
  20 đánh giá; sales trong 30 ngày phải cao hơn trung vị của offer cùng ngành
  và thị trường. Nguồn không có cùng kỳ đo, thang điểm hoặc mẫu so sánh đáng tin
  cậy thì offer ở watchlist, không được tự suy diễn.

Campaign cần duyệt thủ công từ mạng affiliate không được coi là đã duyệt bởi
AI. Job chuyển sang trạng thái chờ và giải thích điều kiện.

### 6.3 Xếp hạng

Ranking chạy sau hard gates; chỉ so offer cùng thị trường và nhóm ngành khi có
thể. “Hoa hồng cao” nghĩa là khoản hoa hồng dự kiến được duyệt trên một đơn
thành công nằm trong nhóm cao của offer tương đương, không phải phần trăm hoa
hồng lớn nhưng giá trị đơn hoặc khả năng bán thấp. Cần tối thiểu:

- Hoa hồng ròng kỳ vọng trên đơn được duyệt, có tính điều kiện campaign và dữ
  liệu hủy/hoàn nếu provider cấp.
- Lượng bán và xu hướng bán trong khoảng thời gian có nguồn; chuẩn hóa theo
  ngày và xếp tương đối với offer cùng nhóm ngành/thị trường.
- Điểm đánh giá, số lượng và độ mới review. Điểm phải đạt quality floor theo
  thang đo của provider/thị trường; số review phải đủ để đạt confidence tối
  thiểu. Không chuẩn hóa được thang điểm hoặc mẫu quá nhỏ thì không qua gate.
- Chất lượng/độ tin cậy shop, tỷ lệ hủy/hoàn và ổn định giá/tồn kho nếu có.
- Độ phù hợp của sản phẩm với khán giả và lịch sử chuyển đổi của kênh.
- Khả năng tạo nội dung hữu ích, khác biệt, có thể trình diễn trung thực.
- Độ mới và độ tin cậy của dữ liệu.

Trong nhóm offer qua gate, ưu tiên offer có hoa hồng ròng kỳ vọng thuộc nhóm
25% cao nhất trong cùng ngành/thị trường, cùng sales trend và độ phù hợp với
khán giả. Điểm đề xuất phải kèm lý do, confidence và từng số liệu nguồn. Các
ngưỡng là chính sách khởi điểm có thể hiệu chỉnh từ kết quả thật; không hạ
quality floor chỉ để lấp lịch đăng.

> **Công thức xếp hạng khởi điểm — giả định A-07, chưa được người dùng duyệt:**
> chỉ tính trong nhóm cùng ngành/thị trường đã qua hard gate. Chuẩn hóa mỗi
> chiều theo percentile trong nhóm rồi tính `score = 35% hoa hồng ròng kỳ vọng
> + 30% sales 30 ngày + 20% độ phù hợp khán giả + 15% độ tin cậy review`.
> Hoa hồng ròng phải dùng tỷ lệ duyệt/hoàn nếu provider có dữ liệu; nếu chưa có,
> hiển thị rõ là ước tính gộp, không gọi là ròng. Sales phải có đúng cửa sổ thời
> gian, không đổi con số tích lũy thành sales gần đây. Điểm chỉ dùng để sắp thứ
> tự tương đối, không phải xác suất bán hay bảo đảm kết quả. Rủi ro shop,
> hoàn/hủy, tồn kho, vi phạm chính sách và dữ liệu quá cũ là cờ chặn/giảm hạng
> riêng; commission cao không được bù qua hard gate. Nếu thiếu một trong bốn
> chiều chính, không bù trọng số bằng số 0 hoặc dữ liệu suy đoán: offer quay về
> watchlist.

Nếu thiếu lượt bán hoặc đánh giá, agent giữ offer trong watchlist với trạng
thái “thiếu bằng chứng”, không gán hạng “bán chạy/đánh giá tốt” và không chuyển
sang affiliate production. Luồng organic vẫn có thể chạy độc lập. Người dùng
không phải duyệt từng sản phẩm. Nếu không có offer nào qua gate hoặc đạt mức
hoa hồng kỳ vọng cao, hệ thống không ép chọn sản phẩm; lane affiliate dừng với
lý do, lane organic tiếp tục.

### 6.4 Affiliate link và đối soát

Link phải gắn publisher account nhận hoa hồng, campaign hợp lệ và định danh
source/channel/content để đo được. Hệ thống đối soát click, đơn và trạng thái
hoa hồng; không cộng pending/rejected vào doanh thu đã duyệt.

## 7. Chiến lược nội dung và phát triển kênh

Mỗi kênh có chiến lược riêng. Agent duy trì hai lane:

- **Organic growth:** giải trí, giáo dục hoặc storytelling phù hợp ngách; mục
  tiêu là reach, xem hết, chia sẻ và follower mới.
- **Affiliate:** giải quyết nhu cầu người xem bằng sản phẩm phù hợp; mục tiêu
  là click và hoa hồng ròng.

Không đặt tỷ lệ cố định cho mọi kênh. Agent điều chỉnh mix theo giai đoạn kênh
và hiệu quả đo được: retention, follower conversion, click-through, đơn đã
duyệt và tần suất quảng bá gần đây. Không biến mọi video thành quảng cáo. Khi
metrics thiếu, giữ kế hoạch thận trọng và đánh dấu thiếu dữ liệu thay vì kết
luận format thắng/thua.

Trend được dùng làm tín hiệu để tạo ý tưởng gốc. Không tải lại hoặc sao chép
video/âm thanh có bản quyền. Một ý tưởng được chuyển thể theo từng nền tảng:
hook, thời lượng, khung hình, caption, metadata, disclosure và lịch đăng.

## 8. Tạo video và kiểm soát chất lượng

### 8.1 Tạo nội dung

- Pipeline affiliate không được yêu cầu người dùng quay/tải clip cho từng job.
- Pipeline viral phải tạo video gốc; radar chỉ là đầu vào nghiên cứu, không là
  nguồn để reup.
- Dùng nguồn miễn phí được cấp phép/cấu hình. Có thể chuyển sang tài khoản
  miễn phí/provider khác khi adapter trước hết quota, theo giới hạn của
  provider.
- Không vượt quota, tạo tài khoản hàng loạt hoặc xoay account nhằm né giới
  hạn. Nếu không còn nguồn miễn phí đủ điều kiện, job dừng với trạng thái và
  nguyên nhân cụ thể.
- Dịch vụ cần billing không được gọi. Không có fallback trả phí.

### 8.2 QC tự động

QC gồm kiểm tra xác định và agent critic độc lập:

- File đọc được, đúng khung hình/thời lượng/âm thanh theo nền tảng.
- Video không đứng hình, không rỗng/đen ngoài dự kiến, audio không clip.
- Sản phẩm hiển thị đúng listing và được dùng/trình bày đúng ngữ cảnh.
- Không có claim giá, hiệu quả, thành phần, chứng nhận hoặc trải nghiệm bịa.
- Không có nội dung vi phạm red lines, quyền riêng tư, bản quyền hay quy tắc
  thương mại.
- Caption, disclosure và affiliate link khớp từng nền tảng.

QC không thể chứng minh mọi frame hoàn hảo hoặc dự đoán viral. Thất bại sau số
vòng retry hữu hạn thì dừng, giữ artifact và báo lý do.

## 9. Xuất bản đa nền tảng

Yêu cầu đích: TikTok, Facebook, Threads, Instagram và YouTube. Adapter phải
đăng riêng từng nội dung/biến thể; kết quả từng nền tảng không được gộp thành
một trạng thái “đã đăng”.

| Nền tảng | Yêu cầu đích | Hành động người dùng |
|---|---|---|
| TikTok | Direct Post chỉ khi app đủ điều kiện; mỗi bài cần creator info mới nhất, preview, metadata/privacy/interactions, commercial disclosure, AI disclosure, consent và trạng thái riêng | Eligibility chờ O-04; thao tác xác nhận chờ O-07 |
| Facebook | Video/Reels/Page tùy loại account và quyền API | Không duyệt từng bài nếu API/permission cho phép |
| Instagram | Reels/format phù hợp account; media và disclosure đúng | Không duyệt từng bài nếu API/permission cho phép |
| Threads | Video/ảnh/text theo API được Meta document; video cần Media Delivery Gateway tạo URL HTTPS công khai tạm thời theo artifact, rồi container → publish. Bài text-only không được tính là video. App review/scope cần xác minh | Không duyệt từng bài nếu API/permission cho phép |
| YouTube | Shorts/video phù hợp kênh, metadata, privacy và lịch xuất bản | Không duyệt từng bài nếu API/permission cho phép |

“Đăng thành công” chỉ được ghi sau khi provider xác nhận trạng thái cuối hoặc
trạng thái xử lý được mô tả đúng. Nếu API yêu cầu domain công khai, app review,
scope hoặc account type, hệ thống hiển thị đó là điều kiện kết nối, không giả
là lỗi render.

### 9.1 Media Delivery Gateway cho API kéo file

Threads yêu cầu Meta tải video từ `video_url` trên public server. URL `/media`
nội bộ hoặc URL cần cookie đăng nhập không đáp ứng điều kiện này. MiuNova cần
một gateway qua HTTPS để cấp URL capability chỉ đọc, gắn với đúng artifact và
có thời hạn; không mở thư mục media hoặc dashboard cho truy cập ẩn danh.

- URL chỉ đọc đúng một artifact đã qua QC; không liệt kê file và không cho ghi.
- URL có token ngẫu nhiên đủ mạnh, thời hạn ngắn nhưng đủ để Meta tải/xử lý
  media; có thể thu hồi sau khi xác nhận tải xong hoặc hết TTL.
- Không ghi token đầy đủ vào log, event, analytics, caption hoặc UI. Ghi ID
  artifact, thời điểm cấp/hết hạn và kết quả fetch thay cho URL bí mật.
- Gateway dùng hostname HTTPS công khai của VPS hoặc storage miễn phí đã cấu
  hình. Không tự bật dịch vụ storage tính phí. Nếu endpoint công khai chưa sẵn,
  Threads video chờ/không hỗ trợ; text post là lane riêng.
- Retry tạo URL mới chỉ khi trạng thái provider cho thấy chưa tạo/xuất bản bài;
  không tạo bài trùng khi kết quả trước còn mơ hồ.

TikTok, Facebook, Instagram và YouTube hiện dùng luồng upload file trong adapter
đang có; không bắt buộc đi qua gateway này. Năng lực adapter thật xem
[IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md).

Với Direct Post, mỗi lần mở bước xuất bản phải lấy creator info mới nhất, hiện
nickname tài khoản và giới hạn thời lượng. Nếu creator không thể đăng lúc này
hoặc clip vượt giới hạn trả về, dừng và báo; không khởi tạo upload. Hiện preview
nội dung, title sửa được và privacy/interactions đúng theo creator options.
Privacy phải do người dùng chọn, không có lựa chọn mặc định; interaction
checkbox cũng không được chọn sẵn. Hiện lựa chọn disclosure thương mại phù hợp
(affiliate/branded content) và AI disclosure. Chỉ gửi media sau consent rõ ràng
riêng cho bài đó; sau đó theo dõi publish status thật. TikTok cấm
watermark/logo/link quảng bá chèn lên nội dung được chia sẻ. Vì vậy một nút xác
nhận cuối sau các lựa chọn bắt buộc là phương án khuyên dùng; “một chạm tổng”
trước khi chọn các mục bắt buộc không tương thích UX hiện hành.

**Cửa eligibility:** Content Sharing Guidelines yêu cầu API client nhắm tới
creator audience rộng, không chỉ nhóm nội bộ/riêng tư, và nêu công cụ upload
lên account do developer/team quản lý là không chấp nhận. Điều khoản trong repo
mô tả MiuNova là app cá nhân tự host, không cung cấp công khai.
**Suy luận:** mô hình hiện tại có thể không qua audit Direct Post; chỉ TikTok
mới xác nhận eligibility. Không coi app review là bước thủ tục chắc chắn.
Nếu giữ app private, phương án dự phòng là handoff file để người dùng đăng trong
TikTok; không dùng browser automation để né giới hạn API. Upload-to-Draft vẫn
cần kiểm tra eligibility riêng. O-04 phải chốt trước O-07.

Nguồn chính thức:
[Direct Post](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)
và [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines).

## 10. Analytics và vòng học

Nguồn số liệu tách theo provider, account, nội dung và thời điểm:

- Channel: followers/subscribers, views, video count, watch time nếu API cấp.
- Video: views, retention/average view, like/comment/share/save, follower gained.
- Affiliate: impressions/clicks, conversion, order, approval/rejection, commission.
- Operations: publish success, latency, retries, provider quota, free-account
  availability, estimated resource use.

Missing != zero. Số liệu phải lưu source, freshness, scope, period và confidence.
Agent chỉ kết luận về kênh/nội dung có đủ số liệu thật. Replan ưu tiên hoa hồng
đã duyệt cùng tăng trưởng bền vững; tránh tối ưu view gây lệch ngách hoặc spam.

## 11. Dashboard và vận hành trên VPS

- Mobile-first dashboard qua HTTPS; người dùng mở từ điện thoại khi app chạy
  trên VPS.
- Mọi social OAuth provider được bật phải trả callback HTTPS về đúng instance
  MiuNova trên VPS, với redirect URI đăng ký chính xác ở provider. Luồng chuẩn
  không yêu cầu localhost, mở desktop VPS hay chép authorization URL thủ công.
  TikTok dùng `/publishers/tiktok/callback`, Facebook dùng
  `/publishers/facebook/callback`; YouTube cần Google Web OAuth client và
  callback VPS. Static TikTok relay chỉ là phương án cứu hộ.
- Account AI web chỉ được khôi phục qua JSON backup; app không truy cập cookie
  trong profile browser của người dùng.
- Trang chủ ưu tiên: sức khỏe hệ thống, việc agent đang làm, blocker, kết quả
  publish, doanh thu đã duyệt, hiệu quả nội dung, quota/nguồn miễn phí.
- Mỗi quyết định quan trọng hiển thị lý do, dữ liệu dùng, thời gian đo và bước
  tiếp theo.
- Người dùng chủ yếu đọc. Cần có dừng khẩn cấp và khả năng ngắt token/provider;
  không yêu cầu người dùng điều phối pipeline thủ công.
- Phiên đăng nhập, token OAuth và credential provider phải được bảo vệ riêng;
  token không render ra UI/log.

## 12. Chỉ số thành công và tiêu chí chấp nhận

### Chỉ số sản phẩm

- Hoa hồng ròng đã duyệt theo kênh/sản phẩm/video.
- Tỷ lệ sản phẩm được chọn có dữ liệu giá, link và campaign còn hiệu lực.
- Video hoàn tất QC và publish theo từng adapter.
- Tăng trưởng follower, retention, click-through và chuyển đổi.
- Tỷ lệ job tự phục hồi; số job dừng có lý do rõ.
- Số tiền dịch vụ video phát sinh phải bằng 0.

### Acceptance criteria cho đích tự hành

1. Sau khi người dùng kết nối social, scheduler tự khám phá offer, tạo link
   tracking, sản xuất video, chạy QC, xếp lịch và xuất bản mà không yêu cầu họ
   chọn sản phẩm hoặc tải clip. Publisher identity, credential, campaign
   approval và payout route phải được provision trước mà không buộc người dùng
   làm thêm bước affiliate; nếu mô hình đó chưa được xác định thì zero-touch
   onboarding chưa đạt. Web AI account là nguồn tùy chọn và chỉ khôi phục từ
   JSON backup; thiếu account không chặn các job dùng provider API.
2. Mỗi sản phẩm affiliate được chọn để sản xuất có bằng chứng sales gần đây và
   rating/review có nguồn, freshness, scope và sample count; agent giải thích
   lý do chọn/bỏ. Review đạt quality floor và sample confidence tối thiểu;
   sales và hoa hồng được xếp tương đối trong nhóm ngành/thị trường. Thiếu
   bằng chứng thì chỉ watchlist, không affiliate video; nếu không có offer đạt
   chuẩn, organic vẫn chạy và dashboard báo lane affiliate đã dừng.
3. Một video có status độc lập cho TikTok, Facebook, Threads, Instagram và
   YouTube. Chỉ bật TikTok API sau khi xác minh app eligibility, scope và
   audit. Nếu đủ điều kiện, mỗi bài phải lấy creator info mới nhất, hiển thị
   nickname và giới hạn video; dừng nếu
   creator không thể đăng hoặc clip vượt giới hạn. Privacy và interactions
   phải hiện đúng options, không có lựa chọn mặc định, người dùng tự chọn; có
   preview, title sửa được, commercial/AI disclosure và consent riêng trước khi
   gửi media. Ghi status theo provider. Nếu app vẫn private hoặc không đủ điều
   kiện, dùng handoff thủ công hoặc đánh dấu unsupported; không lách qua browser.
   O-04 chốt trước O-07. Các platform khác tự publish khi permission và
   capability sẵn sàng.
4. Nếu account Web AI hết quota, thử account khác đã kết nối; nếu hết nguồn
   khả dụng, dừng, lưu trạng thái và báo người dùng. Flow yêu cầu người dùng
   xác nhận credits trước khi tạo video.
5. Không có nguồn metrics thì không ghi số 0 thay cho thiếu dữ liệu và không
   tuyên bố chiến lược đã tối ưu.
6. Mọi bước có idempotency/retry giới hạn, audit log và kill switch.
7. Người dùng xem được trên điện thoại qua VPS HTTPS và hoàn tất OAuth trên
   từng social provider được bật bằng callback HTTPS quay về VPS; không cần
   localhost hoặc chép URL thủ công. Web AI account được khôi phục qua JSON
   backup; app không đọc cookie từ browser đang chạy. Cookie được cập nhật sau
   request khi provider trả cookie mới.
8. Threads video chỉ được tính đã publish khi video qua URL HTTPS công khai
   tạm thời theo artifact, Meta xác nhận xử lý/xuất bản và app ghi status thật.
   URL không mở dashboard hay thư mục media; thiếu gateway thì video ở trạng
   thái chờ/không hỗ trợ, không tính bài text thành video.
9. Có hai lane độc lập: video affiliate chỉ chạy khi offer qua evidence gate;
   video organic tạo nội dung gốc để phát triển kênh. Thiếu sales/review chỉ
   dừng lane affiliate; organic vẫn chạy. Trend chỉ làm tín hiệu ý tưởng, không
   làm nguồn reup. Mỗi nền tảng nhận bản chuyển thể phù hợp. Khi mọi provider
   miễn phí đủ điều kiện cho một job đã hết quota/lỗi, job đó dừng và báo;
   không fallback trả phí. Metrics thiếu không được biến thành số 0 hay lời
   khẳng định đã tối ưu; không hứa viral.
10. Người dùng chủ yếu quan sát dashboard dữ liệu job/event thật, lý do và nguồn
    cho quyết định, artifact/QC, trạng thái từng nền tảng, quota và blocker.
    Thao tác thường lệ giới hạn ở kết nối social, xác nhận từng bài TikTok theo yêu cầu nền tảng và xử lý
    cảnh báo không tự phục hồi. Không yêu cầu chọn sản phẩm, tải clip hoặc duyệt
    video định kỳ.

### Bản đồ yêu cầu người dùng

| Yêu cầu đã chốt | Phần spec | Acceptance criteria | Ghi chú |
|---|---|---|---|
| U-01 — tự tìm offer, tạo video affiliate và nội dung tăng trưởng trên năm nền tảng | §5–§10 | AC1–3, AC8–9 | Sales/review thiếu thì affiliate hold; organic độc lập. |
| U-02 — người dùng chủ yếu quan sát, chỉ kết nối social | §2–§4, §11 | AC1, AC7, AC10 | Publisher identity/payout còn blocker O-01. |
| U-03 — xác nhận từng bài TikTok | §3, §9 | AC3, AC10 | Số thao tác phụ thuộc UX bắt buộc; O-07. |
| U-04 — chỉ miễn phí, hết mọi nguồn thì dừng và báo | §3, §8 | AC4, AC9 | Chỉ thử account/provider được phép; không né quota. |
| U-05 — tên hiển thị MiuNova, không dùng OS làm tên sản phẩm | Tên tài liệu, logo và sơ đồ | — | `ai-creator-os`/`aicos` chỉ là định danh kỹ thuật tương thích. |
| U-06 — login Web AI từ điện thoại khi app ở VPS | §4, §11 | — | Superseded by U-07 (2026-10-23). |
| U-07 — login Web AI chỉ trên desktop cùng máy chạy app; chặn login từ điện thoại | §4, §11 | AC1, AC7 | Superseded by U-08 (2026-10-23). |
| U-08 — giữ login Web AI trên desktop, không mở login từ điện thoại | §4, §11 | AC1, AC7 | Superseded by U-09 (2026-10-23). |
| U-09 — chỉ giữ tự động đọc cookie nếu dùng được browser người dùng chọn; nếu không, bỏ phần đó và tập trung chức năng cốt lõi | §4, §11 | — | Confirmed by U-11. |
| U-10 — bỏ xác nhận Free account; không stream browser | §4, §11 | AC1, AC7 | Ngoại lệ U-04 cho account Web AI do người dùng cung cấp; trạng thái không khẳng định plan/credit. Flow vẫn giữ consent credits. U-11 quyết định bỏ interactive browser login. |
| U-11 — chỉ giữ auto-cookie nếu dùng được browser người dùng chọn; nếu không, gỡ browser login | §4, §11 | AC1, AC7 | Account Web AI optional; chỉ khôi phục qua JSON backup. CDP riêng vẫn dùng nội bộ cho job Flow/Muse, không dùng để đọc browser profile người dùng. |

## 13. Ngoài phạm vi

- Cam kết chắc chắn doanh số, viral, CPM, follower hoặc thời gian tăng trưởng.
- Tự đăng ký/đồng ý điều khoản mạng affiliate thay mặt người dùng.
- Tự mua mẫu sản phẩm, hoàn hàng hoặc xử lý thanh toán.
- Reup video/nhạc người khác hoặc làm giả đánh giá/trải nghiệm.
- Vượt quota, bypass app review, giả danh creator hoặc né nền tảng.
- Tự chi tiền hoặc chuyển qua dịch vụ có billing.

## 14. Việc còn mở

Các mục dưới đây phải chốt hoặc được kiểm chứng trước khi tuyên bố đạt đích:

- Provider affiliate nào cung cấp sales, rating/review, tồn kho và dữ liệu trả
  hàng; credential được provision bởi ai; payout thuộc publisher account nào.
- Ngân sách/quyền dùng các tài khoản AI miễn phí; cách xác định account hợp lệ
  và quota của từng provider.
- TikTok app review, scope publish, Direct Post UX và quyết định O-07 về phạm
  vi “một chạm”.
- OAuth trên VPS cho mọi social provider bật: xác minh TikTok, Facebook và
  Google Web OAuth callback trên VPS bằng credentials/consent thật.
- Quyền analytics/publish chính thức của từng nền tảng, kể cả Threads video.
- Hostname HTTPS và cơ chế cấp/thu hồi URL media tạm cho Threads video; VPS có
  public HTTPS nhưng route `/media` hiện không phải gateway publish.
- Niche/locale mặc định. Hiện trạng Accesstrade nghiêng về thị trường Việt Nam.
- Ngưỡng dữ liệu tối thiểu trước khi agent tuyên bố một sản phẩm/format thắng.
