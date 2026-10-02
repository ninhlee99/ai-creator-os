> ⚠️ **Tài liệu pre-pivot (trước 2026-10-02):** mô tả app khi còn live + phim điện ảnh. Sau pivot (Ninh chốt 2026-10-02) app chỉ còn 3 trụ: Affiliate Accesstrade, Reup Douyin, YouTube kể chuyện. Tài liệu này giữ làm lịch sử, sẽ viết lại theo đợt.
>

# CHANNEL_GROWTH.md — Thiết kế: Xây dựng & Phát triển kênh (zero-touch)

> Thiết kế chốt hướng ngày 2026-10-02 theo yêu cầu của Ninh: giao cho app
> **nhiều** tài khoản TikTok + YouTube (con số "2+2" chỉ là ví dụ — thiết kế
> không hardcode số lượng, scale theo N account/nền tảng) và app tự phát
> triển kênh: tăng follower/subscriber, tăng view.
>
> Khi mâu thuẫn: `docs/MODEL.md` thắng về mô hình kinh doanh,
> `docs/ARCHITECTURE.md` thắng về kỹ thuật hiện tại, `docs/AGENT_TEAM.md`
> thắng về tổ chức đội agent. File này thắng về tính năng growth.
>
> **Trạng thái:** spec sống — MVP + giai đoạn 2 đã triển khai (xem §10–§11,
> kèm giới hạn trung thực còn lại ở cuối §11). Phần triển khai đã bị
> `docs/PROJECT_REVIEW.md` Đợt 1–3 phủ quyết/điều chỉnh (gộp trang, đổi
> mặc định zero-touch, cắt dữ liệu giả) thì làm theo PROJECT_REVIEW.

---

## 0. Nguyên tắc số 1: ZERO-TOUCH (không thương lượng)

**Ninh không can thiệp vào bất kỳ quyết định vận hành nào.** Việc của Ninh
chỉ có:

1. Giữ máy Mac bật 24/7, có điện + mạng.
2. Mở app.
3. **Bootstrap một lần** (xem §6.2): tạo tài khoản, xác minh, giao
   account/key một lần.
4. **Kill switch** khi khẩn cấp — nút duy nhất Ninh bấm trong vận hành.

Mọi thứ còn lại **hệ thống tự quyết**: tự lập kế hoạch nội dung, tự sản xuất,
tự đăng trong giới hạn API chính thức + chính sách nền tảng, tự đọc số liệu,
tự lập lại kế hoạch (kill format yếu, nhân đôi format thắng), tự tạm dừng
account khi có tín hiệu bị phạt.

Hệ quả thiết kế (thay đổi so với AGENT_TEAM.md §5):

- **Không còn cổng "Ninh duyệt lần đăng đầu tiên của mỗi persona".** Thay
  bằng **canary tự động** (xem §5.3): account/persona mới chạy nhịp thấp
  7 ngày đầu, QC 3 lớp + Governance chốt, số liệu bất thường → tự giảm
  nhịp/tự pause. Người không duyệt; luật duyệt.
- Alert gửi Telegram là **notify-only**: báo để biết, không cần Ninh bấm gì.
  Kèm theo alert luôn là "hệ thống đã tự làm gì".
- Mọi ngưỡng kill/scale là **tham số trong Governance config** (kiểu
  `KillViewsNoOrder` hiện có), không phải quyết định hỏi người.

## 1. Cái đã có sẵn trong repo (growth module MỞ RỘNG, không xây lại)

| Thành phần | Hiện trạng | Growth dùng lại |
|---|---|---|
| Vòng đời account | `onboarding → researching → persona_assigned → growing → live_ready → live` (+ `paused/penalized/retired`), `internal/network/account.go` | Giữ nguyên; growth thêm `growth_stage` chi tiết hơn *bên trong* các trạng thái này (§4.1) |
| Persona | 6 persona có `ContentPillars`, `Keywords`, `RedLines`, `AffiliateNiches` (`internal/network/persona.go`) | Trụ nội dung của persona = nguồn format cho content plan |
| Account model | `accounts` có sẵn `followers`, `youtube_channel`, `youtube_content_types`, `theme`, `autopilot`, `min_commission` | Growth profile bám vào đây, không tạo registry song song |
| Hunter | Săn sản phẩm/ngách theo account (`internal/agents/hunter`) | Mở rộng: săn **format/trend nội dung** theo niche, không chỉ sản phẩm |
| Director/Producer | `internal/studio` (director, mediagen, assemble) — phần hoàn thiện nhất | Sinh biến thể theo nền tảng từ cùng một ý tưởng (§5.2) |
| QC | Deterministic checks + review storyboard; chưa thành agent chấm mù độc lập (AGENT_TEAM §8) | Growth kế thừa lộ trình QC agent; metric-based QC thêm ở Analyst |
| Publisher | `internal/publishers` (tiktok.go draft-first theo OAuth từng account; youtube.go resumable upload qua Data API v3 đã có code) | Đăng theo plan; TikTok full-auto mở khóa sau audit (§6.3) |
| Analyst | Đối soát tiền + luật kill/scale sản phẩm qua Governance (`internal/agents/analyst`) | Mở rộng sang **chỉ số tăng trưởng** (follower/view/completion) + kill/double-down format |
| Scheduler | Giờ vàng VN, tối đa 2 live, nghỉ tuần (`internal/network/scheduler.go`) | Thêm lịch đăng video vào cùng khung giờ vàng |
| Ledger | SQLite WAL: `products, content_items, live_sessions, orders, commissions, decisions, api_usage` | Thêm bảng growth (§8), vẫn append-only với số liệu |
| Dashboard | Trang Analytics hiện chỉ có tiền (doanh thu/quà/chi phí API); Schedule chỉ có live slots | Thêm trang `/growth` + khối growth trong account detail (§7) |
| Governance | Hàm thuần, budget caps, kill thresholds, audit trail | Mọi luật growth chạy qua đây — LLM đề xuất, code quyết |

Nói ngắn: vòng đời account trong MODEL.md đã có sẵn giai đoạn `growing`
("Content factory tự đăng video ngắn đến khi đủ 1.000 follower") — module
này **làm cho giai đoạn đó có não**: có kế hoạch, có đo, có tự sửa kế hoạch,
và kéo dài vòng growth qua cả mốc LIVE/YPP thay vì dừng ở 1.000 follower.

## 2. Cơ chế tăng trưởng 2026 (nghiên cứu có nguồn, gắn nhãn độ tin cậy)

Quy ước nhãn: **[chính thức]** = nền tảng công bố · **[đo độc lập]** = bên
thứ ba đo trên dữ liệu lớn · **[vendor claim]** = nhà cung cấp/blog ngành
tuyên bố, chưa có số đo độc lập · **[ước tính]** = suy luận/kinh nghiệm cộng
đồng, biên độ lớn.

### 2.1 TikTok — FYP chấm từng video, không chấm account

1. **Watch time + completion rate là tín hiệu số 1**, trên likes rất xa;
   shares và saves nặng hơn likes/comments. TikTok tự mô tả hệ thống xếp hạng
   theo tương tác người dùng (xem hết, chia sẻ, lưu) — [chính thức về nguyên
   lý; thứ hạng chi tiết là vendor claim].
   Nguồn: https://blog.hootsuite.com/tiktok-algorithm/ ·
   https://www.socialpilot.co/blog/tiktok-algorithm ·
   https://www.digitalmarketingagency.sg/blog/tiktok-algorithm
2. **Mô hình test-pool theo đợt sóng:** video mới được đưa cho ~200–500
   người xem đầu; qua ngưỡng tương tác mới mở rộng lên các đợt lớn hơn
   (5.000–10.000+). Hệ quả: account 0 follower và account lớn "thi cùng đề"
   từng video — cold start không phải án phạt, là kỳ thi đầu vào.
   [vendor claim, được cộng đồng kiểm chứng rộng rãi]
   Nguồn: https://appscrip.com/blog/tiktok-algorithm-explained/ ·
   https://www.lilachbullock.com/why-tiktok-views-stop-at-200/
3. **Hook 1–3 giây đầu + retention đến cuối** quyết định qua/không qua
   test-pool; ngưỡng completion để "đi tiếp" được các vendor ước ~70% ở
   2026. [vendor claim] — Director phải viết hook 3 giây (đã là luật trong
   pipeline affiliate) và QC chấm retention-by-design (cắt nhanh, payoff cuối).
   Nguồn: https://posteverywhere.ai/blog/how-the-tiktok-algorithm-works
4. **Nhịp đăng đều thắng nhịp đăng dồn:** tài khoản mới nên 1 video/ngày ổn
   định tối thiểu ~3 tuần để thuật toán học niche; 3–5 video/tuần là mức duy
   trì phổ biến; tăng lên 2/ngày chỉ khi format đã chứng minh completion giữ
   được. [ước tính — đồng thuận giới làm nghề]
   Nguồn: https://www.lilachbullock.com/how-many-times-a-day-should-you-post-on-tiktok/ ·
   https://www.postermywall.com/blog/2026/09/18/best-tiktok-times-to-post/
5. **TikTok SEO:** tìm kiếm là kênh tăng trưởng dài hạn; caption/on-screen
   text/nội dung nói chứa từ khóa niche giúp video được phân loại đúng và
   sống lâu qua search. [vendor claim]
   Nguồn: http://vamp.com/insights/tiktok-algorithm/
6. **Series ăn follower, video đơn ăn view:** người xem follow vì *kỳ vọng
   tập tiếp theo*. Format có tập (đánh số, cùng khung, cùng persona) là đòn
   bẩy follower chính; video viral đơn lẻ thường không chuyển thành follower
   bền. [ước tính — đồng thuận giới làm nghề]
   Nguồn: https://appscrip.com/blog/tiktok-algorithm-explained/
   (tín hiệu "follows generated from a single video" được thuật toán theo dõi)
7. **LIVE là kênh follower + tiền, mở ở 1.000 follower** (đã encode trong
   code: `FollowersToLive = 1000`). LIVE đẩy follower nhanh hơn video vì
   tương tác trực tiếp; hệ thống đã có Streamer + Scheduler — growth chỉ cần
   coi LIVE là một "format" trong plan sau mốc 1.000. [ngưỡng: chính thức
   theo thực hành nền tảng + đã chốt trong MODEL.md]
8. **Creator Rewards KHÔNG áp dụng cho tài khoản Việt Nam** (danh sách vùng:
   US/UK/FR/DE/JP/KR/BR…). Ngưỡng chương trình: 10.000 follower + 100.000
   view/30 ngày, video ≥ 60 giây, nội dung original. [theo tài liệu chương
   trình được công bố rộng rãi — đối chiếu lại ở creators.tiktok.com khi cần]
   Nguồn: https://timetopost.co/blog/tiktok-creator-rewards-requirements-2026/ ·
   https://creatorsagency.co/blog/tiktok-creator-rewards-program-2026
   → **Hệ quả cho mạng của Ninh:** tiền TikTok đến từ LIVE gifts + TikTok
   Shop affiliate (đã có trong app), KHÔNG trông vào Creator Rewards. KPI
   TikTok = 1.000 follower (mở LIVE) → 10.000 (mốc uy tín/Shop), không phải
   mốc Rewards.

### 2.2 YouTube — Shorts kéo sub, long-form giữ tiền

1. **Shorts chạy engine riêng (tách khỏi long-form từ cuối 2025), mô hình
   Explore → Exploit:** mỗi Short được test trên "seed audience" nhỏ khớp
   chủ đề; tín hiệu quyết định là **swipe-away rate** (mục tiêu < ~30%
   [vendor claim]) và **% xem trung bình (APV)** — Short xem hết/vòng lặp
   được mở rộng dần theo giờ/ngày/tuần. Subscriber không quyết định lượt
   test đầu: kênh 0 sub vẫn bùng được. [cơ chế: vendor claim nhất quán
   nhiều nguồn; số %: ước tính]
   Nguồn: https://socialmediamarketingtechniques.com/youtube-shorts-algorithm-works/ ·
   https://www.socialpilot.co/youtube-marketing/youtube-algorithm
2. **Shorts feed là nguồn view organic lớn nhất:** ~61,5% view organic đến
   từ Shorts feed trong mẫu 799.718 video / 71.177 kênh (Metricool 2026);
   thời lượng xem trung bình một Short ~16 giây. [đo độc lập]
   Nguồn: https://metricool.com/youtube-shorts-algorithm/
3. **Phễu Shorts → long-form:** tính năng "Related Video" (ghim link từ
   Short sang video dài) là đường chuyển đổi chính thức; Short thắng =
   bằng chứng hook đã được kiểm chứng để dựng bản long-form. Long-form vẫn
   là nơi watch-hours và RPM cao sống. [chính thức về tính năng; hiệu quả:
   ước tính]
   Nguồn: https://medium.com/@artem.vardov/how-to-use-youtube-shorts-to-build-a-loyal-subscriber-base-in-2026-the-ultimate-growth-blueprint-c6e43cd3129b
4. **Ngưỡng YPP hiện hành** [chính thức]:
   - Tier fan-funding/Shopping: **500 sub** + 3 upload/90 ngày + (3.000 giờ
     xem 12 tháng **hoặc** 3 triệu view Shorts/90 ngày).
   - Full YPP (quảng cáo): **1.000 sub** + (4.000 giờ xem hợp lệ 12 tháng
     **hoặc** 10 triệu view Shorts/90 ngày). Hai đường **không cộng dồn**.
   Nguồn: https://air.io/en/monetization/youtube-partner-program-requirements-2026-the-complete-guide
5. **⚠️ Hạn chót quan trọng: từ 2027-02-01, người nộp MỚI phải đạt gấp đôi**
   — 8.000 giờ xem hoặc 20 triệu view Shorts/90 ngày (sub giữ nguyên 1.000);
   kênh đã ở trong YPP không bị ảnh hưởng. [chính thức — YouTube công bố
   2026-08] → Chiến lược: **đẩy các kênh qua cửa YPP trước 2027-02-01**;
   growth plan phải tính ngược từ hạn này.
   Nguồn: https://techcrunch.com/2026/08/10/youtube-now-requires-creators-to-have-twice-as-many-watch-hours-to-start-earning-money/ ·
   https://billo.app/blog/youtube-shorts-monetization/
6. **YouTube vừa siết phân phối Shorts "re-upload" (công bố 2026-10-01):**
   kênh chủ yếu đăng lại clip của người khác mà không thêm giá trị riêng sẽ
   bị giảm reach trong Shorts feed; "chỉnh sửa kỹ thuật nhỏ / thay template
   hàng loạt" không tính là original. [chính thức]
   Nguồn: https://www.searchenginejournal.com/youtube-original-shorts-reposted-clips-reach/591774/ ·
   https://www.socialmediatoday.com/news/youtube-updates-shorts-algorithm-to-put-more-focus-on-original-content/831976/

### 2.3 Kỳ vọng 30/60/90 ngày — nói thẳng

Mọi con số dưới đây là **[ước tính]**, biên độ rất lớn, **không phải cam
kết**: thuật toán chấm từng video; một video breakout đổi toàn bộ đường cong.

| Giai đoạn | TikTok (1 video/ngày, niche rõ) | YouTube (Shorts đều + long-form sau) |
|---|---|---|
| 0–30 ngày | Test-pool 200–500 view/video là bình thường; mục tiêu thật: tìm 1–2 format có completion đạt ngưỡng nội bộ; follower thường vài trăm–2.000 nếu format trúng | Shorts test theo seed audience; sub tăng chậm nếu chưa có Short thắng; mục tiêu: nhịp đăng ổn + đo APV/swipe |
| 30–60 ngày | Nhân đôi format thắng thành series; 1.000 follower → mở LIVE là mốc khả thi nếu có format thắng | Hướng 500 sub (tier fan-funding) nếu Shorts có nhịp; bắt đầu long-form từ hook Short đã thắng |
| 60–90 ngày | 3.000–10.000 follower nếu có breakout; LIVE đều → gifts + affiliate | Tích lũy về 1.000 sub + một trong hai đường YPP; **ưu tiên kịp trước 2027-02-01** |

Kênh AI-faceless **không có lợi thế thuật toán** so với kênh người thật —
chỉ có lợi thế sản lượng + kỷ luật đo. Kế hoạch phải thắng bằng vòng lặp
test → đo → kill/nhân đôi, không bằng hy vọng viral.

## 3. Bẫy chính sách (policy traps) — thiết kế phải né từ đầu

1. **TikTok "unoriginal content":** nội dung trùng lặp/không gốc bị loại khỏi
   Creator Rewards và bị hạ phân phối; bản thân các account trong cùng mạng
   đăng **cùng một file/cùng kịch bản** là rủi ro trực tiếp. App đã có
   guardrail "không trùng nội dung" (MODEL.md §7) — growth biến nó thành
   kiểm tra máy: dedup hash + so độ tương đồng kịch bản **trước khi đăng**.
   [chính thức về chính sách Rewards; mức thực thi: vendor claim]
   Nguồn: https://blog.nuelink.com/tiktok-creator-rewards-program/ ·
   https://www.lilachbullock.com/ai-generated-videos-monetised-tiktok/
2. **YouTube "reused content" + bản cập nhật 2025-07-15 về "inauthentic
   content":** nội dung sản xuất hàng loạt theo template, ít biến đổi, có thể
   bị từ chối YPP dù đủ số sub/giờ. Với nhà máy nội dung AI đây là rủi ro số
   1 ở cửa monetization — mỗi video phải có biến đổi đáng kể (kịch bản riêng,
   góc nhìn riêng, dựng riêng), không phải thay mỗi cái tên sản phẩm.
   [chính thức]
   Nguồn: https://testbook.com/question-answer/under-the-revised-youtube-partner-program-criteria--69199d98ab0511b4470be035
3. **Watermark chéo nền tảng:** Shorts/TikTok tải xuống mang watermark;
   đăng chéo nguyên file bị cả hai phía hạ reach. Luật cứng: **chỉ đăng từ
   master sạch** (file master 4K không watermark đã có trong pipeline
   Studio) và render biến thể riêng cho từng nền tảng. [chính thức về
   watermark Shorts; hạ reach: vendor claim nhất quán]
   Nguồn: https://www.androidpolice.com/youtube-shorts-watermarks/?ref=nextbigwhat ·
   https://subscribr.ai (thực hành re-purpose: bắt buộc bỏ watermark + adapt)
4. **TikTok Content Posting API — audit gate:** client chưa qua audit chỉ
   đăng được `SELF_ONLY` (chỉ mình xem), tối đa 5 user/24h và account phải
   để private — **vô dụng cho growth**. Đăng nháp (inbox draft) thì được cho
   account public nhưng người phải bấm đăng trong app. Muốn zero-touch đăng
   công khai phải qua **Direct Post audit** (cần website đối ngoại có
   Privacy Policy/Terms — repo đã có `docs/tiktok-app-site` — và demo flow;
   có rủi ro bị từ chối nếu bị coi là tool dùng riêng tư). Đây là việc
   **bootstrap một lần**, không phải vận hành. [chính thức theo tài liệu
   TikTok, đối chiếu 2026-09]
   Nguồn: https://www.outstand.so/blog/tiktok-content-posting-api ·
   https://github.com/openonion/connectonion/issues/262
5. **YouTube Data API v3 — quota là trần cứng theo project:** mặc định
   10.000 units/ngày/project; 1 lần upload = 1.600 units → **~6 upload/ngày
   cho cả project**, không phải mỗi kênh. Đọc số liệu thì rẻ (1 unit/lần).
   Hệ quả scale: mỗi kênh nên có Google Cloud project riêng (hoặc xin tăng
   quota) nếu muốn > 6 upload/ngày toàn mạng. Project chưa qua xác minh của
   Google còn bị khóa video ở private. [chính thức — quota calculator
   developers.google.com]
   Nguồn: https://github.com/palolol/palolol/blob/HEAD/apigoogle.md ·
   https://dev.to/siyabuilt/youtubes-api-quota-is-10000-unitsday-heres-how-i-track-100k-videos-without-hitting-it-5d8h
6. **AI disclosure:** TikTok yêu cầu gắn nhãn nội dung AI khi đăng; YouTube
   yêu cầu khai báo "altered/synthetic content" khi upload. Publisher phải
   tự set hai cờ này trong metadata — bỏ qua = rủi ro phạt + hạ reach.
   [chính thức] (Đã là luật trong pipeline: disclosure bắt buộc, Streamer
   không được tắt.)
7. **Không có đường tắt "warm-up" tự động:** các dịch vụ khuyên "nuôi"
   account bằng proxy/fingerprint/lướt-tương tác giả. **App tuyệt đối không
   làm**: tự động lướt/like/follow giả lập là engagement automation — vi
   phạm ToS, rủi ro khóa cả cụm account liên quan. Account mới đi thẳng vào
   đăng nội dung gốc nhịp thấp (canary §5.3); thuật toán phân loại qua nội
   dung, không cần diễn. [khuyến nghị thiết kế dựa trên chính sách nền tảng]

## 4. Thiết kế tính năng

### 4.1 Growth profile cho từng account

Bám vào `accounts` hiện có, thêm hồ sơ growth (bảng `growth_profiles`, §8):

- **Định danh:** platform (tiktok/youtube), username/channel, persona, niche
  (đã có), `ContentPillars` của persona = khung format ban đầu.
- **Giai đoạn growth** (`growth_stage`, chi tiết hóa trạng thái account):
  `cold_start` (0–~100 follower, canary) → `format_testing` (đang test các
  họ format) → `scaling` (đã có format thắng, nhân series + tăng nhịp) →
  `monetization_push` (sát ngưỡng LIVE/YPP, plan tối ưu về mốc) →
  `monetized` (đã qua cửa; growth chuyển sang tối ưu doanh thu cùng
  Analyst-tiền) → `stalled` (đứng số liệu, kích hoạt replan sâu). Account bị
  phạt vẫn dùng `status = penalized` như cũ, growth_stage đứng yên.
- **KPI đích (cấu hình được, có mặc định theo nền tảng):**
  - TikTok: follower 100 → **1.000 (mở LIVE)** → 10.000; median views/video;
    completion nội bộ.
  - YouTube: sub 100 → **500 (tier 1)** → **1.000 + 4.000h hoặc 10M Shorts
    (YPP full)**, có đếm ngược tới 2027-02-01.
- **Nhịp đăng hiện hành** (`cadence`): hệ thống tự điều chỉnh trong trần
  Governance đặt (TikTok mặc định 1/ngày, trần 3/ngày; YouTube Shorts
  1/ngày + long-form 2–3/tuần; trần cứng theo quota API §3.5).

### 4.2 Vòng lặp Growth Engine (ánh xạ đội agent hiện có)

```
        ┌──────────────────── mỗi account một vòng lặp độc lập ───────────────────┐
        │                                                                        │
  Hunter ──► Director ──► Producer ──► QC ──► Publisher ──► (nền tảng) ──► Analyst
  săn format/  content plan   Studio      3 lớp   biến thể      đăng theo      đọc metrics
  trend niche  30 ngày lăn    sản xuất    + dedup theo nền      lịch giờ       → kill/nhân đôi
        ▲        ▲                                        tảng + disclosure  → replan ──────┘
        │        └──────────────── Governance chốt mọi luật kill/scale/pause ────┘
        └────────────── bài học theo account (episodic memory, cô lập) ──────────┘
```

- **Hunter (mở rộng):** ngoài sản phẩm, săn *format đang thắng trong niche*:
  cấu trúc hook, độ dài, series của đối thủ (chỉ qua API chính thức/dữ liệu
  công khai được phép), nhạc trending (đã có chart trong Studio). Đầu ra:
  danh sách ứng viên format có bằng chứng (link video mẫu + chỉ số công khai).
- **Director (mở rộng):** duy trì **content plan 30 ngày lăn** cho từng
  account từ: trụ nội dung persona × format Hunter săn được × format đang
  thắng của chính account (Analyst báo). Tự viết biến thể kịch bản theo nền
  tảng (§5.2). Plan 60/90 ngày là khung pha (testing → scaling →
  monetization_push), không phải lịch cứng từng video.
- **Producer:** dùng Studio như hiện tại (job có storyboard, QC mắt người
  thay bằng QC agent theo lộ trình AGENT_TEAM). Một ý tưởng → N biến thể
  render (§5.2).
- **QC:** 3 lớp như AGENT_TEAM §5 + lớp 0 của growth: **dedup check** (hash
  file + tương đồng kịch bản với mọi video đã đăng toàn mạng) và **policy
  check** (AI-disclosure có mặt, không watermark lạ, độ dài/định dạng đúng
  nền tảng đích).
- **Publisher:** đăng theo lịch (Scheduler mở rộng sang lịch đăng), set cờ
  AI-disclosure, idempotency key như hiện tại. YouTube qua Data API v3
  (quota guard: đếm units/ngày/project trong `api_usage`, tự dời lịch khi
  chạm trần thay vì lỗi). TikTok: Direct Post sau audit; trước audit dùng
  draft (§6.3).
- **Analyst (mở rộng):** đồng bộ snapshots (§8) theo lịch (YouTube: Data API
  + Analytics API; TikTok: Display API — view/like/comment/share/follower;
  completion TikTok không có qua Display API → dùng proxy-metrics: tốc độ
  view 24h đầu, tỉ lệ share+save/view; completion đầy đủ để v3 qua Business
  API hoặc import). Từ snapshots tính `format_stats` và **đề xuất hành động
  qua Governance**:
  - **Kill format:** ≥ 8 video + ≥ 14 ngày mà median completion (hoặc proxy)
    dưới ngưỡng config (mặc định 35%) → format `killed`, Director ngừng lên
    plan, alert info kèm lý do.
  - **Nhân đôi (double-down):** 1 video ≥ 5× median views 30 ngày của chính
    account → tự sinh series 3 tập tiếp theo trong 7 ngày, cadence +1
    slot/ngày trong trần.
  - **Breakout:** video ≥ 10× median trong 48h → replan nhanh trong 24h
    (Director ưu tiên format đó), alert.
  - **Stalled:** 14 ngày follower tăng < 2% và completion dưới ngưỡng →
    replan sâu: Hunter săn lại format niche-kề, Director đổi họ format
    trong cùng trụ persona (không tự đổi persona/niche cốt lõi — đó là
    thay đổi chiến lược, hệ thống ghi quyết định vào Ledger + alert
    critical để Ninh *biết*, vẫn không cần Ninh duyệt).
  - **Tín hiệu phạt:** API trả lỗi policy, video bị gỡ, hoặc views_30d rơi
    > 70%/3 ngày không do lịch đăng → **tự pause account**
    (`status = paused`, reason ghi rõ), đồng thời Governance gắn cờ tránh
    họ format bị nghi gây phạt trên các account khác (theo MODEL.md §5.7:
    pause hành vi tương tự, tuyệt đối không "né" bằng account khác).

### 4.3 Kế hoạch 30-60-90 ngày (hệ thống tự sinh, tự lăn)

- **Ngày 0–30 (`cold_start` + `format_testing`):** canary 7 ngày đầu
  (§5.3), sau đó 1 video/ngày; mỗi tuần test 2–3 họ format từ trụ persona;
  mục tiêu duy nhất: tìm format qua ngưỡng completion nội bộ. Chưa LIVE,
  chưa bán hàng nặng — affiliate chỉ gắn nhẹ khi format đã ổn (tránh tín
  hiệu "kênh bán hàng" sớm).
- **Ngày 31–60 (`scaling`):** format thắng → series hóa (đánh số tập, lịch
  cố định), cadence tăng trong trần; TikTok chạm 1.000 → Scheduler bắt đầu
  xếp live; YouTube mở long-form từ hook Short thắng (Related Video ghim
  hai chiều).
- **Ngày 61–90 (`monetization_push`):** plan tính ngược từ mốc tiền gần
  nhất: TikTok LIVE đều + Shop affiliate (Hunter sản phẩm như pipeline hiện
  tại); YouTube dồn về 1.000 sub + một đường YPP **trước 2027-02-01**. Qua
  cửa → `monetized`, vòng growth nhập vào vòng tiền của Analyst hiện có.

Plan là **rolling**: mỗi tuần Analyst chốt số, Director viết lại 30 ngày
tiếp theo (bản plan mới `superseded` bản cũ, không sửa lịch sử — truy vết
được hệ thống đã nghĩ gì).

### 5.2 Một ý tưởng — nhiều biến thể (per-platform adaptation)

Cùng một *ý tưởng/format thắng*, Publisher nhận các biến thể khác nhau:

| Khía cạnh | TikTok | YouTube Shorts | YouTube long-form |
|---|---|---|---|
| File | Render riêng từ master sạch, không watermark nền tảng khác | Render riêng từ master sạch | Dựng bản dài riêng (không nối Shorts chắp vá) |
| Hook | Hook nói 3s + text overlay khớp từ khóa SEO | Hook thị giác 1–2s chống swipe | Title/thumbnail CTR + hook 30s |
| Caption/metadata | Caption chứa từ khóa niche (SEO TikTok), 3–5 hashtag | Title ngắn chứa từ khóa search, mô tả + Related Video | Title/description/tags theo search intent |
| Âm thanh | Nhạc trending native TikTok (chart trong Studio) | Nhạc/âm thanh hợp lệ trên YouTube | Nhạc có quyền dùng |
| Disclosure | Bật nhãn AI-content của TikTok | Khai báo synthetic content khi upload | Khai báo synthetic content |
| Lịch | Trừ hao 24–72h so với nền tảng gốc khi funnel chéo | Đăng sau TikTok 24–72h (hoặc ngược lại nếu YouTube thắng trước) | Sau khi Short tương ứng thắng |

**Funnel chéo hợp lệ:** TikTok thắng → bản adapt lên Shorts (và ngược lại);
Short thắng → dựng long-form; cuối video/profile dẫn về nhau bằng tính
năng native (link in bio, Related Video, ghim comment). Không dẫn bằng
watermark, không đăng cùng file hai nơi, không kêu gọi tương tác giả.

### 5.3 Canary tự động (thay cổng duyệt người)

1. Account/persona mới: 7 ngày đầu cadence tối thiểu (TikTok 1 video/ngày,
   YouTube 1 Short/ngày, chưa long-form, chưa LIVE kể cả đủ follower).
2. Điều kiện lên nhịp (Governance tự kiểm): QC pass 100%, 0 lỗi policy từ
   API, completion median không dưới sàn cứng (mặc định 25%), 0 cảnh báo
   gỡ video.
3. Vi phạm bất kỳ điều kiện nào → tự lùi về cadence tối thiểu hoặc pause +
   alert critical kèm hành động đã tự làm.
4. Sau canary: cadence do Analyst điều chỉnh theo §4.2, vẫn trong trần.

## 6. Ranh giới tự động hóa (bảng chốt)

### 6.1 Hệ thống tự làm 100% (vận hành hằng ngày — Ninh không chạm)

- Lập & lăn content plan 30-60-90 cho từng account; săn trend/format.
- Viết kịch bản + biến thể đa nền tảng; sản xuất qua Studio; QC 3 lớp +
  dedup + policy check.
- Đăng theo lịch trong trần API/chính sách (YouTube Data API; TikTok Direct
  Post sau khi có audit); set AI-disclosure; idempotency chống đăng trùng.
- Đọc metrics, chụp snapshots, tính format_stats; kill/nhân đôi format;
  replan hằng tuần + replan nhanh khi breakout.
- Tự pause account khi có tín hiệu phạt; tự hạ nhịp khi quota API sắp cạn
  hoặc metrics sập bất thường.
- Xếp lịch LIVE sau mốc 1.000 follower (Scheduler/Streamer hiện có).
- Gửi alert notify-only (Telegram/UI) kèm hành động đã tự thực hiện.

### 6.2 Bootstrap một lần duy nhất (Ninh làm, xong là hết)

| Việc | Vì sao không tự động được |
|---|---|
| Tạo tài khoản TikTok/YouTube, xác minh SĐT/email, qua CAPTCHA/2FA | Nền tảng bắt người thật; tự động tạo account = vi phạm ToS |
| Giao credentials/OAuth từng account một lần (app đã có flow OAuth + token file riêng từng account) | Quyền sở hữu tài khoản |
| Nộp **TikTok Direct Post audit** (website đối ngoại + Privacy/Terms — dùng `docs/tiktok-app-site`; quay demo flow) | TikTok duyệt *doanh nghiệp/app*, không duyệt thay được; có rủi ro bị từ chối nếu bị coi là tool riêng tư — chấp nhận rủi ro này hoặc chấp nhận TikTok ở chế độ draft (xem §6.3) |
| Tạo Google Cloud project + xác minh API + (khi scale) xin tăng quota YouTube; khuyến nghị 1 project/kênh để quota không nghẽn chung | Google yêu cầu chủ project xác minh |
| Lấy RTMP key, xin quyền LIVE trong app TikTok từng account | Quyền nằm trong app của chủ account |
| Khai AdSense + thông tin thuế/thanh toán khi kênh đạt YPP; đăng ký TikTok Shop seller/affiliate nếu bán hàng | Pháp lý/tiền của Ninh |
| Nạp Gemini key / API key vào Settings (app đã có sẵn mục này) | Secrets của Ninh |

Sau bootstrap, các mục trên **không xuất hiện lại trong vận hành** — token
refresh, quota, lịch, số liệu đều tự động. Hết hạn/bị thu hồi token thì hệ
thống báo alert critical "cần cấp lại quyền" (ngoại lệ duy nhất, vì nền
tảng không cho cách khác).

### 6.3 Giới hạn nền tảng phải nói thẳng

- **Trước khi có TikTok audit:** đăng công khai zero-touch qua API là
  *không thể* (chưa audit = `SELF_ONLY`). Lựa chọn trung thực: (a) làm
  bootstrap audit để mở full-auto; hoặc (b) TikTok chạy chế độ draft —
  hệ thống tự sản xuất + đẩy draft, Ninh chạm 1 nút đăng trong app khi
  rảnh. (b) là ngoại lệ do nền tảng ép, không phải quyết định vận hành;
  YouTube không bị giới hạn này. Khuyến nghị: (a).
- **TikTok completion/retention** không lấy được qua Display API → vòng
  tối ưu TikTok dùng proxy-metrics (view velocity, share/save rate) ở
  MVP/v2; retention đầy đủ chỉ khi nối Business API (v3, tùy chọn).
- **Quota YouTube** là trần theo project (§3.5): số account YouTube × nhịp
  đăng phải khớp quota; hệ thống tự dời lịch khi chạm trần và alert khi
  cần thêm project/quota — quyết định thêm project là bootstrap, không phải
  vận hành.

### 6.4 Cấm tuyệt đối (không thiết kế, không ngoại lệ)

- Mua/thuê follower, view, like; dùng dịch vụ "buff" dưới mọi hình thức.
- Bot tương tác: tự like/comment/follow/share — **kể cả giữa các account
  của chính Ninh với nhau** (đã là guardrail MODEL.md §7; growth không mở
  lại cửa này).
- Giả lập người dùng: auto-browsing "warm-up", proxy/fingerprint farm để
  qua mặt hệ thống phát hiện.
- Reupload nguyên xi nội dung người khác, hoặc đăng cùng một video lên
  nhiều account của mình (kể cả đổi tên file).
- Né phạt bằng account khác khi một account bị phạt (ban evasion).

Lý do không phải đạo đức suông: cả hai nền tảng đều có cơ chế phạt
(account `penalized`, mất phân phối, từ chối YPP/Rewards) và tài khoản là
tài sản khan hiếm nhất của hệ thống (ARCHITECTURE §1.6). Một account chết
vì buff bẩn kéo theo rủi ro liên đới cả cụm cùng thiết bị/mạng.

## 7. Đặc tả UI (mọi tính năng đều có UI — luật nhà)

Không sửa code web trong đợt thiết kế này; đặc tả để đợt build bám theo
`UI_UX_BLUEPRINT.md`:

1. **Trang mới `/growth` (nhóm Vận hành — "Phát triển kênh"):**
   - Bảng toàn mạng: mỗi account một dòng — platform, stage badge, follower/
     sub hiện tại + **progress bar tới mốc KPI kế tiếp**, view 30 ngày,
     số format winner/killed, cadence hiện hành, cảnh báo mở.
   - Bộ lọc theo platform/stage; sparkline follower 30 ngày (từ snapshots).
   - Khối "hạn chót": đếm ngược YPP 2027-02-01 cho các kênh YouTube chưa
     qua cửa; tình trạng quota YouTube hôm nay (units đã dùng/trần).
2. **Khối "Phát triển kênh" trong trang chi tiết account** (mở rộng trang
   account hiện có):
   - Growth profile: niche/persona (sửa được), KPI milestones dạng checklist
     tự tích khi đạt (1.000 LIVE, 500 sub, YPP…), growth_stage hiện tại +
     điều kiện lên stage (hệ thống tự tính, hiển thị để Ninh *xem*).
   - Content plan 30 ngày: danh sách theo ngày (format, nền tảng, trạng thái
     planned→published) + nút "Replan ngay" (ra lệnh cho hệ thống, không
     phải duyệt plan).
   - **Format scoreboard:** bảng theo họ format — số video, median
     completion/proxy, views trung bình, follow/1.000 view, verdict
     (testing/winner/killed) + lý do verdict.
   - Nhật ký quyết định growth (từ `decisions`): kill/nhân đôi/replan/pause
     — đọc được hệ thống đã tự quyết gì, khi nào, vì số liệu nào.
3. **Trang Analytics mở rộng:** thêm section tăng trưởng tách khỏi section
   tiền hiện có — follower toàn mạng theo thời gian, median completion theo
   format, tỉ lệ format thắng sau test (hit-rate) theo account.
4. **Alerts:** danh sách ở `/growth` + Telegram notify-only; mỗi alert ghi
   `action_taken` (hệ thống đã tự pause/kill/replan gì) — không có nút
   "duyệt" nào trong luồng growth, chỉ có nút "đã đọc".

Mọi trang vẫn tôn trọng kill switch + banner dry-run như hiện tại; khi
kill switch bật, vòng growth dừng cùng mọi agent khác.

## 8. Phác thảo data model (SQLite, bám phong cách Ledger hiện tại)

```sql
-- Hồ sơ growth 1-1 với account (persona/niche/followers đã có ở accounts)
CREATE TABLE growth_profiles (
    account_id     INTEGER PRIMARY KEY REFERENCES accounts(id),
    growth_stage   TEXT NOT NULL DEFAULT 'cold_start',
    -- cold_start | format_testing | scaling | monetization_push | monetized | stalled
    cadence_per_day REAL NOT NULL DEFAULT 1.0,   -- hệ thống tự chỉnh trong trần Governance
    plan_horizon_days INTEGER NOT NULL DEFAULT 30,
    started_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Mốc KPI có thứ tự cho từng account (đạt mốc = reached_at, không sửa ngược)
CREATE TABLE growth_targets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER NOT NULL REFERENCES accounts(id),
    metric      TEXT NOT NULL,   -- followers | subs | watch_hours_12m | shorts_views_90d
    threshold   REAL NOT NULL,
    label       TEXT NOT NULL,   -- 'Mở LIVE' | 'YPP tier 1 (500 sub)' | 'YPP full' | ...
    deadline    TEXT,            -- vd '2027-02-01' cho mốc YPP của kênh chưa qua cửa
    reached_at  TEXT,
    UNIQUE(account_id, metric, threshold)
);

-- Kế hoạch nội dung: bản mới supersede bản cũ, không sửa lịch sử
CREATE TABLE content_plans (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER NOT NULL REFERENCES accounts(id),
    phase       TEXT NOT NULL,   -- d30 | d60 | d90
    status      TEXT NOT NULL DEFAULT 'active',  -- active | superseded
    rationale   TEXT,            -- Analyst ghi căn cứ số liệu của lần replan này
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE content_plan_items (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id       INTEGER NOT NULL REFERENCES content_plans(id),
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    planned_for   TEXT NOT NULL,          -- thời điểm dự kiến (ICT)
    format_id     TEXT NOT NULL,          -- họ format: 'story_ep' | 'tip_60s' | ...
    platform_variant TEXT NOT NULL,       -- tiktok | youtube_shorts | youtube_long
    status        TEXT NOT NULL DEFAULT 'planned',
    -- planned | producing | qc | queued | published | dropped
    content_item_id INTEGER,              -- liên kết content_items/studio job khi đã sản xuất
    published_ref   TEXT,                 -- video id trên nền tảng sau khi đăng
    UNIQUE(account_id, planned_for, format_id, platform_variant)
);

-- Điểm theo họ format × account — Analyst tổng hợp, Governance đọc để kill/scale
CREATE TABLE format_stats (
    account_id        INTEGER NOT NULL REFERENCES accounts(id),
    format_id         TEXT NOT NULL,
    videos            INTEGER NOT NULL DEFAULT 0,
    median_completion REAL,               -- NULL nếu nền tảng không cấp (TikTok qua Display API)
    avg_views         REAL,
    follows_per_1k    REAL,               -- follower/sub mới trên 1.000 view
    verdict           TEXT NOT NULL DEFAULT 'testing',  -- testing | winner | killed
    verdict_reason    TEXT,
    updated_at        TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (account_id, format_id)
);

-- Ảnh chụp số liệu theo thời điểm (append-only như các bảng tiền)
CREATE TABLE account_metric_snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER NOT NULL REFERENCES accounts(id),
    taken_at    TEXT NOT NULL DEFAULT (datetime('now')),
    followers   INTEGER,
    views_30d   REAL,
    videos      INTEGER,
    extra_json  TEXT,   -- watch_hours_12m, shorts_views_90d (YouTube), quota còn lại...
    source      TEXT NOT NULL  -- api | import
);

CREATE TABLE video_metric_snapshots (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id       INTEGER NOT NULL REFERENCES accounts(id),
    platform_video_id TEXT NOT NULL,
    taken_at         TEXT NOT NULL DEFAULT (datetime('now')),
    views INTEGER, likes INTEGER, comments INTEGER, shares INTEGER, saves INTEGER,
    completion REAL,          -- NULL khi API không cấp
    extra_json TEXT           -- apv, swipe_away (YouTube Analytics), view velocity...
);

-- Cảnh báo growth: luôn kèm hành động hệ thống đã tự làm (notify-only)
CREATE TABLE growth_alerts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER REFERENCES accounts(id),
    severity    TEXT NOT NULL,  -- info | warn | critical
    kind        TEXT NOT NULL,  -- breakout | penalty_signal | stalled | milestone | quota | token_expired
    message     TEXT NOT NULL,
    action_taken TEXT,          -- hệ thống đã tự pause/kill/replan/dời lịch gì
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
```

Nguyên tắc dữ liệu: snapshots append-only (không sửa số cũ); quyết định
kill/nhân đôi/replan ghi vào `decisions` sẵn có kèm input số liệu (audit
trail như luật tiền); metric chỉ lấy từ API nhà cung cấp — **không bịa số**
(ARCHITECTURE §1.5 áp dụng nguyên xi cho growth).

## 9. Lộ trình xây (cỡ công việc tương đối: S = vài ngày, M = 1–2 tuần, L = nhiều tuần — một người làm bán thời gian; đây là cảm giác cỡ việc, không phải cam kết ngày tháng)

### MVP — "đo được, đăng được, tự pause được" (cỡ M)
1. Bảng `growth_profiles`, `growth_targets`, `account_metric_snapshots`,
   `video_metric_snapshots`, `growth_alerts` + đồng bộ metrics tự động
   (YouTube Data/Analytics API; TikTok Display API cơ bản) theo lịch.
2. Trang `/growth` overview + khối growth trong account detail (stage,
   progress tới mốc, milestones tự tích, hạn chót YPP 2027).
3. Canary tự động + alert penalty cơ bản (API policy error/vi phạm → tự
   pause + alert kèm action_taken).
4. Đăng theo plan đơn giản: Director sinh plan 30 ngày từ trụ persona
   (chưa cần Hunter-format), Producer/Publisher chạy như hiện tại; TikTok
   theo §6.3 (draft trước audit — ngoại lệ nền tảng), YouTube full-auto
   trong quota guard.

### v2 — "vòng lặp tự tối ưu" (cỡ M–L)
1. `content_plans`/`content_plan_items` rolling 30 ngày + replan hằng tuần
   tự động; trang plan trong account detail.
2. `format_stats` + luật kill/double-down/breakout qua Governance (ngưỡng
   trong config, có test như luật tiền).
3. Pipeline một-ý-tưởng-nhiều-biến-t
...[truncated 6241 chars]

---

## 10. Trạng thái triển khai (cập nhật 2026-10-02 — MVP đã code xong, chưa commit)

Package `internal/growth` + trang `/growth`. Mọi mục dưới đây đều có test tự
động kèm theo (`go test ./internal/growth/`, 23 test).

### ĐÃ LÀM (MVP)

1. **Schema** (`internal/growth/store.go`, DDL theo tiền lệ studio.go — tự tạo
   bảng lúc khởi tạo store, không sửa `schema.sql`): `growth_profiles`,
   `growth_targets`, `content_plans`, `content_plan_items`, `format_stats`,
   `metric_snapshots` (tên trong code; doc gọi là
   `account_metric_snapshots`), `video_metric_snapshots`, `growth_alerts`.
   Hồ sơ + thang mốc tự sinh khi gặp tài khoản mới (zero-touch bootstrap):
   TikTok 100/1.000/10.000 follower; YouTube 100/500/1.000 sub + 4.000 giờ
   xem 12 tháng HOẶC 10 triệu view Shorts/90 ngày, mốc đầy đủ gắn hạn YPP
   **2027-02-01**.
2. **Trang `/growth`** (nhóm Vận hành): thẻ tổng theo dõi/lượt xem, đếm ngược
   tới hạn YPP, bảng từng tài khoản (badge giai đoạn, progress tới mốc kế,
   trạng thái kết nối YouTube), cảnh báo kèm hành động hệ thống đã làm, danh
   sách plan sắp tới toàn mạng, nút "Đồng bộ số liệu + chạy vòng quyết định"
   và nút sinh plan 30 ngày từng tài khoản. Account detail có khối Phát
   triển kênh (thang mốc + progress, plan sắp tới, hiệu quả theo format,
   cảnh báo, nút Replan). Empty-state đầy đủ, nhãn tiếng Việt, đúng CSS
   variables hai theme.
3. **Plan engine thuần logic** (`engine.go`): nhịp theo giai đoạn đúng §9 —
   cold_start canary 7 ngày (TikTok 1/ngày, Shorts 1/ngày nếu có YouTube,
   KHÔNG video dài) rồi mới ramp; format_testing/shorts; scaling nhân đôi
   (series đánh số tập, 2 video dài/tuần); monetization_push 3 video
   dài/tuần. Slugify bỏ dấu tiếng Việt cho `format_id`.
4. **Luật quyết định** (config `DefaultConfig`, có table test):
   - Kill format: ≥8 video VÀ ≥14 ngày VÀ median completion < 35%; không có
     completion thì dùng proxy (share+save)/view < 1% — đúng ngưỡng §4.2 có
     thể chỉnh.
   - Double-down: video ≥5× median view toàn kênh → format winner (series);
     breakout ≥10×.
   - Penalty: views 30 ngày rơi >70% giữa hai lần đọc → tự tạm dừng tài
     khoản (qua AccountManager, giữ nguyên máy trạng thái hợp lệ) + alert
     critical + decision log. Policy-error/vỡ video: **chưa có nguồn số
     liệu ở MVP nên để 0** — chỉ leg views là sống.
   - Stalled: follower tăng <2% trong ≥14 ngày → stage `stalled`.
   - Chuyển giai đoạn: canary sạch 7 ngày → format_testing; có winner →
     scaling; ≥80% mốc kế → monetization_push; qua cửa (TikTok: mốc 1.000;
     YouTube: 1.000 sub + giờ/view đủ) → monetized. Không nhảy bậc.
   - Mọi quyết định ghi `decisions` (audit dùng chung) + alert notify-only
     ghi rõ việc hệ thống ĐÃ làm.
5. **Metrics** (`metrics.go`): interface `MetricsSource` + client YouTube
   Data API v3 (`channels.list?part=statistics`, key qua Settings
   `YOUTUBE_API_KEY`, gắn kênh qua trường Kênh YouTube sẵn có ở account
   detail). **Fail-closed tuyệt đối**: thiếu key/quota/lỗi HTTP → lỗi
   "chưa kết nối", không ghi snapshot, không bịa số. TikTok: nguồn luôn
   "chưa kết nối" (draft-only tới khi có audit Direct Post) — không
   scraping. Giới hạn trung thực: Data API chỉ cho tổng view tích lũy +
   số video; `views_30d` và giờ xem YPP cần Analytics API (OAuth) — để
   trống, luật penalty views-30d chỉ chạy khi có nguồn cấp chỉ số đó.

### CHƯA (nợ giai đoạn 2)

- **Biến thể đa nền tảng + dedup enforcement** (một ý tưởng → TikTok/Shorts/
  long-form, chặn trùng lặp lịch sử): plan engine mới lên lịch theo ngày
  và trụ; chưa có `concept_hash` dedup chéo.
- **Hook A/B** và biến thể thumbnail/title: chưa có cột/bucket thử nghiệm.
- **TikTok full metrics** qua Business/Display API sau audit: chưa có OAuth
  số liệu; hiện TikTok không có snapshot nào (trang growth nói rõ).
- **Nguồn policy-error/takedown** cho luật penalty: chưa có provider nào
  cấp; hiện chỉ leg views-30d hoạt động, mà leg đó cũng chờ chỉ số 30 ngày
  (YouTube Analytics API OAuth).
- **Nối plan item → Studio job**: chưa có hook sạch (plan item không tự
  sinh job; trạng thái published chỉ được ghi khi có nguồn đăng thật ghi
  `published_ref`). Plan engine chỉ lên lịch/mô tả.
- **Đồng bộ nền theo lịch (daemon)**: vòng sync hiện chạy khi bấm nút ở
  /growth; chưa cắm vào daemon nền zero-touch.
- **Quota YouTube theo project/kênh** (§6.3): chưa triển khai bộ đếm quota;
  cần khi bắt đầu Analytics API.

### Kiểm chứng (2026-10-02)

`go build ./...` OK; `go test ./...` xanh 20/20 package có test (package
`internal/growth` mới: 23 test gồm table test luật kill/double-down/
penalty/stalled/chuyển giai đoạn, test DB supersede plan, snapshot
append-only, metrics fail-closed qua httptest). Smoke test binary thật:
`/growth`, `/`, `/accounts`, `/accounts/new`, `/settings` → 200; tạo tài
khoản → stage canary hiển thị đúng; sinh plan → 303; account detail render
khối growth; đồng bộ không key → báo "chưa kết nối" cho cả hai nguồn.

---

## 11. Giai đoạn 2 — nối plan vào sản xuất thật (cập nhật 2026-10-02)

Giai đoạn 2 đóng đúng 4 món nợ của mục 10: hook plan → Studio, biến thể đa
nền tảng + chống trùng, đường đăng YouTube fail-closed có quota guard, và
đồng bộ nền. Toàn bộ chạy dưới một **vòng tự động (growth tick) 5 phút**
trong daemon `aicos` (`web.Server.GrowthAutomationTick`), gồm 3 việc theo
thứ tự: đồng bộ số liệu (tối đa mỗi giờ, dùng lại `syncOneAccount` của
/growth thủ công), sản xuất các mục plan đến hạn, và đăng YouTube các biến
thể đã render xong. Riêng 2 việc cuối gác cổng bằng toggle "Sản xuất & đăng
tự động" ở trang /growth (setting `growth.production_enabled`, ghi sổ quyết
định mỗi lần đổi). **Đợt 3 (zero-touch): mặc định BẬT khi chưa từng lưu —
giá trị "0" đã lưu luôn được tôn trọng; DRY-RUN là cổng an toàn toàn cục
chặn mọi sản xuất/đăng thật, kill switch thắng tất cả.** Bút toán đồng bộ cuối ghi ở
`growth.last_sync`. Nút "Chạy một vòng ngay" bỏ qua giờ chờ đồng bộ để kiểm
tra bằng tay.

### ĐÃ NỐI (phase 2)

1. **Hook plan → Studio** (`internal/web/growth_produce.go`): mục plan đến
   ngày (kèm stagger, xem dưới) của tài khoản đang hoạt động sẽ vào Studio:
   tài khoản có chủ đề sản phẩm → luồng affiliate (tự chọn sản phẩm theo cơ
   chế luân phiên sẵn có của autopilot); tài khoản persona → film job với
   độ dài theo biến thể. Mục chuyển `producing` + ghi `studio_job_id`
   **trước khi** enqueue, nên mọi tick tiếp theo hoặc restart app đều chỉ
   thấy "đã có job" — không sinh trùng (có test restart dùng lại cùng thư
   mục data). Mỗi tick tối đa 3 job/tài khoản. Lỗi enqueue: item ở lại
   `planned`, tăng `attempts`, không bao giờ lặp nóng (chờ tick sau); lỗi
   render: item `failed` + alert + ghi quyết định, không tự thử lại vô hạn.
   DRY-RUN và kill switch chặn enqueue (item ở nguyên `planned`, trang
   /growth hiện badge lý do) nhưng KHÔNG chặn đồng bộ số liệu và đánh giá
   luật kênh.
2. **Biến thể theo nền tảng** (`internal/growth/production.go`): cùng ý
   tưởng, mỗi biến thể là một render riêng — TikTok 60s đăng nháp (xem giới
   hạn), Shorts 45s, bản dài 150s + tiêu đề thêm "— bản đầy đủ" (Shorts thêm
   ` #Shorts`), caption/hashtag riêng theo biến thể, và **stagger lịch**:
   TikTok ngày N, Shorts N+1, bản dài N+3 (plan engine đã ghi
   `variant_group = <ngày>|<format>` cho mọi mục; cùng group = các anh em
   của một ý tưởng). Mốc cũ quá 14 ngày bị `dropped` thay vì render muộn.
3. **Chống trùng chéo** (`internal/growth/dedup.go`): khi một mục được sản
   xuất, hệ thống tính `concept_hash` = SHA-256 của tập token đã chuẩn hoá
   (gấp dấu tiếng Việt, bỏ chữ 1 ký tự) của ý tưởng (chủ đề gốc trước hậu tố
   "— tập N"/"— bản dài"), và so độ tương đồng Jaccard token-set với mọi
   item đã sản xuất/đăng 45 ngày gần nhất TRÊN TOÀN MẠNG (mọi tài khoản).
   Miễn so: chính item, anh em cùng variant group trên cùng tài khoản, và
   cùng dòng series (cùng tài khoản + format — script của Studio luôn viết
   mới theo tập). ≥0,8 → item bị ép đổi góc kể (8 góc xác định, deterministic
   theo item id + số lần thử) rồi mới enqueue, kèm alert `dedup` + quyết
   định trong sổ; pipeline không dừng. Lưu ý trung thực: renderer không
   nhận seed viết kịch bản, nên "đổi góc" thực thi bằng đổi tiêu đề/góc kể
   của item — script Studio vốn đã viết mới mỗi job, đây là lớp bảo vệ ở
   tầng kế hoạch.
4. **Đăng YouTube fail-closed + quota** (`PublishVideo` của
   `internal/publishers` — resumable upload Data API v3, quota 1.600
   units/lượt): trạng thái kết nối đọc từ chính cơ chế `/settings/env`
   (`YOUTUBE_CLIENT_ID`, `YOUTUBE_CLIENT_SECRET` + token OAuth theo từng
   tài khoản tại `data/youtube_token_<username>.json` + trường channel id,
   ẩn danh mặc định lấy từ `YOUTUBE_DEFAULT_PRIVACY` — đã thêm vào danh sách
   biến môi trường Cài đặt). Thiếu bất kỳ mảnh nào: item ở trạng thái
   `waiting_connect` ("Chờ kết nối YouTube") với ghi chú nêu đúng mảnh
   thiếu; không có đường nào giả vờ đã đăng. Bộ đếm quota theo ngày ở bảng
   `growth_youtube_quota` (1 ngày = 10.000 units ≈ 6 lượt tải cho TOÀN BỘ
   project OAuth): hết quota → `waiting_quota` + alert + quyết định, chờ
   qua ngày. Chỉ trừ quota khi tải lên thành công (upload lỗi giữ quota,
   tránh khoá oan). Video luôn đăng kèm cờ `containsSyntheticMedia` và
   dòng công khai "nội dung tạo/hỗ trợ bởi AI" trong mô tả. Khi cả Shorts
   và bản dài của cùng ý tưởng đã có link, hai item tự nối
   `related_item_id` hai chiều (metadata cho tính năng Related Video của
   YouTube; mô tả của bản đăng sau chứa link youtu.be của bản đăng trước).
   Kênh chưa bật loại nội dung tương ứng cũng rơi về `waiting_connect`
   thay vì thử lại vô hạn.
5. **UI**: thẻ "Sản xuất & đăng tự động theo kế hoạch" ở /growth (bật/tắt,
   badge dry-run/kill, đồng hồ quota ngày, nút chạy ngay); cột "Đăng
   YouTube" (Đã sẵn sàng / thiếu mảnh nào) ở bảng tài khoản; bảng "Kế
   hoạch & sản xuất sắp tới" hiện trạng thái từng mục kể cả `produced` /
   `waiting_connect` / `waiting_quota`, badge "đã đổi góc" khi dedup can
   thiệp, badge anh em "Shorts ↔ bản đầy đủ"; trang chi tiết tài khoản có
   bảng "Đã sản xuất / đang xử lý gần đây" (job Studio, link youtu.be).
   Tất cả tiếng Việt, dùng biến màu theme sẵn có.

### CÒN LẠI (sau phase 2)

- **Bản dài YouTube chưa có render 16:9 thật**: Studio hiện render dọc
  9:16 cho mọi job; biến thể long-form khác ở độ dài (150s) và metadata,
  CHƯA khác ở khung hình. Render ngang là việc của Studio, không nằm trong
  vòng growth.
- **Quota theo kênh (§6.3)**: bộ đếm hiện là 1 project dùng chung — đúng
  cho cấu hình OAuth hiện tại. Tách project theo kênh khi scale (nhiều
  OAuth client + token theo tài khoản đã hỗ trợ sẵn ở tầng token).
- **YouTube Analytics API** (views 30 ngày, giờ xem YPP): vẫn là nợ từ MVP —
  luật penalty giờ-xem chưa có số thật; đường OAuth hiện có là cho upload.
- **TikTok**: vẫn đăng nháp qua luồng affiliate sẵn có; đăng công khai chờ
  audit Direct Post; metrics TikTok chờ OAuth số liệu (MVP đã ghi rõ).
- **A/B hook + thumbnail**: chưa có bucket thử nghiệm; trường `attempts` và
  lineage biến thể đã sẵn làm nền.
- **Đồng bộ nền phụ thuộc app đang chạy**: tick nằm trong tiến trình
  `aicos` (không phải daemon hệ thống riêng) — đúng mô hình Ninh mở máy
  24/7 và chạy app như hiện tại.

### Kiểm chứng phase 2 (2026-10-02)

`go build ./...`, `go vet ./internal/...`, `gofmt` sạch trên file mới;
`go test ./...` xanh 20/20 package. Riêng `internal/growth` +
`internal/web`: 63 test đạt, gồm hash/similarity/dedup (gấp dấu, anh em
miễn trừ, ≥0,8 ép đổi góc), biến thể (lag/tựa/caption khác nhau), quota
guard (6 lượt/ngày, chỉ trừ khi thành công), hook idempotent (tick 2 lần +
restart không trùng job), fail-closed (chưa kết nối → waiting_connect,
enqueue lỗi → retry có kiểm soát, dry-run chặn đứng), và toggle/mẫu trang
/growth. Smoke test binary thật trên cổng riêng: tạo tài khoản → /growth
hiển thị thẻ sản xuất ở trạng thái TẮT (đúng mặc định) → bật toggle →
"đang bật" → chạy một vòng → sinh plan → bảng sắp tới + cột "Đăng YouTube"
render đúng; log máy chủ không panic, tick báo đã gác cổng. *(Ghi chú Đợt 3:
mặc định nay là BẬT khi chưa lưu — bản cài cũ đã lưu "0" vẫn giữ TẮT.)*
