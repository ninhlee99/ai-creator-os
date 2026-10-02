# Nghiên cứu pivot: Affiliate Accesstrade + Reup Douyin

Trạng thái: **nghiên cứu, chưa code, chưa commit**.
Ngày: 2026-10-02. Người chốt pivot: Ninh — bỏ live + bỏ phim, tập trung
(1) affiliate qua **Accesstrade Vietnam**, (2) reup video viral Douyin
(thần tiên/tỉ tỉ) có transform chống bản quyền. Tự động 100%.

> Lưu ý trung thực đọc trước: **không có kỹ thuật nào đảm bảo 100% không bị
> đánh bản quyền.** Content ID/perceptual hash ngày càng mạnh; transform chỉ
> giảm rủi ro. Lớp bảo vệ mạnh nhất là nội dung gốc thêm vào (voiceover bình
> luận tiếng Việt, biên tập compilation) vì mang tính transformative.

---

## 1. Accesstrade Vietnam — Publisher API

Tài liệu chính thức: **https://developers.accesstrade.vn/**
(bản markdown lấy bằng cách thêm `.md` vào URL trang docs).

### 1.1. Xác thực
- Mọi request (GET/POST/PUT…) đều cần header:
  `Authorization: Token <access_key>` + `Content-Type: application/json`.
- `access_key` lấy trong trang publisher profile:
  `https://pub.accesstrade.vn/publisher_profile/personal_info?position=info`
  (một số docs ghi `https://pub2.accesstrade.vn/profile/api_key`).
- Không OAuth phức tạp, không thấy sandbox công khai → test bằng key thật
  trên tài khoản publisher của Ninh (nên tạo key riêng cho app, có thể revoke).

### 1.2. Endpoint chính (base `https://api.accesstrade.vn`)

| Chức năng | Method | Endpoint |
|---|---|---|
| Danh sách chiến dịch (+ chi tiết, filter `approval=successful`, `campaign_id`) | GET | `/v1/campaigns` |
| Tạo tracking/affiliate link (deeplink, utm, sub1-4, short link) | POST | `/v1/product_link/create` |
| Datafeed sản phẩm (tên, ảnh, giá, giảm giá, `aff_link`, category, SKU; filter domain/price/discount, `limit` max 200) | GET | `/v1/datafeeds` |
| Danh sách đơn hàng v2 (since/until ISO, status 0=pending/1=approved/2=rejected, `pub_commission`, utm attribution) | GET | `/v1/order-list` |
| Giao dịch, chi tiết đơn, sản phẩm của đơn, chi tiết sản phẩm | GET | `/v1/...` (xem llms.txt) |
| Voucher/coupon/deal + khuyến mại đang chạy + top sản phẩm bán chạy | GET | `/v1/...` (xem llms.txt) |
| TikTok Shop product search v2 (sort `HIGH_COMMISSIOM_RATE`, `BEST_SELLERS`, filter `title_keywords`, phân trang `page_token`) | GET | `/v2/tiktokshop_product_feeds` |

### 1.3. Rate limit & lưu ý vận hành
- `/v1/order-list`: **10 requests/phút**, cache 1 phút phía server.
- Commission lúc đơn còn hold thường trả `0`, sau đó mới điền số ước tính —
  sync đối soát phải **UPDATE tại chỗ** entry pending thay vì bỏ qua
  (bài học từ bot thực tế: hoa hồng "đóng băng" ở 0đ nếu chỉ insert-new).
- Một số campaign (VD Citibank) tracking click/đơn trực tiếp trên site
  advertiser — publisher không xem được click realtime qua API; AT update
  tay mỗi tuần. Thiết kế sync phải chịu được merchant "mù số liệu".
- Cookie duration/policy nằm trong response campaign — dùng để giải thích
  tại sao đơn không được ghi nhận (ghi đè cookie).

### 1.4. Đánh giá cho automation
- ✅ API chính thức, đầy đủ 4 mảnh cần thiết: campaign → deeplink → datafeed
  sản phẩm → đối soát đơn/hoa hồng. Không cần cào web.
- ⚠️ Không sandbox → mọi test chạm dữ liệu thật (chỉ đọc thì an toàn).
- ⚠️ Rate limit thấp ở order-list → sync theo lịch thưa (VD 15–30 phút/lần),
  cache kết quả.
- ❓ Chưa rõ giới hạn campaign approval tự động — nhiều campaign cần đăng ký
  và chờ duyệt tay (5 phút → vài ngày). App phải xử lý trạng thái
  `approval=pending/unregistered` và chỉ tạo link cho `successful`.

---

## 2. Tải video Douyin tự động

### 2.1. yt-dlp (khuyến nghị chính)
- yt-dlp có extractor Douyin, hỗ trợ video có/không watermark tùy phiên bản
  và thời điểm (Douyin đổi chữ ký liên tục).
- Vấn đề thực tế 2026: Douyin có **browser fingerprint + X-Gorgon/X-Khronos
  signature** — cookie export thường thiếu signature → yt-dlp báo
  "Fresh cookies needed". Khắc phục: `--cookies-from-browser` tươi, hoặc
  Playwright headless (nặng, không hợp pipeline Go).
- Độ tin cậy automation: **trung bình-khá** nếu giữ yt-dlp luôn mới nhất
  (Douyin xoay chữ ký → extractor gãy định kỳ vài tuần/lần). Cần health
  check + auto-update yt-dlp trong app (tải binary từ GitHub release).
- Chất lượng: tới 1080p khi lấy được bản không watermark.

### 2.2. TikWM API (fallback)
- `https://www.tikwm.com/api/?url=<encoded>` — public, không cần key,
  trả `play` (no-watermark MP4), `hdplay`, `wmplay`, metadata
  (duration, play_count, digg_count, author, title).
- **Còn hoạt động 2026**: nhiều project (4 ngày – 2 tháng trước) vẫn dùng
  làm fallback chính cho TikTok/Douyin; có project báo success ~98%.
- Rủi ro: bên thứ ba không rõ operator — **có thể die bất cứ lúc nào**,
  không SLA. Link CDN có chữ ký, hết hạn nhanh (vài phút) → phải tải ngay.
- Dùng làm fallback sau yt-dlp, không phải nguồn chính duy nhất.

### 2.3. API trả phí bên thứ ba (dự phòng, không bắt buộc)
- JustOneAPI (`/api/douyin/get-user-video-list/v3` — cần token trả phí),
  TikHub, SocialCrawl (`/v1/douyin/search`, tính credit theo row).
- Chỉ cân nhắc khi 2 nguồn trên cùng gãy; tốn tiền, đi ngược tinh thần free.

### 2.4. Douyin Open Platform (chính thức — LOẠI)
- `open.douyin.com` yêu cầu đăng ký app, xét duyệt, thường đòi pháp nhân
  Trung Quốc. Không khả thi cho automation cá nhân của Ninh. Loại.

### 2.5. Chiến lược tải khuyến nghị
1. **yt-dlp** (primary) — binary do app tự quản, auto-update, thử no-watermark.
2. **TikWM** (fallback) — khi yt-dlp fail, tải ngay link `play`.
3. Mỗi video lưu: `source_url`, `douyin_id`, `author`, `caption`, `duration`,
   `downloaded_at`, `watermark_free` (bool), `sha256` (chống tải trùng).
4. Health check mỗi giờ: thử tải 1 URL mẫu; fail → cảnh báo + chuyển nguồn.

---

## 3. Transform chống bản quyền (trung thực)

### 3.1. Bối cảnh 2026 — hàng rào đang cao dần
- **TikTok (01/2026)**: siết "Unoriginal Content" — AI kiểm duyệt mạnh hơn
  ~10x theo báo cáo cộng đồng creator; nội dung repost/không gốc bị **không
  monetization + không lên FYP**. Không chỉ là Content ID, mà là penalty
  phân phối.
- **YouTube Shorts (10/2026)**: chính thức ưu tiên nội dung gốc, giảm phân
  phối video re-upload "không thêm giá trị có ý nghĩa"; **sửa kỹ thuật nhỏ
  (minor edits) bị nêu đích danh là không đủ**.
- Kết luận: thời "crop + mirror là qua" đã qua. Transform kỹ thuật chỉ là
  lớp 1; **lớp transformative (bình luận, biên tập, kể chuyện) mới là thứ
  nền tảng 2026 đòi hỏi.**

### 3.2. Bảng kỹ thuật (tất cả tự động được bằng FFmpeg/Go)

| # | Kỹ thuật | FFmpeg | Hiệu quả 2026 | Ghi chú |
|---|---|---|---|---|
| T1 | Crop/reframe (VD 9:16 → zoom 105–110% + pan nhẹ) | `crop`/`scale` | Thấp–trung bình | Perceptual hash hiện đại chịu được crop nhẹ |
| T2 | Mirror ngang | `hflip` | Thấp | Dễ bị phát hiện; chỉ hợp cảnh đối xứng |
| T3 | Đổi tốc độ ±4–8% | `setpts`/`atempo` | Trung bình | Vượt qua audio fingerprint cũ; video hash vẫn bắt được |
| T4 | Grade màu (contrast/sat/warm shift) | `eq`/`colorbalance` | Thấp–trung bình | Thay đổi pixel nhưng không thay đổi cấu trúc |
| T5 | Zoom động (Ken Burns) | `zoompan` | Trung bình | Tốt hơn crop tĩnh; đã có sẵn trong studio |
| T6 | Overlay (viền, khung, logo, chữ) | `overlay`/`drawtext` | Thấp | Chỉ che hash ở vùng phủ; nền tảng crop bỏ qua |
| T7 | Thay audio / đổi nhạc | `amix`/thay track | Trung bình–cao (cho audio ID) | Mất nhạc gốc viral = mất một phần sức hút |
| T8 | **Voiceover/bình luận tiếng Việt** (TTS hoặc VieNeu) | TTS chain có sẵn | **Cao nhất** | Thêm lớp transformative thật: kể chuyện, reaction, giải thích tình tiết. Vừa chống audio-ID vừa tăng "original value" theo tiêu chí 2026 |
| T9 | Compilation/mashup (3–5 clip + chuyển cảnh + intro/outro riêng) | `xfade`/`concat` | Cao | Cấu trúc mới, khó hash; tốn công biên tập tự động |
| T10 | Cắt/gộp lại nhịp (jump-cut theo beat, đảo thứ tự cảnh) | `select`/`concat` | Trung bình–cao | Phá vỡ fingerprint theo thời gian |

### 3.3. Rủi ro còn lại (phải nói rõ với Ninh)
1. **Không có combo nào đảm bảo 100%.** Kẻ tấn công (nền tảng) luôn mạnh
   dần; hôm nay qua, mai có thể bị quét lại (retroactive claim).
2. **Penalty phân phối > copyright strike**: với reup, nguy hiểm lớn nhất
   2026 không phải strike mà là **video 0 view** (TikTok unoriginal flag,
   YouTube Shorts de-boost). Không có cách code nào "đảm bảo lên xu hướng".
3. **Nhạc**: Douyin dùng nhạc bản quyền TQ; reup sang TikTok VN với nhạc
   gốc = audio-ID bắt ngay. T8/T7 là bắt buộc, không tùy chọn.
4. **Tài khoản là tài sản khan hiếm**: reup hàng loạt trên 1 account mới
   = rủi ro ban cao. Cần giới hạn tần suất, warm-up account, mỗi account
   một "phong cách transform" khác nhau (tránh fingerprint theo uploader).
5. **Pháp lý**: reup = vi phạm bản quyền nguyên tắc, dù transform. Ninh
   cần biết và chấp nhận rủi ro này trước khi chạy.

### 3.4. Công thức transform khuyến nghị (tự động, theo độ mạnh)
- **Mức 1 (mặc định mọi video)**: T5 zoom động + T3 tốc độ ±5% + T4 grade
  nhẹ + T8 voiceover tiếng Việt (bình luận/tóm tắt tình tiết, TTS VieNeu)
  + T7 nhạc nền thay bằng nhạc licensed.
- **Mức 2 (video hot, đầu tư hơn)**: Mức 1 + T9 compilation 3 clip cùng
  chủ đề + intro/outro + caption tiếng Việt.
- Tuyệt đối không reup nguyên bản (0 transform) — vừa vi phạm policy nền
  tảng 2026 vừa lãng phí account.

---

## 4. Kiến trúc đề xuất trong app Go

### 4.1. Pipeline A — Affiliate Accesstrade (package mới `internal/accesstrade`)

```
Scheduler (daily) → ATClient (campaigns/datafeeds) → ProductRank
→ products.Store (Source="accesstrade") → studio affiliate render
→ publishers (TikTok/YouTube/FB) → order-list sync → ledger đối soát
```

- **Package mới** `internal/accesstrade/`: `client.go` (Token auth, retry,
  rate-limit 10 req/min cho order-list), `campaigns.go`, `datafeeds.go`,
  `links.go` (product_link/create + utm/sub per account), `orders.go`.
- **Tái dùng**: `internal/products` (Product/Query/Score — thêm
  `Source="accesstrade"`, map `aff_link` → ProductURL, `image` → ImageURLs);
  `internal/studio` affiliate flow (đã có: ảnh sản phẩm + nhạc, không chữ
  không voiceover — đúng format Ninh chốt); `internal/publishers`
  (TikTok draft-first, YouTube private-first); ledger đối soát.
- **Bảng DB mới** (SQLite): `at_campaigns` (cache campaign + approval),
  `at_links` (link đã tạo per product/account), `at_orders` (raw order-list,
  upsert theo order_id + update pending), `at_sync_state` (watermark).
- **UI**: Cài đặt → tab "Accesstrade" (nhập access_key, test, trạng thái);
  Sản phẩm → nguồn "Accesstrade" trong hunter; đối soát hoa hồng ở dashboard.
- **Automation**: hunter chạy daily (datafeeds sort HIGH_COMMISSION_RATE);
  order sync 30 phút/lần; campaign approval check daily (pending → báo).

### 4.2. Pipeline B — Reup Douyin (package mới `internal/reup`)

```
Scheduler → DouyinSource (discover: hashtag/user viral) → Downloader
(yt-dlp → TikWM fallback) → Transform (T1–T10 theo mức) → QC (độ dài,
độ phân giải, audio) → publishers → theo dõi view/flag → kill nếu 0-view
liên tục
```

- **Package mới** `internal/reup/`: `discover.go` (danh sách nguồn: user/
  hashtag Douyin cấu hình tay — Ninh nhập 1 lần; auto-pick video viral
  theo play_count qua TikWM metadata), `download.go` (yt-dlp binary quản
  lý bởi app + TikWM fallback), `transform.go` (FFmpeg filter chains theo
  mức 1/2 + voiceover TTS VieNeu), `source.go` (dedupe sha256).
- **Tái dùng**: `internal/studio` (assemble/concat/xfade, music mux —
  tách hàm dùng chung, không copy); `internal/engines/tts` (VieNeu local
  cho voiceover); `internal/publishers`; `internal/growth` (kill rule cho
  video 0-view: ≥N video + <X view → dừng nguồn/đổi transform).
- **Bảng DB mới**: `reup_sources` (douyin user/hashtag + cấu hình),
  `reup_videos` (metadata gốc + transform đã áp + fingerprint),
  `reup_posts` (post per account/platform + metrics).
- **UI**: trang "Reup" mới: nguồn Douyin, hàng đợi video, xem trước
  before/after transform, toggle mức transform, metrics + kill switch
  riêng cho pipeline reup.
- **Automation**: discover 6h/lần → download → transform → xếp hàng đăng
  theo lịch mỗi account (tối đa X video/ngày/account, warm-up account mới).

### 4.3. Điểm chung 2 pipeline
- Fail-closed mọi nơi: thiếu key AT → hunter dừng + báo; yt-dlp gãy →
  TikWM → cả hai gãy → job failed + cảnh báo (không bịa video).
- Mỗi account một fingerprint transform (seed riêng) — chống liên đới.
- Ledger ghi mọi quyết định (ai post gì, transform nào, link nào).

---

## 5. Rủi ro trung thực (tổng hợp)

1. **Reup không bao giờ an toàn 100%** — transform giảm rủi ro, không xóa rủi ro.
2. **Nguy cơ lớn nhất là 0 view / de-boost** (policy 2026), không phải strike.
3. **Account ban**: reup là hành vi rủi ro cao; warm-up + giới hạn tần suất + mỗi account một phong cách.
4. **Accesstrade**: không sandbox (test bằng key thật); order-list rate limit thấp; campaign cần duyệt tay; commission hold → 0đ tạm thời.
5. **yt-dlp gãy định kỳ** theo chữ ký Douyin — cần auto-update + fallback TikWM (bên thứ ba, có thể die).
6. **Nhạc Douyin** bản quyền — bắt buộc thay audio/voiceover.
7. **Pháp lý**: reup vi phạm bản quyền nguyên tắc — Ninh chấp nhận rủi ro.

---

## 6. Thứ tự implement khuyến nghị

1. **AT client + campaign/link** (`internal/accesstrade`, 1–2 ngày): auth,
   campaigns, product_link/create, UI nhập key. Giá trị ngay: tạo link tay
   nhanh hơn vào web.
2. **AT datafeed hunter** (1 ngày): thay TikTok Shop hunter bằng AT datafeed
   (sort HIGH_COMMISSION_RATE) → vào products.Store hiện có.
3. **AT order sync + đối soát** (1 ngày): order-list + upsert pending +
   dashboard hoa hồng.
4. **Reup downloader** (`internal/reup`, 2–3 ngày): yt-dlp manager +
   TikWM fallback + dedupe + UI nguồn/hàng đợi.
5. **Transform mức 1** (2 ngày): zoom động + tốc độ + grade + voiceover
   VieNeu + thay nhạc — tái dùng studio assemble.
6. **Transform mức 2 + QC + kill rule** (2–3 ngày): compilation, metrics,
   dừng nguồn 0-view.
7. Cuối: review vòng (build/test/preview) rồi mới bật automation thật.

*Tổng ước tính: ~2 tuần làm việc theo đợt, mỗi đợt nghiệm thu độc lập.*
