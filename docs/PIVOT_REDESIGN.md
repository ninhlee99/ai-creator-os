# PIVOT REDESIGN — Thiết kế lại toàn app theo 3 trụ mới

Ngày: 2026-10-02 · Người chốt: Ninh · Trạng thái: **thiết kế, chưa code**

## 0. Quyết định phạm vi

**TẬP TRUNG (3 trụ):**
1. **Affiliate qua Accesstrade** — tự lấy chiến dịch/sản phẩm/link/đối soát hoa hồng
2. **Reup video Douyin** — tự tải video viral → transform chống bản quyền + voiceover Việt → đăng
3. **YouTube kể chuyện** — truyện ngôi thứ nhất + ảnh minh họa từng cảnh + giọng đọc (KHÔNG quay video)

**BỎ:**
- Live (Lịch live, live scheduler, streamer, avatar live, tab Nhân vật AI)
- Phim điện ảnh (film mode 30–60 phút trong Studio, Veo, realism upgrades)

**Cách bỏ:** park bằng build tag `parked` (mẫu đã có từ R2-W6) — code nằm im,
không biên dịch vào binary, không xóa vội. Xóa hẳn chỉ khi Ninh yêu cầu sau.

**Nguyên tắc giữ nguyên:** zero-touch (Ninh không can thiệp), fail-closed,
mỗi khả năng có UI/UX, ít chữ + card/modal/toast, màu xám ấm + chàm–slate,
1 binary Go duy nhất, key free-only.

---

## 1. Sidebar mới (9 → 8 mục)

| # | Mục mới | Từ | Vai trò mới |
|---|---|---|---|
| 1 | **Trang chủ** | `/` | Dashboard 3 trụ: doanh thu AT hôm nay, đơn hàng, video đã đăng theo trụ, view, cảnh báo (hết quota, transform fail, kênh bị flag). Kill switch giữ nguyên |
| 2 | **Affiliate** | `/products` cải tiến sâu | Tab: Chiến dịch AT (approval) · Sản phẩm (sắp xếp theo hoa hồng) · Link đã tạo · Đối soát đơn/hoa hồng |
| 3 | **Reup** | **mới** `/reup` | Nguồn Douyin (user/hashtag) · Hàng đợi tải · Xem trước before/after transform · Metrics từng video · Kill switch riêng pipeline |
| 4 | **Kể chuyện** | **mới** `/stories` (tách từ studio) | Danh sách truyện · Tạo truyện (chủ đề → truyện ngôi thứ nhất) · Storyboard ảnh · Nghe thử giọng đọc · Video đã dựng · Lịch đăng YouTube |
| 5 | **Studio AI** | `/studio` thu gọn | Chỉ 2 mode: **Video affiliate** (ảnh sản phẩm + nhạc, không chữ không voiceover — giữ nguyên format Ninh chốt) và **Video kể chuyện** (truyện + ảnh + giọng đọc). BỎ: film dài, Veo. GIỮ: Nhạc thịnh hành (dùng cho reup/affiliate) |
| 6 | **Kênh** | `/accounts` đổi tên | Quản lý kênh TikTok/YouTube/Facebook · trạng thái warm-up · fingerprint transform riêng mỗi kênh · OAuth |
| 7 | **Phát triển** | `/growth` | Giữ nguyên + thêm **kill rule reup**: N video liên tiếp 0-view → dừng nguồn / đổi phong cách transform + báo |
| 8 | **Cài đặt** | `/settings` | Tabs: Hệ thống · **Accesstrade** (mới: nhập access_key, test, trạng thái) · **Reup** (mới: yt-dlp, mức transform, giới hạn/ngày) · Nhà cung cấp · Model local · An toàn. BỎ tab Nhân vật AI (park theo avatar live) |

**Bỏ hẳn khỏi sidebar:** Lịch live (`/schedule` → park), Agent Team dạng live.

### Về trang Agent Team (`/team`)
Không xóa — **định nghĩa lại**: thay vì team phục vụ live, mỗi pipeline có một
team chuyên trách hiển thị dạng cây:
- **Affiliate team**: Hunter (quét AT datafeed) → Producer (render video) → Publisher (đăng) → Analyst (đối soát hoa hồng)
- **Reup team**: Hunter (tìm video viral) → Director (kế hoạch transform + voiceover) → Producer (FFmpeg) → QC → Publisher → Analyst (view/flag)
- **Story team**: Writer (viết truyện) → Illustrator (vẽ ảnh) → Voice (giọng đọc) → Producer → Publisher
- Governance/Scheduler gác cổng giữ nguyên. Đây vẫn là "linh hồn" kiến trúc, chỉ đổi nhiệm vụ.

---

## 2. Thay đổi từng tính năng (feature-by-feature)

### 2.1. Trang chủ — dashboard 3 trụ
- Bỏ: thẻ live sắp tới, lịch live, avatar status.
- Thêm: thẻ **Doanh thu affiliate** (hôm nay / 7 ngày, từ AT order sync), thẻ
  **Đơn hàng** (pending/approved), thẻ **Reup** (video đã đăng, tổng view, video
  0-view), thẻ **Kể chuyện** (truyện đã xuất bản, giờ xem), thẻ **Cảnh báo**
  (quota key, transform fail, kênh bị flag, campaign pending duyệt).
- Khôi phục đúng tinh thần "mở app là biết hôm nay kiếm được bao nhiêu".

### 2.2. Affiliate (từ /products)
- Nguồn sản phẩm duy nhất: **Accesstrade datafeed** (bỏ TikTok Shop hunter).
  Giữ form thêm tay (dự phòng).
- Mỗi sản phẩm: ảnh, giá, % hoa hồng, **link AT đã tạo** (tự động, utm/sub
  theo kênh), nút copy.
- Đối soát: đơn từ `/v1/order-list` sync 30 phút/lần, upsert tại chỗ
  (commission hold → 0đ rồi update sau — đã ghi trong research).
- Autopilot: khi bật, tự tạo video affiliate cho sản phẩm hoa hồng cao
  (Studio mode affiliate), tự đăng theo lịch kênh.

### 2.3. Reup (mới)
- Nguồn: Ninh nhập 1 lần danh sách user/hashtag Douyin (thần tiên, tỉ tỉ...).
  App tự phát hiện video viral (theo play_count qua TikWM metadata).
- Tải: yt-dlp (tự quản binary, auto-update) → TikWM fallback. Dedupe sha256.
- Transform 2 mức (theo research):
  - **Mức 1** (mặc định): zoom động + tốc độ ±5% + grade nhẹ + **voiceover
    bình luận tiếng Việt** (TTS) + nhạc licensed thay nhạc gốc.
  - **Mức 2**: compilation 3 clip cùng chủ đề + intro/outro + caption Việt.
- Tuyệt đối không đăng nguyên bản. Mỗi kênh một seed transform riêng.
- Metrics + kill: video 0-view liên tiếp → dừng nguồn, báo Ninh.

### 2.4. Kể chuyện (mới, tách từ Studio)
- Tạo truyện: nhập chủ đề → AI viết **truyện ngôi thứ nhất** (tái dùng
  `prompts/story.txt` của director cũ — viết truyện là piece giữ lại).
- Storyboard: mỗi đoạn truyện → 1 ảnh minh họa (Gemini image, key free) +
  giọng đọc TTS (ngôi thứ nhất).
- Dựng: ảnh + giọng đọc + nhạc nền nhẹ + phụ đề (tái dùng assemble của
  studio, bỏ letterbox/xfade điện ảnh — chỉ cần chuyển cảnh đơn giản).
- Đăng YouTube: private-first → public theo lịch; Shorts cắt từ đoạn hay
  (tái dùng logic trailer).
- **Không dùng**: Veo, cinematic zoompan nâng cao, character bible, lip-sync.

### 2.5. Studio AI (thu gọn)
- Giữ 2 job kind: `affiliate` và `story`. Xóa/bỏ park: `film` (phim dài),
  `livestream` (nếu có).
- Form tạo video affiliate giữ nguyên (ảnh + nhạc, không chữ không voiceover).
- Trends (nhạc thịnh hành) giữ — dùng chung cho affiliate + reup.

### 2.6. Kênh (từ /accounts)
- Đổi tên + thêm: trạng thái warm-up (kênh mới), giới hạn video/ngày/kênh,
  fingerprint transform đang dùng, lịch sử flag/strike.
- Lịch đăng (posting calendar) chuyển về đây từ /schedule cũ — mỗi kênh có
  khung giờ đăng riêng cho từng trụ.

### 2.7. Phát triển kênh (growth)
- Giữ plan engine, kill/double-down cho affiliate.
- Thêm: **kill rule reup** — nguồn nào cho ra N video 0-view liên tiếp thì
  pause nguồn + đề xuất đổi transform; breakout rule cho video viral
  (làm tiếp cùng chủ đề/phong cách).

### 2.8. Cài đặt
- Tab Accesstrade: nhập `access_key`, nút test kết nối, hiện trạng thái +
  số campaign đã duyệt.
- Tab Reup: cấu hình yt-dlp (auto-update), mức transform mặc định, số
  video tối đa/ngày/kênh, toggle voiceover.
- Bỏ: tab Nhân vật AI. Giữ: Hệ thống, Nhà cung cấp, Model local, An toàn.

### 2.9. Giữ nguyên không đổi
- Backup/restore, API nội bộ (`/api/*`, mặc định tắt), kill switch,
  dry-run, master switch, key rotation, onboarding wizard (cập nhật nội
  dung theo 3 trụ mới), đa nền tảng publishers (TikTok/YouTube/FB).

---

## 3. Kiến trúc code

### 3.1. Park (build tag `parked`, không xóa)
- `internal/stream/` (live engine), `internal/engines/avatar/` (avatar +
  MuseTalk lip-sync — gắn với live/phim)
- Film trong `internal/studio`: tách file — `film_*.go`, `cinematic.go`,
  `capabilities.go` (probe Veo), `prompts/{screenplay,breakdown,film_keyframe}.txt`
  → parked. **GIỮ**: `prompts/story.txt` (viết truyện cho trụ Kể chuyện),
  affiliate flow, assemble/concat, TTS, trends.
- `internal/web`: handlers `/schedule`, `/team` (bản live), tab Nhân vật AI.

### 3.2. Package mới
- `internal/accesstrade/` — client (Token auth), campaigns, datafeeds,
  links (tạo deeplink + utm/sub), orders (sync + upsert).
- `internal/reup/` — discover (TikWM metadata), download (yt-dlp manager +
  TikWM fallback), transform (filter chains 2 mức + voiceover), dedupe.
- `internal/stories/` — story jobs (tái dùng story.txt + image gen + TTS +
  assemble đơn giản). (Cân nhắc: đặt trong studio như kind mới — quyết
  khi code, ưu tiên ít package mới.)

### 3.3. Tái dùng (không viết lại)
- `internal/products` — thêm `Source="accesstrade"`, map aff_link/ảnh/giá.
- `internal/publishers` — TikTok draft-first, YouTube private-first (giữ).
- `internal/growth` — kill/breakout rules (mở rộng cho reup).
- `internal/ledger` — đối soát hoa hồng AT (mở rộng từ reconcile hiện có).
- `internal/engines/tts` — VieNeu local cho voiceover reup + giọng kể chuyện.
- `internal/studio` — affiliate render, assemble/concat/xfade, trends.

### 3.4. Bảng DB mới (SQLite)
- `at_campaigns`, `at_links`, `at_orders`, `at_sync_state`
- `reup_sources`, `reup_videos`, `reup_posts`
- `stories`, `story_assets` (hoặc dùng `studio_jobs` với kind=`story`)

---

## 4. Automation (zero-touch, Ninh không chạm)

| Tick | Tần suất | Việc |
|---|---|---|
| AT hunter | daily | Quét datafeed sort HIGH_COMMISSION_RATE → products |
| AT order sync | 30 phút | order-list (tôn trọng 10 req/phút) → đối soát |
| AT campaign check | daily | Phát hiện campaign pending → báo |
| Reup discover | 6 giờ | Tìm video viral từ nguồn Douyin |
| Reup render+post | theo lịch kênh | download → transform → đăng (giới hạn/ngày) |
| Story generator | weekly (cấu hình được) | viết truyện → vẽ ảnh → giọng đọc → dựng → xếp lịch YT |
| Growth tick | hourly | metrics + kill rule (affiliate + reup) |

Tất cả sau kill switch + dry-run + master switch như hiện tại.

---

## 5. UI/UX

- Sidebar 8 mục theo §1. Mỗi trụ có màu nhận diện nhẹ (không chói):
  affiliate = chàm, reup = slate, kể chuyện = nâu ấm — chỉ dùng ở badge/icon.
- Dashboard: số liệu trước, chữ ít. Chi tiết trong modal.
- Trang Reup: xem trước before/after transform (2 video cạnh nhau) trước khi
  duyệt đăng lô đầu — sau đó full auto.
- Mọi trang mới đều có trong preview Vercel trước khi Ninh dùng thật.

---

## 6. Lộ trình implement (mỗi đợt commit + push + preview riêng)

1. **Đợt A — Park & dọn**: park live + film, sidebar 8 mục, dashboard mới.
2. **Đợt B — Accesstrade core**: client + UI key + campaigns + tạo link.
3. **Đợt C — Affiliate pipeline**: datafeed hunter + Studio video + order sync + đối soát.
4. **Đợt D — Reup core**: download (yt-dlp + TikWM) + UI nguồn/hàng đợi.
5. **Đợt E — Transform**: mức 1 (voiceover + nhạc) → mức 2 (compilation) + kill rule.
6. **Đợt F — Kể chuyện**: stories pipeline + đăng YouTube.
7. **Đợt G — Review vòng**: build/test/preview toàn app, bật automation.

---

## 7. Rủi ro & trung thực

- Reup không bao giờ an toàn 100% (đã nói với Ninh 2026-10-02).
- yt-dlp gãy định kỳ → auto-update + TikWM fallback (không SLA).
- AT không sandbox, order-list 10 req/phút, campaign cần duyệt tay.
- Key free quota: reup voiceover dùng VieNeu local (không tốn key);
  kể chuyện tốn image gen + TTS theo quota free (đã có keyring + resume).
