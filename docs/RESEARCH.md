# RESEARCH.md — Nghiên cứu nền tảng và kho tư liệu

> PRODUCT_SPEC và PRODUCT_DECISIONS là nguồn quyết định sản phẩm. Mục “Current
> findings” dưới đây được rà lại ngày 2026-10-22. Tư liệu cũ phía sau giữ để
> tham khảo lịch sử; không dùng kế hoạch/policy cũ thay cho spec hiện hành.

## Current findings — 2026-10-22

### TikTok Content Posting API

- Content Sharing Guidelines cập nhật 2026-08-04: API client phải phục vụ
  creator audience rộng, không chỉ nhóm nội bộ/riêng tư; TikTok nêu rõ utility
  chỉ upload cho account developer/team quản lý là use case không chấp nhận.
  Client chưa audit chỉ đăng private; Direct Post còn phải lấy creator info,
  cho creator tự chọn privacy/interactions và consent rõ trước khi gửi.
- Direct Post Reference cập nhật 2026-08-24 xác nhận endpoint cần creator info,
  metadata và consent trước khi khởi tạo; app cần audit để bỏ giới hạn private.
- **Suy luận cho MiuNova:** app cá nhân tự host có xung đột intended-use và
  public Direct Post chưa thể hứa. Chỉ TikTok có thể quyết định eligibility;
  nếu chưa được chấp thuận, handoff sang TikTok app.
- Nguồn chính thức: [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines),
  [Direct Post API](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post).

### Meta video publishing

- Bộ sưu tập Threads API trong workspace Postman chính thức của Meta mô tả tạo
  video container qua `media_type=VIDEO` và `video_url` công khai, sau đó publish
  bằng container ID. Điều này chứng minh endpoint video được tài liệu hóa;
  không chứng minh MiuNova đã có scope/app review hoặc publish thành công.
- Bộ sưu tập chính thức của Meta cũng mô tả Facebook Page Reels và Instagram
  Reels publishing. Account type, app review, quyền, media hosting và phiên bản
  API vẫn phải được kiểm tra trước khi bật từng adapter.
- Trang canonical developers.facebook.com bị giới hạn fetch (HTTP 429) khi rà
  lại. Vì vậy đối chiếu changelog/docs chính thức và credential thật trước khi
  coi giới hạn/quyền là đã xác minh.
- Nguồn: [Meta Threads API collection](https://www.postman.com/meta/threads/collection/dht3nzz/threads-api),
  [video container request](https://www.postman.com/meta/threads/documentation/dht3nzz/threads-api?entity=request-34203612-b3b2c12a-7ce6-4d86-a3c6-6d31e3b66ea1),
  [Meta Instagram API workspace](https://www.postman.com/meta/instagram/collection/6yqw8pt/instagram-api),
  [Meta Facebook API collection](https://www.postman.com/meta/facebook/documentation/r56bjfd/facebook-api).

### AccessTrade product evidence

- API `/v1/datafeeds` documents price, discount, product, campaign, affiliate
  link and update time; it does not document sales or rating/review fields.
- `/v1/top_products` accepts date filters, but docs define outer `total` as
  “số lượng sản phẩm” and do not define it as units/orders sold per offer or
  state a per-offer sales window. Do not use it as sales evidence.
- Separate TikTok Shop product-feed docs describe `units_sold`, commission,
  inventory and sort by best sellers/high commission. `units_sold` is total units
  per SKU, can be omitted when seller has not allowed precise data sharing, and
  docs do not give a recent-sales period. The user previously removed TikTok
  Shop API from scope; no current decision reopens it. Even if reopened, this
  source does not supply ratings/reviews or prove 30-day sales.
- The current MiuNova adapter uses `/v1/datafeeds`; it does not call
  `/v1/top_products` or TikTok Shop product feeds. No AccessTrade review/rating
  source is verified. The current connected AccessTrade source cannot pass both
  recent-sales and review gates in PRODUCT_SPEC; affiliate production must hold
  until an allowed evidence source is connected.
- Sources: [Datafeeds API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-thong-tin-datafeeds),
  [Top-selling products API](https://developers.accesstrade.vn/api-publisher-vietnamese/top-cac-san-pham-ban-chay-nhat),
  [TikTok Shop product search v2](https://developers.accesstrade.vn/api-publisher-vietnamese/tich-hop-api-publisher-at-cho-chien-dich-tiktok-shop/version-2-updated-version/tiktok-product-search-v2),
  [current source scope](features/products.md),
  [historical explicit exclusion](REVIEW_ROUND2.md).

### Nguồn affiliate thay thế đã rà

- **Shopee Affiliate:** Help Center chính thức nói API có danh sách shop/brand/
  product offers, short link và conversion/validated reports. Trang Open API
  của Shopee Việt Nam là ứng dụng JavaScript; schema trường không đọc được từ
  trang công khai đã kiểm tra. Chưa xác nhận product-level sales theo 30 ngày,
  rating/review, điều kiện publisher hoặc chi phí API. Chỉ là ứng viên cần
  kiểm chứng, chưa đủ làm evidence source.
- **Rentracks Vietnam:** Publisher docs có data feed/coupon feed và API. Hướng
  dẫn 2025 nói field trong datafeed thay đổi theo campaign/nhà cung cấp; API
  được mô tả cho publisher dạng sub-network cần liên hệ team Rentracks. Tài
  liệu công khai chưa chứng minh trường sales theo kỳ hay rating/review và
  quyền truy cập miễn phí cho publisher cá nhân.
- **ADPIA:** tài liệu thương hiệu nói API hỗ trợ tạo trang sản phẩm top-sale,
  nhưng không công bố schema, cửa sổ sales, freshness, review/rating hay điều
  kiện/chi phí truy cập. Không suy ra trang “top-sale” là số bán 30 ngày.
- **Awin:** tài liệu product-feed list có timestamp cập nhật; ví dụ cột feed
  gồm `average_rating`, `reviews` và `number_stars`. Ví dụ hiện trên docs là
  advertiser GB; schema này không chứng minh sales per-product theo kỳ hay độ
  sẵn có tại Việt Nam. Có thể khảo sát cho trường review, chưa thay được cổng
  sales.
- Kết luận ngày 2026-10-22: chưa xác minh được provider thay thế nào đồng thời
  có commission/link hợp lệ, sales 30 ngày theo product, rating/review có mẫu,
  freshness/provenance rõ, quyền API hợp lệ và mức phí bằng 0. O-02 còn mở;
  không bật affiliate production khi chưa đạt evidence gate.
- Nguồn chính thức: [Shopee API Access](https://help.shopee.sg/portal/10/article/191702-API-Access),
  [Shopee Vietnam Open API](https://affiliate.shopee.vn/open_api/list),
  [Rentracks Publisher](https://rentracks.com.vn/publisher/),
  [Rentracks dashboard guide 2025](https://rentracks.com.vn/huong-dan-cach-su-dung-dashboard-rentracks-2025/),
  [ADPIA brand guideline](https://adpia.vn/assets/pdf/branding-adpia-new.pdf),
  [Awin product-feed list](https://help.awin.com/developers/docs/product-feed-list-download).

### AccessTrade publisher identity and payout

- AccessTrade's current publisher FAQ says payout goes to the publisher's
  registered default bank account. Payout requires approved commission of at
  least VND 200,000, bank details, citizen ID, tax ID and completed e-contract
  verification. Publisher identity and payout setup therefore need a real
  account owner; they are not replaced by social OAuth.
- The same FAQ says a bank account may be shared across multiple Publisher
  accounts. It does not describe per-creator commission splitting or delegated
  payout, so this is only a possibility to verify with AccessTrade, not proof
  that an operator account can automatically settle revenue to each user.
- The product-link API describes UTM and `sub1`–`sub4` as tracking parameters.
  It does not document delegated publisher identity, split payout or settlement
  to each creator. **Inference:** these tracking fields cannot be treated as a
  payout-routing mechanism. If MiuNova/operator owns the publisher account,
  separate permission and settlement arrangements would be needed to route
  earnings to users; do not assume they exist.
- Current connected AccessTrade setup therefore conflicts with the literal
  “only login social” requirement unless the user does affiliate onboarding
  once or a permitted managed-payout model is established. Neither path is
  currently decided or implemented.
- Rentracks Vietnam describes an API for publisher-owned sub-networks and says
  the operator must contact its team for integration details. Its dashboard
  guide also asks a publisher to complete identity, channel and bank details
  before campaign registration. This makes a managed sub-network a candidate
  for O-01, but public docs do not establish creator-level delegation, split
  settlement, or whether downstream creators can avoid their own onboarding.
  Do not treat it as a working payout solution without written provider terms.
- Sources: [AccessTrade publisher FAQ](https://help.accesstrade.vn/faq-publisher/),
  [AccessTrade product-link API](https://developers.accesstrade.vn/api-publisher-vietnamese/tao-tracking-link),
  [Rentracks publisher API](https://rentracks.com.vn/publisher/),
  [Rentracks dashboard guide 2025](https://rentracks.com.vn/huong-dan-cach-su-dung-dashboard-rentracks-2025/).

### YouTube Data API quota

- Google quota docs currently show separate default buckets: 100 `videos.insert`
  calls/day and 100 `search.list` calls/day, each 1 unit/call; other methods
  share a 10,000-unit/day bucket. `videos.insert` resets at midnight Pacific.
  Limits are project-specific, visible in Cloud Console, and can change.
- Source: [Quota Calculator](https://developers.google.com/youtube/data/determine_quota_cost),
  [videos.insert reference](https://developers.google.com/youtube/data/reference/rest/v3/videos/insert),
  [Revision History](https://developers.google.com/youtube/data/revision_history).

### OAuth callback trên VPS

- TikTok: code cho phép đặt `TIKTOK_REDIRECT_URI`; callback trực tiếp về VPS cần
  URL HTTPS tĩnh đã đăng ký trong app TikTok. TikTok Web Login Kit yêu cầu
  redirect HTTPS đã đăng ký; PKCE áp dụng cho Desktop, không cho Web. Code hiện
  chỉ gửi PKCE với callback loopback và giữ state cho Web. Xem
  [Login Kit Web](https://developers.tiktok.com/docs/en/login-kit-web/) và
  [Login Kit overview](https://developers.tiktok.com/docs/en/login-kit-overview?enter_method=left_navigation).
- Facebook: code cho phép `META_REDIRECT_URI` kết thúc bằng
  `/publishers/facebook/callback`; cần đăng ký URI tương ứng trong Meta App.
- YouTube: code giữ Desktop/loopback khi `YOUTUBE_REDIRECT_URI` trống; khi có
  URI HTTPS hợp lệ tại `/publishers/youtube/callback`, dùng callback đã cấu
  hình cho Google Web OAuth client. Google Web Server OAuth yêu cầu redirect
  URI đã đăng ký. Xem [Google Web Server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server)
  và [Google OAuth for installed apps](https://developers.google.com/identity/protocols/oauth2/native-app).
- Kết luận: code có callback VPS cấu hình cho TikTok, Facebook và YouTube.
  Chưa xác minh flow trên VPS với credentials thật. Web AI remote login đã có
  screenshot/input bridge trong source; VPS/provider acceptance vẫn chưa chạy.

## Historical archive — 2026-10-02

> Các mục dưới đây phản ánh quyết định/pivot và research trước PRODUCT_SPEC hiện
> tại; giữ để tra cứu thời điểm, không phải hướng sản phẩm đang chạy.

Mức tin cậy trong archive: **[chính thức]** = nền tảng/tài liệu gốc công bố ·
**[đo thật]** = có benchmark/số đo trên phần cứng nêu rõ · **[nguồn thứ cấp]**
= blog/vendor, dùng thận trọng.

---

## 1. Chính sách live TikTok & giọng AI (đọc ngày 2026-10-01)

1. TikTok Shop **cấm giọng AI/audio thu sẵn trong live bán hàng**
   (promotional livestream/live commerce) — hiệu lực 5/2026, siết qua Account
   Health Rating từ 7/2026. Live giải trí thuần túy (không bật tính năng Shop)
   **không thấy văn bản cấm**; giọng AI trong live giải trí chỉ cần gắn nhãn
   AI như nội dung realistic. **[nguồn thứ cấp nhất quán: pymnts, ppc.land,
   techtimes]**
2. Live bán hàng: hình tĩnh/loop che >50% màn hình và figure không-phải-người
   che >50% màn hình đều vi phạm; avatar digital được phép nhưng <50% màn
   hình. Hễ ghim giỏ hàng/bật Shop trong live là vào diện cấm trên.
   **[nguồn thứ cấp]**
3. Không có API chính thức nào đọc chat/gift realtime của LIVE TikTok.
   Chỉ có client reverse-engineer không chính thức (TikTokLive, EulerStream).
   → Kiến trúc của ta **không** phụ thuộc chúng (luật: chỉ API chính thức).
   **[chính thức về sự vắng mặt + nhiều repo độc lập]**
4. Không có API chính thức để start/stop LIVE hay lấy RTMP key tự động; key
   lấy tay qua LIVE Center/app, FFmpeg đẩy RTMP là đường tuân thủ.
   **[chính thức]**

Hệ quả kiến trúc (đã chốt trong MODEL.md): live giải trí = gift, không bán
hàng trong live; affiliate đi qua video ngắn + showcase.

## 2. Việc được phép cho từng vertical (TikTok × YouTube, 10/2026)

Kết luận: **không vertical nào bị cấm ở mức nền tảng** (với điều kiện gắn nhãn
AI đầy đủ, không dùng tính năng Shop trong live). Thứ tự khuyến nghị triển
khai: kể chuyện → game → hát → nhảy → PK (khó automate nhất, affiliate thấp).

| Vertical | TikTok | YouTube |
|---|---|---|
| Hát nhạc AI tự sáng tác | Cho phép + nhãn AI | Cho phép + YPP nếu có nguyên bản + disclosure |
| Nhảy avatar AI | Cho phép + nhãn AI | Cho phép |
| Kể chuyện AI đọc | Cho phép + nhãn AI | Cho phép + YPP được (truyện tự sáng tác) |
| PK với host người | Cho phép, rủi ro trung bình | N/A |
| AI tự chơi game | Cho phép | Cho phép — gameplay được miễn disclosure |

Rủi ro thật không nằm ở nền tảng mà ở **ToS của từng game** (bot/anti-cheat)
— danh sách game an toàn xem MODEL.md §8b. YouTube cấm monetize: nội dung
template hàng loạt na ná nhau; **AI avatar đóng vai chuyên gia y tế/tài
chính/pháp lý**. **[chính thức + nguồn thứ cấp, 10/2026]**

## 3. Cách kiếm tiền TikTok ngoài bán hàng (10/2026)

1. **Quà tặng LIVE là trụ cột #1 cho live giải trí ở VN:** mở ở VN, không phân
   biệt AI/người, miễn live tuân thủ guidelines. TikTok giữ ~50% giá trị gift.
   Live cần 1.000 follower. **[đo độc lập/nguồn thứ cấp nhất quán]**
2. **Creator Rewards KHÔNG xác minh được ở VN** (danh sách nước công bố không
   có VN) → không đặt cược; chỉ tin khi thấy trong TikTok Studio thật.
   **[chính thức — danh sách công bố]**
3. SoundOn **chấp nhận nhạc AI tự sáng tác**, giữ 100% royalty trên nền tảng
   ByteDance; nhạc phải qua quét đạo nhạc ACRCloud trước khi phát hành. Nhưng
   distributor khác (LANDR) không cho nhạc AI đủ điều kiện Content ID →
   phát hành được ≠ hưởng đủ quyền Content ID. **[chính thức SoundOn]**
4. TikTok Series dùng được cho nội dung gốc ($0.99–189.99/series, điều kiện
   10K follower hoặc chứng minh đã bán premium); LIVE Subscription cần 10K
   follower + 100K view/tháng. Tình trạng mở ở VN **chưa verify**.
   **[chính thức + chưa verify VN]**

Các mục này là căn cứ cho thứ tự ưu tiên trong MODEL.md §4.

## 4. API TikTok Shop (đường chính thức — Ninh đã từ chối dùng, 2026-10-01)

Kết luận còn dùng: **toàn bộ API là miễn phí**; Affiliate Creator API có sẵn
endpoint săn sản phẩm open-collaboration (lọc theo hoa hồng) và đối soát đơn
hoa hồng; Content Posting API có 2 mode — Direct Post (đăng thẳng) và Upload
to Inbox (draft). Nhưng Direct Post phải qua **app review (~1–2 tuần) + Direct
Post audit (~5–10 ngày làm việc)**; chưa audit chỉ đăng được `SELF_ONLY` tối
đa 5 user. Giới hạn: ~15 bài/ngày/tài khoản qua Direct Post, 6 request/phút
theo token. **[chính thức + nguồn thứ cấp, 10/2026]**

Vì sao Ninh loại: Partner Center + app review + setup doanh nghiệp "rất phức
tạp". Đường thay thế đã chọn trong app: ADB quét Product Marketplace trên máy
Android của Ninh; đăng TikTok dạng draft. Mục này giữ lại chỉ để sau này khỏi
nghiên cứu lại: nếu đổi ý, chi phí thật là thời gian duyệt, không phải tiền.

## 5. TTS tiếng Việt

App ưu tiên giọng đọc macOS nội bộ, sau đó Edge và Gemini. Vì vậy giọng nói mặc định không tiêu thụ quota API.

## 6. Avatar realtime — kết luận cuối cùng (2 vòng research 2026-10-02)

**Verdict: KHÔNG có cách nào đạt avatar photoreal ≥20fps realtime chạy local
trên MacBook Pro M1 Pro 32GB.** Mọi ông lớn (HeyGen, D-ID, Tavus) đều render
bằng GPU server và phát WebRTC về máy khách — chất lượng của họ không phải
bằng chứng model chạy được trên laptop. Đây là lý do Ninh hạ ưu tiên live
avatar; kết luận này chỉ xem lại khi phần cứng/model đổi hẳn.

Số đo chính (gắn nhãn chip thật đã đo — không thay chip):

| Đo trên | Model | Kết quả | Mức |
|---|---|---|---|
| M1 Pro 32GB | LivePortrait | ~1 fps | [đo thật] |
| M2 Pro | MuseTalk 1.5 (PyTorch MPS) | ~1,6 fps | [đo thật] |
| M5 Max | MuseTalk-MLX | 22,4 fps @256px; 29,8 fps @128px | [đo thật, không phải M1 Pro] |
| M1 Pro (suy ra từ M5 Max) | MuseTalk-MLX | ước ~5–9 fps @256px, ~11–15 fps @128px; cộng thêm độ trễ windowing ~5,1s | [ước tính — chưa đo] |

Sự thật khác đã kiểm chứng:

1. Hội thoại Gemini Live API là **realtime giọng nói có tiếng Việt**, nhưng
   API công khai chỉ xuất audio — không xuất video avatar. "Live Avatar" của
   Google là Enterprise/allowlist, chưa có API công khai. Veo là async (vài
   phút/clip). **[chính thức, đọc 2026-10-02]**
2. Cloud photoreal rẻ nhất nếu sau này cần: Simli ~$0,009–0,01/phút
   (third-party, chưa đối chứng trang giá) và LiveAvatar LITE ~$0,09/phút
   (ước từ gói cước). Free quota đều ở mức demo (10 credits–50 phút/tháng).
   **[nguồn thứ cấp, đọc 2026-10-02]**
3. Não + tai + giọng realtime có thể $0: Groq free (Whisper nghe tiếng Việt;
   TTS của nó chỉ Anh/Ả Rập), NVIDIA NIM free có TTS Magpie tiếng Việt —
   nhưng credits NIM hữu hạn và trạng thái hosted Audio2Face còn tranh cãi.
   **[đo thật bởi bên thứ ba, 2026-09-22]**
4. **Kiến trúc local khả thi duy nhất nếu bắt buộc làm live local:** kiểu
   NVIDIA ACE — audio → blendshape ARKit-52 (model ONNX ~76MB) → render
   mesh 3D (VRM/ReadyPlayerMe/WebGL) trên M1. ≥25fps là khả thi về mặt
   tính toán, nhưng avatar **stylized, không photoreal**. Nỗ lực ước 2–4
   tuần. Chưa có benchmark M1 Pro nào cho route này. **[phân tích kỹ thuật]**
5. Default local hiện tại của app là **MuseTalk v1.5** (port
   `dunso/musetalk-mac`) chạy sidecar, render **offline** — chỉ sync môi,
   không tự sinh chuyển động đầu theo audio. Quality bar của Ninh (mọi frame
   nhân vật cử động như người thật, cấm slideshow) là tiêu chuẩn chấm, xem
   ARCHITECTURE.md §5.

---

*Cập nhật file này khi có kết quả benchmark M1 Pro thật hoặc khi chính sách
nền tảng đổi. Không chép số không nguồn/chưa gắn nhãn mức tin cậy vào đây.*
