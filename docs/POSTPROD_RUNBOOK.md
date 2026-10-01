# Runbook hậu kỳ: TikTok + CapCut + Google Drive (Mac của Ninh)

Việc **thủ công, ngoài app `aicos`**: Claude/Ninh làm trên máy Mac (M1 Pro, 32GB).
App chỉ dùng các API đã setting sẵn. Không dùng Claude/Codex/Agy để làm việc
*bên trong* app hay điều khiển máy thay app.

## App đã tự làm gì (đừng làm trùng)

- Tìm/lưu sản phẩm hoa hồng cao (trang **Sản phẩm**; thêm tay khi TikTok Shop API chưa nối).
- Sinh video affiliate "giật giật": 3–5 ảnh mẫu + sản phẩm thật, zoom nảy theo nhịp, cắt cứng, 1080×1920, 30fps, ~30s.
- Viết **caption + hashtag** tiếng Việt (Studio → Xem storyboard).
- **Tự đăng TikTok** khi bật "Tự đăng TikTok khi video xong" (trang Sản phẩm). Video lên dạng **nháp/inbox**, không public.

Việc của người vận hành (app **không** làm được):

| # | Việc | Tần suất | Ai làm |
|---|---|---|---|
| 1 | Tạo app trên developers.tiktok.com + OAuth | 1 lần | Ninh (cần đăng nhập + điền form của TikTok) |
| 2 | Gắn giỏ hàng + sound + đăng | mỗi video | Ninh, trên điện thoại (API không cho gắn giỏ hàng) |
| 3 | Tinh chỉnh CapCut + export | mỗi video, khi cần | Ninh/Claude |
| 4 | QC + upload Google Drive | mỗi video | `scripts/postprod.sh` |

## Vị trí file

- Thư mục app: `~/ninhlee/apps/ai-creator-os` (chạy binary trong thư mục này. Token và `data/` nằm cạnh nó).
- Video app xuất: `data/output/studio-<job-id>.mp4`.
- Ảnh master 4K: `data/output/studio/<job-id>/photo-NN-4k.png`.
- Caption: bảng `studio_jobs` trong `data/studio.db` (script tự đọc).
- `scripts/postprod.sh latest` in ra video mới nhất, job id, ảnh 4K và caption.

---

## PHẦN 1: Kết nối TikTok (một lần mỗi tài khoản)

### 1.1 Tạo app trên TikTok Developers (Ninh làm, cần trình duyệt)

App chỉ chạy local. TikTok bắt redirect https, nên redirect về trang trung chuyển trên GitHub Pages
và trang đó tự chuyển về app ở `127.0.0.1:8080`. App luôn dùng PKCE (bắt buộc với Desktop).

0. Terms/Privacy URL thì TikTok vẫn bắt buộc phải là link công khai (link trong repo private sẽ 404). Không cần website, chọn 1 trong 2:
   - **GitHub Gist:** gist.github.com → tên file `terms.md` → dán `docs/tiktok-app-site/terms.md` (thay `CONTACT_EMAIL`)
     → **Create public gist** → copy URL. Làm lại với `privacy.md`.
   - **Google Docs:** dán nội dung vào doc → File → Share → **Publish to web** → copy link `…/pub`.
1. **developers.tiktok.com → Manage apps → Connect an app**, điền:
   - Category: `Others`
   - Description: `Personal tool that uploads a creator's own AI-made videos to their TikTok inbox as drafts for review before posting.`
   - Terms of Service URL / Privacy Policy URL: 2 link Google Docs ở bước 0.
   - Platforms: chỉ tick **Desktop** (bỏ Web, bỏ Android/iOS).
2. Add products: **Login Kit** + **Content Posting API** (Direct Post để TẮT). Ghi lại **Client Key** + **Client Secret**.
3. Login Kit → **Redirect URI**: `https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html`
   (TikTok bắt buộc https). Trang này là trang trung chuyển tĩnh (`docs/tiktok-app-site/callback.html`):
   nhận `code` rồi tự chuyển tiếp về app local `http://127.0.0.1:8080/publishers/tiktok/callback`.
   Nhớ thêm vào `.env`: `TIKTOK_REDIRECT_URI=https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html`.
   Web/Desktop URL: `https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/`.
4. Scopes: `user.info.basic`, `video.upload`. Chỉ thêm `video.publish` khi muốn Direct Post (`TIKTOK_DRAFT_ONLY=0`).
5. Bật **Sandbox** → thêm tài khoản creator làm **Target user**. Dùng được ngay, không chờ audit.
   Muốn ra production: app audit mất ~1–2 tuần, Direct Post audit ~5–10 ngày làm việc. Đây là giới hạn của TikTok.

### 1.2 Khai báo key cho app

Không gõ secret thẳng vào Terminal (sẽ nằm trong lịch sử shell). Ghi vào `.env`
(đã gitignore) ở thư mục app:

```bash
cd ~/ninhlee/apps/ai-creator-os
cat >> .env <<'EOF'
TIKTOK_CLIENT_KEY=...
TIKTOK_CLIENT_SECRET=...
TIKTOK_REDIRECT_URI=https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html
TIKTOK_DRAFT_ONLY=1
EOF
chmod 600 .env
```

Nhiều tài khoản dùng app TikTok khác nhau: thêm hậu tố là username **viết HOA, ký tự
không phải chữ/số đổi thành `_`**, ví dụ `shop.thoitrang` → `TIKTOK_CLIENT_KEY_SHOP_THOITRANG`.
Bản không hậu tố dùng chung cho mọi tài khoản.

Chạy app (app không tự đọc `.env`, phải nạp vào shell):

```bash
go build -o aicos-darwin-arm64 ./cmd/aicos   # nếu chưa có binary
set -a; source .env; set +a; ./aicos-darwin-arm64
```

### 1.3 OAuth bằng nút trong app (không cần curl)

1. Mở app đúng địa chỉ trong Redirect URI: `http://127.0.0.1:8080/publishers`.
   Dòng tài khoản có nút **Kết nối TikTok** (thấy chữ "thiếu client key" là bước 1.2 chưa đúng).
2. Bấm nút, trang TikTok mở bằng phiên **đã đăng nhập sẵn** trên trình duyệt → kiểm tra đúng tài khoản creator → **Allow**.
3. TikTok → trang trung chuyển → tự về app → dòng xanh "Đã kết nối TikTok…" và cột TikTok token chuyển **Có**. Không phải copy gì.
   App ghi `tiktok_token_<username>.json` (quyền 600). Tên file giữ nguyên hoa/thường, ký tự đặc biệt
   đổi thành `_`, ví dụ `shop.vn` → `tiktok_token_shop_vn.json`. Token không bao giờ hiện lên trang hay log.
4. Lỗi `redirect_uri` mismatch → `TIKTOK_REDIRECT_URI` trong `.env` khác Redirect URI trên portal.
   Trang trung chuyển không tự về app (app chạy cổng khác 8080) → copy URL, dán vào ô "Lưu token" trên trang Đa nền tảng.

### 1.4 Bật tự đăng + test

1. Trang **Sản phẩm** → bật **"Tự đăng TikTok khi video xong"** → Lưu lịch.
2. Chạy autopilot 1 lần cho 1 tài khoản → log job phải có dòng
   `Đã đăng lên TikTok dưới dạng NHÁP (draft id …)`.
3. Điện thoại: TikTok báo vào **Hộp thư** ("video của bạn đã sẵn sàng để chỉnh sửa"). Mở từ thông báo đó.
4. Log "Tự đăng thất bại: …":
   - `no refresh_token` / `invalid_grant` → token hỏng/hết hạn (refresh token sống 365 ngày) → bấm **Kết nối lại**.
   - `spam_risk_too_many_pending_share` → quá nhiều nháp chưa xử lý trong inbox → đăng/xóa bớt nháp.
   - `unaudited_client_can_only_post_to_private_accounts` → app chưa audit: tài khoản phải để private hoặc là test user.

---

## PHẦN 2: Gắn giỏ hàng + đăng (mỗi video, BẮT BUỘC làm tay)

API TikTok không cho gắn sản phẩm qua code.

1. Mở nháp từ thông báo Hộp thư (bước 1.4.3).
2. **Thêm liên kết → Sản phẩm** → tìm đúng sản phẩm affiliate → gắn.
3. Đổi nhạc sang **sound trending** trong thư viện TikTok nếu bản nháp chưa dùng sound trend.
4. Dán caption: Studio → Xem storyboard → Copy caption (hoặc `caption.txt` trong folder Drive).
5. Kiểm tra lần cuối (mặt mẫu, tay, sản phẩm đúng) → **Đăng**.

Claude **không** tự bấm đăng ở bất kỳ nền tảng nào.

---

## PHẦN 3: CapCut (chỉ khi cần tinh chỉnh)

Video từ app đã có giật-giật + nhạc. Chỉ mở CapCut (`/Applications/CapCut.app`) khi cần căn lại beat hoặc thay sound.

1. **Chuẩn bị:** `scripts/postprod.sh latest` → lấy video. Chạy `scripts/postprod.sh qc <video>` để xem thông số gốc.
   Xem 1 lượt: số ảnh, BPM, chỗ cắt chưa khớp nhịp.
2. **Edit:** project mới 9:16, import video.
   - Snap điểm cắt vào từng nhịp trống (Beat → Auto beat).
   - Đoạn chưa đủ "giật": thêm **Shake** / **Camera in-out** đúng nhịp mạnh, hoặc **Flash** trắng 2–3 frame ở điểm chuyển ảnh.
   - 2–3 giây đầu (hook) phải bùng nổ nhất: ảnh hook + nhịp trống đầu đập cùng lúc.
   - KHÔNG thêm chữ, voiceover, sticker.
3. **Nhạc:** Audio → Music, sound đang trend VN (tag trending / lượt dùng cao). Chỉ dùng nhạc trong thư viện CapCut/TikTok.
   Không chắc bản quyền thì hỏi Ninh. Nhạc rõ, không rè, không cụt đầu/cuối.
4. **Export:** MP4, 1080×1920, 30fps, bitrate ≥ 12 Mbps (CapCut: Bitrate → Custom/High).
   Tên file: `<YYYY-MM-DD>_<ten-san-pham>_final.mp4` (script cũng tự đặt tên đúng khi upload).

## PHẦN 4: QC

```bash
scripts/postprod.sh qc ~/Movies/CapCut/…/export.mp4 --final
```

Script tự kiểm (FAIL thì sửa trong CapCut, không bỏ qua):
1080×1920 · 30fps · 25–35s · không rơi frame · bitrate ≥ 12 Mbps · có nhạc · nhạc phủ hết video (không cụt) · đỉnh âm < −0.1 dB (không rè) · âm lượng không quá nhỏ.

Người xem phải tự tick (máy không chấm được):
- [ ] Mặt mẫu đồng nhất suốt video
- [ ] Tay tự nhiên, không dị dạng
- [ ] Sản phẩm đúng mẫu, không méo
- [ ] Không chữ / watermark / logo lạ
- [ ] Nhạc khớp nhịp cắt

## PHẦN 5: Upload Google Drive

Drive for desktop đã đăng nhập (`~/Library/CloudStorage/GoogleDrive-…/My Drive`). Không thấy thư mục này thì DỪNG, báo Ninh đăng nhập.

```bash
scripts/postprod.sh pack \
  --product "Túi kem quilted" \
  --final ~/Movies/CapCut/…/export.mp4 \
  --video data/output/studio-<job-id>.mp4 \   # bỏ trống = video mới nhất
  --sound "Tên sound" --artist "Artist" --link "https://www.tiktok.com/music/…" \
  --music nhac.mp3                            # nếu có file nhạc
# thêm --dry-run để xem trước, không copy
```

Script làm:
1. Chạy QC `--final`. Không đạt thì **không upload**.
2. Tạo `My Drive/AICOS/videos/<YYYY-MM-DD>_<slug>/` (slug bỏ dấu: "Túi kem quilted" → `tui-kem-quilted`).
3. Copy: `<date>_<slug>_final.mp4`, `studio-<job>.mp4`, mọi `photo-*-4k.png`, file nhạc (nếu có), `sound.txt`, `caption.txt`.
4. **Không ghi đè**: file trùng tên thì dừng và liệt kê. Hỏi Ninh trước khi xóa/đổi tên trên Drive.
5. In link folder Drive khi đồng bộ xong. Folder để **private** (không tự chia sẻ).

File gốc trong `data/output/` chỉ được đọc/copy, không xóa.

## PHẦN 6: Báo cáo cho Ninh

- Video đã xong (tên file final)
- Sound trending đã dùng (tên + artist)
- Link folder Google Drive
- Vấn đề gặp phải + cách xử lý

## Quy tắc an toàn

- Không tự đăng video lên TikTok hay nền tảng nào. Chỉ chuẩn bị file (app chỉ đẩy **nháp**).
- Không xóa file trong `data/output/`.
- Hỏi Ninh trước khi ghi đè bất kỳ file nào trên Drive.
- Không đọc, in ra hay gửi đi API key/secret/token của app.
- Bước nào không làm được (CapCut lỗi, Drive chưa login, chưa có video) → DỪNG ở bước đó, báo rõ lý do.
