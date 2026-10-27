# Product spec — TikTok affiliate và TikTok viral

Cập nhật: 2026-10-25. Risk: P1 — sản phẩm, hoa hồng, nội dung công khai và số liệu ảnh hưởng quyết định kinh doanh.

## Phạm vi

Sản phẩm tập trung hai flow đang hoàn thiện:

1. TikTok affiliate: tìm sản phẩm → đánh giá → duyệt/loại → tạo video bằng nhân vật kênh → xem trước, xác nhận và đăng video có product anchor khi API đủ quyền → đo đơn hàng → cải tiến.
2. TikTok viral: tìm video theo chủ đề → phân tích → tạo phiên bản sáng tạo bằng nhân vật kênh → xem trước, xác nhận và đăng hoặc gửi Draft theo quyền tài khoản → đo hiệu quả → cải tiến.

Truyện audio để phase sau.

## Nguyên tắc dữ liệu và tự động hóa

- Mỗi dữ kiện ngoài hệ thống phải lưu nguồn, ID/URL, thời điểm lấy và kỳ đo.
- Không có dữ liệu nghĩa là `unknown`, không phải 0, không phải “không phù hợp”.
- Phân biệt dữ kiện provider trả về, kết quả tính toán và nhận định AI. Nhận định phải dẫn dữ kiện đầu vào.
- Không kết luận thị trường, người trẻ, độ viral, doanh số hoặc nguyên nhân nhân quả nếu dữ liệu không hỗ trợ.
- Giữ lịch sử phân tích, quyết định, cấu hình và version. Cho xem lại, chạy lại, hoàn tác.
- Tự động duyệt sản phẩm theo rule có thể chỉnh. Trước mỗi lần gửi nội dung TikTok, người vận hành phải xem preview, chọn account/privacy/disclosure và xác nhận rõ thao tác publish; sau consent, hệ thống có thể tự gửi và reconcile trạng thái qua API đã được cấp quyền.
- Không mặc định Direct Post đã được duyệt hoặc dùng được cho app nội bộ. Khi API/audit/scope chưa đủ, dùng TikTok Draft và yêu cầu người vận hành hoàn tất đăng trong TikTok.
- Không tải lại hoặc đăng nguyên video tham chiếu. Phân tích cấu trúc, rồi tạo nội dung mới cho nhân vật kênh. Âm thanh cho cả hai flow lấy từ video nhảy viral TikTok Radar đã thu thập.

## Flow affiliate TikTok

### 1. Radar sản phẩm

- Người vận hành cấu hình category, merchant nếu cần, commission tối thiểu, giá tối đa, lịch chạy và độ mới dữ liệu.
- Radar lấy offer từ nguồn affiliate đã kết nối, lọc chiến dịch có thể tạo tracking link, chuẩn hóa currency/commission/category và khử trùng theo provider + product ID.
- Lưu mọi candidate tìm thấy, kể cả candidate chưa đủ dữ liệu hoặc bị loại. Mỗi lần quét tạo snapshot; refresh không ghi đè lịch sử.
- Candidate có trạng thái: `new`, `analyzing`, `approved`, `rejected`, `needs_review`, `stale`, `archived`.
- Danh sách riêng: Đang đánh giá, Đã duyệt, Bị loại. Danh sách bị loại giữ lý do, evidence và nút đánh giá lại.

### 2. Phân tích, đánh giá và duyệt

Phân tích theo từng nhóm, không gộp thành một điểm mờ:

| Nhóm | Dữ kiện cần có | Quy tắc |
|---|---|---|
| Offer | Giá hiện tại/gốc, currency, commission rate/value, campaign, affiliate link, thời điểm cập nhật | Thiếu giá hoặc link hợp lệ → `needs_review`; commission thấp hơn rule → loại |
| Nhu cầu | Lượt bán và tốc độ bán theo kỳ, rating, số review, thời điểm và nguồn đo | Chỉ đánh giá khi provider trả đủ nguồn/kỳ đo; thiếu thì `unknown` |
| Thị trường | Category, xu hướng TikTok theo chủ đề, vùng, views/velocity của nguồn, độ mới | Không dùng caption/số video đơn lẻ làm bằng chứng nhu cầu mua |
| Tệp kênh | Persona, category kênh, lịch sử video/đơn đã đo | “Phù hợp giới trẻ” chỉ kết luận khi có dữ liệu audience phù hợp; nếu không ghi chưa đủ dữ liệu |
| Rủi ro | Claim sản phẩm, mức giá, policy/category, chất lượng nguồn | Claim không chứng minh được hoặc nguồn lỗi thời → chặn duyệt tự động |

Đầu ra gồm: quyết định, điểm từng nhóm, confidence, bằng chứng, dữ liệu còn thiếu, lý do và version rule. Rule cứng chặn sản phẩm không đủ điều kiện. Rule mềm xếp hạng theo hoa hồng kỳ vọng, nhu cầu đã đo, hợp tệp kênh và độ mới.

Auto-approve chỉ khi tất cả hard gate qua, evidence đủ mới, confidence đạt ngưỡng cấu hình và không có policy blocker. Ngưỡng ban đầu phải cấu hình được; không hard-code một con số chưa được vận hành xác nhận. Mọi trường hợp thiếu sales/rating hoặc tệp audience cần người xem lại. Reject lưu vào danh sách bị loại; người dùng có thể mở evidence, sửa rule hoặc yêu cầu đánh giá lại.

### 3. Xu hướng, mức mua và tiềm năng viral

- Tách “đang thịnh hành”, “có tín hiệu mua”, “phù hợp kênh” và “tiềm năng viral” thành các nhận định độc lập.
- Tiềm năng mua dựa trên sales/conversion/order data theo kỳ, giá, commission value, click/order funnel và độ mới.
- Tiềm năng viral dựa trên metrics video tương tự trong TikTok Radar và lịch sử video của kênh. Đây là ước lượng, không phải dự báo chắc chắn.
- Phù hợp giới trẻ dựa trên audience analytics có quyền truy cập hoặc dữ liệu vận hành được cung cấp; không suy ra tuổi từ view count.
- Báo rõ nguồn và confidence cạnh từng nhận định. Không đủ evidence → “Chưa đủ dữ liệu”.

### 4. Sản xuất chiến dịch

- Chỉ sản xuất từ candidate đã duyệt.
- Gắn sản phẩm, offer snapshot, tracking link, tài khoản và nhân vật kênh vào campaign/job.
- Tạo concept, kịch bản, shot list và CTA từ claim được nguồn hỗ trợ; không bịa công dụng, giá, review hoặc kết quả.
- Người vận hành chọn video nhảy viral trong TikTok Radar. Studio tách audio từ video đó; lưu video ID/URL, author, collected-at và audio metadata.
- Tạo video 9:16 với nhân vật kênh, đúng nhận diện sản phẩm; chạy QC hình, audio, claim, file và link.
- Thiếu sản phẩm/ảnh/claim/audio hợp lệ hoặc QC fail → dừng job, ghi rõ lý do.

### 5. Đăng và đo chuyển đổi

- Màn hình review trước đăng hiển thị video preview, đúng TikTok account/nickname, caption/hashtag sửa được, privacy do người dùng chọn (không đặt mặc định), quyền comment/duet/stitch theo creator info, disclosure nội dung thương mại, disclosure AI khi áp dụng và consent phù hợp. Không gửi media trước consent.
- Với video affiliate TikTok Shop, dùng Shoppable Video API đã được TikTok cấp quyền để precheck, publish video và bind TikTok Shop `product_id` + anchor title. Candidate phải lưu riêng AccessTrade offer ID và TikTok Shop product ID; chỉ bind sau khi xác minh hai ID trỏ tới cùng sản phẩm và sản phẩm đủ điều kiện. Gửi `video_info.is_ai_generated=true` cho video do AI tạo. `product anchor` là link mua trong TikTok; AccessTrade tracking URL chỉ dùng cho attribution và không được giả thành product card.
- Với video viral hoặc khi Shop API không khả dụng, dùng Direct Post API chỉ khi app được duyệt, tài khoản/scope đủ điều kiện và cách dùng được TikTok cho phép. Nếu không, gửi Draft qua Upload API; người vận hành hoàn tất bước còn thiếu trong TikTok.
- Không đóng dấu watermark, logo quảng bá, URL hoặc promotional text lên nội dung video. Caption/hashtag có sẵn phải sửa được trước khi gửi.
- Theo dõi status endpoint/webhook cho tới khi có kết quả: `draft_ready`, `precheck_pending`, `publishing`, `public`, `failed` hoặc `reconcile_required`. Chỉ hiện `Đã đăng công khai` khi provider trả post ID hoặc public status; timeout không rõ kết quả phải reconcile trước retry.
- Với Direct Post API chưa qua audit, chặn lựa chọn public; TikTok giới hạn trạng thái `SELF_ONLY` và quota người đăng. Vì MiuNova hiện là app nội bộ cho các account do nhóm quản lý, khả năng dùng Direct Post công khai phải được TikTok xác nhận trước khi bật.
- Nếu video cần nhãn AI-generated mà publishing adapter không hỗ trợ gửi nhãn đó, chặn public submit và chuyển sang Draft/manual handoff để người dùng bật nhãn trong TikTok.
- Lưu campaign ID, product ID, TikTok Shop anchor state, AccessTrade tracking link, TikTok video ID, publish state, consent evidence và timestamps để nối dữ liệu.
- Đồng bộ clicks, orders, canceled/returned orders, commission và settlement theo khả năng provider. Phân biệt pending/approved/rejected; không cộng trùng khi sync lại.
- Attribution phải hiển thị provider, cửa sổ attribution và giới hạn ghép video↔đơn. Nếu không ghép được chính xác, báo ở cấp campaign/account, không quy công cho một video.

## Flow TikTok viral

### 1. Radar theo chủ đề

- Nguồn hiện tại là Web Crawler cục bộ. Hành vi thu thập, fallback, timeout và unknown-field contract chỉ định tại [tài liệu TikTok Web Crawler](features/tiktok-web-crawler.md).
- Người dùng chọn một topic đã lưu hoặc thêm topic mới.
- Radar lưu video ID/URL, author, caption, sound metadata, views/likes/comments/shares, thời điểm thu thập và provider.
- Danh sách xếp theo lượt xem đã biết; lượt thích phá hòa. Đây là xếp hạng phổ biến, không phải tốc độ tăng view; tốc độ chỉ dùng trong phân tích khi có đủ snapshot.
- Video mới ở trạng thái `Chưa xác nhận vũ đạo`; chỉ người vận hành đã xem mới xác nhận. Video chưa xác nhận không được chọn làm nguồn audio. Không có phân loại dance tự động trong phạm vi sản phẩm hiện tại.

### 2. Phân tích video

Phân tích hook, mở đầu, pacing, cảnh/shot, text, CTA, sound, độ dài và nhịp tương tác. Mỗi kết luận liên kết tới frame/timecode hoặc metric đã thu thập; confidence thấp phải hiện là giả thuyết.

### 3. Tạo phiên bản mới

- Lấy cấu trúc tham khảo; viết concept/kịch bản mới theo persona, nhân vật, sản phẩm/chủ đề kênh.
- Không sao chép nguyên video, lời thoại, watermark hay nhận diện creator khác.
- Với nhạc, người dùng chọn video nhảy trong Radar. Tách audio từ đúng video đã chọn; không fallback sang sound catalog hoặc video khác.
- Lưu source video/audio provenance và kiểm tra audio track ở file cuối.
- QC trước khi gửi TikTok Draft.

### 4. Đo viral và cải tiến

- Đồng bộ views, likes, comments, shares, saves, average watch time/completion, profile visits/follows khi API cho phép.
- Snapshot theo mốc 1h/24h/72h/7d sau publish; lưu account ID, TikTok video ID, provider, timestamp và kỳ đo.
- So sánh với median/baseline của chính kênh, cùng format/topic/độ dài; hiển thị cỡ mẫu.
- Thiếu retention/follower metrics thì không suy đoán các chỉ số đó. Phân biệt tương quan với nguyên nhân.

## Theo dõi và vòng cải tiến chung

### Affiliate

Phân tích funnel `impression/view → click → product detail → order → approved commission` khi từng metric có nguồn. Chẩn đoán theo bằng chứng:

- View thấp + retention thấp: đề xuất thử hook/nhịp mở đầu.
- View cao + click thấp: xem CTA, mức liên quan sản phẩm và vị trí link.
- Click cao + order thấp: kiểm tra giá, phí/vận chuyển, offer, stock, sales data và audience fit.
- Order cao nhưng commission thấp: xếp lại sản phẩm theo commission thực nhận và điều kiện attribution.

Nếu không có một mắt xích dữ liệu, nêu rõ khoảng trống và không đoán nguyên nhân. So sánh cohort cùng account/topic/format và kỳ đo.

### Viral

Đối chiếu view velocity, retention/completion, shares/saves, profile visits và follows với baseline kênh. Dùng kết quả để đề xuất thay đổi hook, độ dài, pacing, topic, CTA hoặc sound source. Mỗi lần chỉ thay một nhóm biến khi có thể để đánh giá được tác động.

### Tự chỉnh cấu hình

- Hệ thống tạo recommendation có evidence, confidence, tác động dự kiến, thay đổi cụ thể và version rule.
- Auto-apply chỉ cho thay đổi đảo ngược được và trong bounds cấu hình: category weights, commission floor, freshness window, format/hook mix.
- Cần người duyệt cho đổi tài khoản, offer đã duyệt, nội dung/claim, publish mode, quyền provider hoặc giới hạn chi phí.
- Ghi audit trước/sau; cho xem diff, rollback và tắt tự động.
- Không đổi radar hoặc generator chỉ vì một video/campaign. Cần minimum sample size và complete measurement window theo policy.

## UI và công nghệ

- Go `html/template` render trang và partial phía server.
- HTMX xử lý lọc, refresh radar, đánh giá, duyệt/loại, tải trạng thái job, metrics và rollback mà không cần SPA.
- Tailwind CSS được pin version và compile thành static CSS khi build; không phụ thuộc CDN/runtime compiler ở production.
- HTMX requests trả partial phù hợp; full navigation vẫn dùng URL ổn định và form POST redirect/flash khi phù hợp.
- Màn hình cốt lõi: Affiliate Radar (category/filter/list), Candidate detail (evidence/decision), Rejected list, Affiliate campaign/job, Viral Radar/topic, Viral analysis/job, TikTok performance/insights.
- Mỗi thao tác có busy/error/success state; dữ liệu `unknown`, stale, pending và provider error phải phân biệt.

## Nguồn dữ liệu và giới hạn hiện tại

| Nhu cầu | Hiện có trong repo | Chưa chứng minh |
|---|---|---|
| Offer, campaign, commission, affiliate link | AccessTrade clients/cache | Sales và rating/review theo từng sản phẩm |
| Orders/commission | AccessTrade order sync hiện có | Ghép attribution tới đúng TikTok video, canceled/returned và settlement đầy đủ |
| Video Radar | Web Crawler cục bộ, SQLite, conditional metrics | Quyền sử dụng production, coverage/freshness thật; chi tiết ở [crawler contract](features/tiktok-web-crawler.md) |
| TikTok publish | Draft upload adapter hiện có | Direct Post API có public mode sau audit và creator consent; app nội bộ có thể không đạt intended-use. TikTok Shop Shoppable Video API có precheck/publish/product anchor nhưng cần partner + creator authorization. Repo chưa xác minh scope, audit, anchor hoặc public publish thật. |
| TikTok performance | Display API source cho profile/public-video counters + SQLite snapshots | Retention, watch time, demographics, profile visits/follows và video-level order attribution |
| UI interactions | Go templates + vendored HTMX partials + pinned Tailwind compiled to local static CSS | Xác minh OAuth/provider thật và tiếp tục mở rộng partials cho các control còn lại |

Không được thay giới hạn ở bảng này bằng số liệu giả hoặc nhận định AI không dẫn nguồn.

## Acceptance criteria

1. Radar affiliate lọc theo category/commission, lưu candidate và snapshot, khử trùng theo provider ID.
2. Mỗi candidate hiển thị nguồn/updated-at; thiếu sales/rating hiện `Chưa có dữ liệu`, không hiện 0.
3. Evaluation cho verdict + evidence + confidence + lý do; reject xuất hiện trong danh sách xem lại; đánh giá lại giữ lịch sử.
4. Chỉ candidate approved mới tạo campaign. Campaign gắn đúng product, account, tracking link và video nhảy đã chọn trong Radar.
5. Affiliate video vượt QC claim/audio/file; preview/consent bắt buộc trước submit; UI phân biệt draft, Shop product anchor, public post và metrics. Product anchor phải khớp offer đã duyệt; public state chỉ có provider evidence.
6. Viral Radar lọc topic; mỗi analysis dẫn source/timecode/metric; không tải lại nguyên video tham chiếu.
7. Cả hai flow giữ source audio provenance và fail closed khi source thiếu audio hoặc ngoài Radar.
8. Metrics snapshots có source/time/video/account/window; unknown không thành zero; sync lặp không nhân đôi đơn.
9. Insight nêu evidence/sample size/unknowns và chỉ auto-apply bounded, reversible settings. Có audit diff và rollback.
10. HTMX controls trả đúng partial/state; Tailwind CSS load từ local compiled asset; không cần runtime CDN.
11. Publish review hiển thị đúng account, preview, caption sửa được, privacy/disclosure/AI label/consent và lỗi/quota; không gửi trước consent. Nếu API không đủ quyền hoặc không gửi được nhãn bắt buộc, app đưa Draft/manual handoff. Shoppable post chỉ hoàn tất khi TikTok xác nhận product anchor khớp offer đã duyệt.

## UI handoff oracle

| Scenario | Control/screen | Oracle |
|---|---|---|
| Radar không có sales/rating evidence | Candidate detail | `Chưa có dữ liệu` hiển thị; auto-approval bị chặn |
| Candidate bị loại | Candidate detail → Bị loại | Candidate xuất hiện trong danh sách Bị loại cùng reason/evidence |
| Video nhảy chưa được xác nhận | Viral/Affiliate Radar | Trạng thái `Chưa xác nhận vũ đạo`; không tự chọn source |
| Draft được TikTok nhận | Campaign/job detail | Provider state + TikTok draft ID; không hiển thị `Đã đăng` |
| Đăng video affiliate có product anchor | Publish review → Campaign/job detail | Consent timestamp + provider publish ID + TikTok video ID + `anchor_state=attached`; thiếu một tín hiệu thì không hiện `Đã đăng kèm sản phẩm` |
| Offer và Shop product ID không khớp | Publish review | Publish bị chặn; UI nêu mismatch và yêu cầu sửa mapping hoặc dùng manual handoff |
| Direct Post chưa qua audit | Publish review | Privacy chỉ cho giá trị provider cho phép; public bị chặn; giải thích rõ Draft/manual handoff |
| Publish timeout sau submit | Campaign/job detail | `Đang đối chiếu`; không tạo lần publish mới cho tới khi status API/webhook xác định kết quả |
| Metrics thiếu retention | Insights | retention hiển thị `Chưa có dữ liệu`; không có kết luận watch-time |
| Recommendation được áp dụng | Insights/settings history | Hiện diff, version và nút `Hoàn tác`; version trước phục hồi sau rollback |

## Phase

1. Làm đúng hai flow TikTok và data truth/publish/metrics.
2. Sau khi acceptance trên đạt, thiết kế lại truyện audio.

## Tài liệu nền tảng đăng TikTok

- [TikTok Content Posting API](https://developers.tiktok.com/products/content-posting-api/) hỗ trợ Direct Post và gửi nội dung vào Draft.
- [TikTok Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines) quy định audit, intended use, preview, metadata/privacy/disclosure, consent, watermark và theo dõi trạng thái.
- [TikTok Get Post Status](https://developers.tiktok.com/docs/en/content-posting-api-reference-get-video-status) định nghĩa poll/webhook và các trạng thái upload/publish.
- [TikTok Shop Shoppable Video Integration](https://partner.tiktokshop.com/docv2/page/ng0l9170) mô tả luồng precheck, publish và bind product anchor; quyền partner/creator cần được cấp riêng.
- [TikTok AI-generated content requirements](https://support.tiktok.com/en/using-tiktok/creating-videos/ai-generated-content) yêu cầu gắn nhãn nội dung AI thực tế/được chỉnh sửa đáng kể; video Shop có trường `is_ai_generated` riêng.
