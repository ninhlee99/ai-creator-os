# IMPLEMENTATION_STATUS — MiuNova

> Ảnh chụp source code: 2026-10-23.
> Đây là rà soát mã nguồn và tài liệu hiện có; không chứng minh mọi endpoint đã
> chạy thành công với credential thật.
> Mục tiêu đích: [PRODUCT_SPEC.md](PRODUCT_SPEC.md).
> Tên repository/module kỹ thuật vẫn là `ai-creator-os`; tên hiển thị sản phẩm
> là MiuNova theo quyết định U-05 trong [PRODUCT_DECISIONS](PRODUCT_DECISIONS.md).

## Tóm tắt

App hiện có nhiều khối nền: kho sản phẩm, AccessTrade, tạo shot plan, QC, growth
planner, agent harness và adapter xuất bản. Luồng end-to-end tự tìm hàng → tự
tạo clip → QC → đăng → học trên đủ năm nền tảng chưa hoàn chỉnh. Một số tài liệu
cũ gọi tính năng “hoàn thành” trong khi implementation bị giới hạn hoặc chờ
credential/quota.

## Bảng năng lực

| Năng lực đích | Trạng thái code | Bằng chứng/giới hạn |
|---|---|---|
| Kết nối 5 nền tảng | Một phần | Publisher factory có TikTok, Facebook, Instagram, Threads, YouTube. Flow credential khác nhau; Instagram/Threads dựa token/ID cấu hình. |
| Onboarding chỉ cần login social | Chưa đạt / mô hình mở | AccessTrade yêu cầu key `at.access_key` qua Settings; payout cũng cần publisher KYC, MST, e-contract và ngân hàng. Self-hosted VPS owner thường là người dùng; identity và payout chưa được MiuNova provision. UTM/sub-ID chỉ track nguồn, không chuyển payout. Nếu bật Web AI miễn phí, còn cần login provider một lần. Chưa có luồng chứng minh user chỉ phải kết nối social. |
| Săn sản phẩm affiliate | Một phần | AccessTrade datafeed/campaign/link/order client có trong internal/accesstrade. Chưa xác minh bằng AccessTrade key thật theo feature docs. |
| Chọn theo lượt bán/đánh giá | Chưa đạt | Product model có Rating/SoldCount; adapter hiện dùng AccessTrade `/v1/datafeeds`, parser/mapper không có hai field. Docs `/v1/top_products.total` ghi số lượng sản phẩm, không định nghĩa là số bán. TikTok Shop feed có `units_sold` nhưng đã bị loại khỏi scope trước đây; không tích hợp, không có kỳ sales gần đây được docs nêu và không có review. Chưa có review source được xác minh. Xem [RESEARCH](RESEARCH.md). |
| Agent tự chọn sản phẩm | Một phần | AT Hunter lọc campaign đã duyệt/feed còn mới rồi sort theo `price × commission_rate`. Cổng sản xuất hiện giữ offer thiếu sold/rating trong kho, không tạo affiliate video; datafeed hiện map chưa có hai trường nên mọi offer mới đang chờ evidence. |
| Tự tạo affiliate link | Một phần | AccessTrade `product_link/create` và thao tác UI có; AT Hunter tự động dùng `aff_link` trong datafeed. Chưa thấy hunter tạo link riêng theo publisher/channel/content để bảo đảm attribution cấp video. |
| Tự tạo video affiliate | Chưa đạt theo free-only | Manual Studio cần clip chuyển động operator cung cấp. AT Hunter chỉ nhận sản phẩm có sold/rating; datafeed hiện không có nên offer mới ở lại kho và không tạo video. Nhánh Google Flow vẫn cần bật `at.hunter_flow_credits`; free account/quota phải được xác minh riêng, chưa chứng minh chi phí $0 end-to-end. |
| Tự tạo video viral | Chưa đạt | Viral radar có; full character edit/video pipeline bị tắt theo free-only khi provider cần billing. |
| Free-account fallback | Một phần, chưa chứng minh end-to-end | Web AI pool chỉ chọn account active có cờ `free` do người dùng xác nhận; đăng nhập hỏi xác nhận vì provider không có API plan chung để app kiểm chứng. Flow và Muse direct call nhận HTTP 429 được phân loại chắc chắn trước submit thì lưu cooldown theo `Retry-After` và thử account free kế tiếp; hết nguồn thì dừng kèm lý do. Gemini API LLM/TTS/image bị khóa cho tới khi operator xác nhận toàn bộ key set miễn phí; thay đổi key tự thu hồi xác nhận. Veo bị chặn vì cần billing. Headless Flow/reference jobs và chi phí thực tế vẫn chưa có quota proof toàn luồng. |
| TikTok publish | Một phần / eligibility chưa xác minh | Content Posting API integration; mặc định draft-only. Direct Post cần creator info, preview, metadata/privacy/interactions, commercial disclosure và consent từng bài. TikTok guidelines loại công cụ chỉ phục vụ account developer/team. **Suy luận:** app có thể không đủ điều kiện audit Direct Post. O-04 phải giải quyết trước O-07; không tuyên bố public auto. Trang Terms/Privacy còn `CONTACT_EMAIL`; cần thay bằng thông tin liên hệ thật trước khi gửi app review hoặc public các trang này. |
| Facebook publish | Một phần | Facebook publisher qua Graph API hoặc CLI, mặc định draft-only. Credential/permission và trạng thái thực cần xác minh. |
| Instagram Reels | Một phần | Adapter có; mặc định tạo container/chờ, publish công khai khi bật IG_AUTO_PUBLISH. Không có draft API trong implementation notes. |
| Threads | Chưa đạt video | Adapter hiện đăng text kèm link; chưa đăng video trong flow hiện tại. Meta có tài liệu chính thức cho `video_url` trên public server. MiuNova chưa có Media Delivery Gateway. Route `/media/{name}` chỉ phục vụ app; `authGate` cũng chặn request chưa đăng nhập khi bật mật khẩu. Chưa có URL HTTPS tạm, giới hạn theo artifact, hoặc xác nhận Meta fetch/process/publish. Scope/app review/live behavior cũng chưa kiểm chứng. Xem [RESEARCH](RESEARCH.md). |
| YouTube publish | Một phần | Data API upload có; affiliate path private-first. Guard dùng bucket `videos.insert` riêng (mặc định 100 calls/ngày, 1 unit/call), reservation nguyên tử trước upload, ngày reset Pacific; provider báo quota cạn thì dừng tới lần reset. Giới hạn project thật vẫn cần kiểm tra trong Google Cloud Console. Cần OAuth/quota; public scheduling không phải mặc định. [Quota docs](https://developers.google.com/youtube/data/determine_quota_cost), [videos.insert](https://developers.google.com/youtube/data/reference/rest/v3/videos/insert). |
| Analytics đa nền tảng | Chưa đạt | YouTube Data/Analytics sources có. TikTok metrics source là stub; chưa thấy Meta/Instagram/Threads metric adapters. |
| Tự điều chỉnh mix viral/affiliate | Chưa đạt | Growth planner/format decisions có; không có bằng chứng cross-platform optimizer dùng doanh thu affiliate cùng growth metrics để đổi mix. |
| Dashboard cho người quan sát | Một phần | Có dashboard, harness, activity/decision logs; chưa hợp nhất toàn bộ trạng thái end-to-end và nguồn quota theo PRODUCT_SPEC. |
| Mobile từ VPS: social OAuth | Đã có cấu hình, cần xác minh deployment | TikTok dùng PKCE cho loopback/Desktop và state cho HTTPS Web callback qua `TIKTOK_REDIRECT_URI`; Facebook dùng `META_REDIRECT_URI`; YouTube dùng Google Web OAuth khi có `YOUTUBE_REDIRECT_URI` HTTPS, vẫn giữ Desktop/loopback khi để trống. Chưa xác minh trên VPS bằng client credentials thật. TikTok static relay còn là fallback copy/paste thủ công. |
| Web AI login khi app chạy VPS | Desktop-only trong source; mobile bị tắt; live VPS/provider chưa xác minh | UI ẩn nút login trên mobile/coarse pointer; server từ chối mobile request ở routes start/screen/input/status/cancel. Viewer chỉ gửi mouse/keyboard; Google Flow mở Google Account thay cho canvas nặng. CDP loopback-only, session ID ngẫu nhiên, HTTPS gate từ xa. Chưa chạy login thật với provider credentials. Xem [remote-webai-login](features/remote-webai-login.md). |

Sơ đồ quy tắc chọn account miễn phí và retry Flow: [MiuNova free-account routing](assets/miunova-free-account-rotation.svg).

## Chi tiết theo pipeline

### Product discovery

- Provider đang dùng là Accesstrade; marketplace discovery compatibility provider
  báo không có API trực tiếp được cấu hình.
- Shopee Affiliate, Rentracks, ADPIA và Awin mới được rà tài liệu công khai như
  ứng viên thay thế; chưa có adapter, credential, schema end-to-end, thị trường
  hoặc mức phí $0 được xác minh. Không tính các nguồn này là khả năng đang chạy;
  chi tiết và giới hạn bằng chứng xem [RESEARCH](RESEARCH.md).
- Feed parser đọc SKU, tên, ảnh, giá, link affiliate, category, shop và
  commission. Parser/mapper không có rating/sold count; product store không có
  bằng chứng cho hai thuộc tính này.
- AccessTrade `/v1/top_products` có date filters, nhưng tài liệu định nghĩa
  `total` là số lượng sản phẩm, không phải đơn vị bán theo offer. Code chưa gọi
  endpoint này; không dùng `total` làm sales evidence.
- TikTok Shop product feed docs có `units_sold`, commission và inventory; field
  có thể vắng nếu seller chưa cho phép chia sẻ chính xác và không có sales
  period được nêu. API đã bị loại khỏi scope theo quyết định trước; không dùng
  lại nếu chưa có quyết định mới. Dù mở lại, vẫn cần rating/review source.
- AT Hunter sort `Price × CommissionRate` và dùng link affiliate có sẵn từ
  feed. Endpoint tạo link có trong UI/API nhưng chưa chứng minh tự chạy và gắn
  attribution cho từng video.
- Cổng sản xuất AT Hunter giữ sản phẩm thiếu sold/rating trong catalog và báo
  lý do; parser datafeed hiện không map các field này nên chưa có offer đủ điều
  kiện để tự dựng affiliate video.
- Campaign có thể cần đăng ký/chờ duyệt trên AccessTrade; app không tự phê duyệt.
- Commission order sync phân biệt pending/approved/rejected.

### Affiliate/video

- Product Analyst và Director tạo brief/shot plan từ listing images.
- Manual Studio yêu cầu clip chuyển động từng shot từ operator. AT Hunter có
  thể tự sinh shot bằng Google Flow khi `at.hunter_flow_credits` bật; app hỏi
  xác nhận credits nhưng không kiểm tra cost/quota là $0. Flag mặc định tắt.
  Không tính nhánh này là tuân thủ free-only hoặc fallback miễn phí.
- Web AI Studio trực tiếp chỉ retry account Flow/Muse khác khi nhận HTTP 429
  trong bước được xác định chắc chắn là trước khi provider nhận job/prompt. Lỗi
  sau submit không retry để tránh tạo job trùng. Retry-After được lưu thành cooldown.
- Tier free là xác nhận của operator, không phải thông tin được provider trả.
  Tài khoản cũ/import chưa xác nhận sẽ bị loại khỏi pool tới khi đăng nhập lại.
- Gemini API chains yêu cầu xác nhận riêng gắn với fingerprint của bộ key hiện
  tại; đổi bộ key sẽ khóa calls tới khi xác nhận lại. Đây vẫn là tự khai báo,
  không phải billing check từ Google. Google Veo luôn bị chặn theo chính sách
  free-only vì endpoint cần project bật billing.
- Temporal/visual QC tồn tại; lấy mẫu frame không chứng minh mọi khoảnh khắc
  sản phẩm được dùng đúng.
- Autopilot không được đánh dấu thành công khi thiếu motion clips.

### Growth/metrics

- Growth planner, plan items, canary, kill/double-down và YouTube analytics
  infrastructure có trong code.
- YouTube Data API đọc lifetime channel stats; YouTube Analytics cần OAuth scope
  riêng cho các metric theo kỳ.
- TikTokSource trả trạng thái chưa kết nối; code không scrape TikTok.
- No source data means no valid cross-platform automatic optimization.
- Guard trong source hiện khớp tài liệu Google: `videos.insert` có bucket mặc
  định 100 calls/ngày, 1 unit/call và reset nửa đêm Pacific. Bộ đếm cũ được
  chuyển đổi theo số lần upload thành công đã lưu; các request cũ thất bại
  không thể khôi phục từ dữ liệu cũ. App không thể thấy request ngoài app trước
  lần API đầu tiên; khi Google trả lỗi quota, guard khóa tới ngày reset.
  Giới hạn được cấp thật có thể khác; xem [Quota Calculator](https://developers.google.com/youtube/data/determine_quota_cost),
  [videos.insert](https://developers.google.com/youtube/data/reference/rest/v3/videos/insert)
  và [Revision History](https://developers.google.com/youtube/data/revision_history).

### Publishing

| Platform | Hiện trạng implementation |
|---|---|
| TikTok | Draft-first; UX Direct Post cần per-post controls + consent; nghĩa “một chạm” chưa chốt (O-07) |
| Facebook | Page publish adapter; draft-only mặc định |
| Instagram | Reels adapter; direct public publish là opt-in |
| Threads | Text + link; không phải video publish |
| YouTube | OAuth/Data API; affiliate mặc định private-first và có quota guard |

Build/credential/API review/live behavior cần kiểm chứng trước khi nâng các dòng
này lên “đã đạt”.

### Media delivery cho Threads

- `internal/web/routes.go` đăng ký `/media/{name}`; handler đọc file cục bộ từ
  output directory. Route nằm sau `authGate`, nên URL này không phải nguồn công
  khai cho Meta khi dashboard bật mật khẩu.
- Không thấy code cấp URL HTTPS tạm, capability token theo artifact, TTL/thu
  hồi hoặc trạng thái fetch từ Meta. Vì vậy Threads video chưa publish được qua
  source hiện tại; không tính text + link là hoàn thành mục tiêu video.
- Đích thiết kế nằm trong [PRODUCT_SPEC](PRODUCT_SPEC.md) và
  [SYSTEM_BLUEPRINT](SYSTEM_BLUEPRINT.md): VPS public HTTPS hoặc storage miễn
  phí cấu hình sẵn, URL đọc một artifact đã QC, hết hạn/thu hồi, không mở
  dashboard hay thư mục media.

## Kết luận

Hiện trạng không đạt acceptance “người dùng chỉ kết nối social rồi hệ thống tự
chọn sản phẩm, tự tạo video, đăng và học trên đủ năm nền tảng”. Khoảng cách lớn
nhất: sales/rating provider, affiliate payout provisioning, tự tạo video miễn
phí có fallback hợp lệ, TikTok consent UX, Threads video và analytics đa nền
tảng.
