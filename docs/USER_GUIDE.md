# Hướng dẫn sử dụng (dành cho Ninh)

Tất cả quản trị đều qua **dashboard web** — không cần chạm CLI. Chạy app rồi mở `http://127.0.0.1:8080`.

---

## 1. Cài đặt và chạy trên Mac

```bash
brew install ffmpeg          # bắt buộc duy nhất
chmod +x aicos-darwin-arm64
./aicos-darwin-arm64
```

Mở trình duyệt: `http://127.0.0.1:8080`. Tắt app: `Ctrl+C`. Dữ liệu nằm trong thư mục `data/` cạnh binary.

Model local (VieNeu-TTS v3, LLM GGUF): bấm nút **Tải** trong trang Cài đặt.

Build từ source (nếu cần): `brew install go` rồi `go build -o aicos ./cmd/aicos`.

---

## 2. Các trang trong dashboard

| Trang | Làm gì |
|---|---|
| Trang chủ | Tổng quan mạng lưới, kill switch |
| Tài khoản | Thêm account (username + gợi ý niche), AI tự research + gán persona, onboarding |
| Lịch live | Xếp giờ vàng, tối đa 2 live cùng lúc |
| Agent Team | Cây agent theo thời gian thật — bấm tác vụ để xem trạng thái từng agent |
| Studio AI | Tạo video affiliate / phim ngắn: dựng kịch bản → chụp ảnh → dựng video → caption |
| Sản xuất video | Tạo video từ kịch bản + lồng tiếng AI |
| Đa nền tảng | Kết nối TikTok/Facebook/YouTube, nút Kết nối TikTok (OAuth) |
| Shop Affiliate | Kệ sản phẩm, bật "Tự đăng TikTok khi video xong" |
| Sản phẩm | Tìm sản phẩm affiliate, thêm sản phẩm tay |
| Phân tích | Doanh thu, gift, chi phí API |
| Cài đặt | API key, chuỗi provider TTS/LLM, dry-run, vùng nguy hiểm |

---

## 3. Vận hành hằng ngày (hands-off)

1. Mở app khi muốn hệ thống ON; orchestrator chạy agents theo lịch, stream theo lịch LIVE.
2. Trang chủ cho thấy commission, chi phí, quyết định hệ thống đã đưa ra.
3. Telegram báo khi: stream sập, quota gần hết, có sự kiện kill, lỗi đường tiền.
4. Mỗi tuần xem lại quyết định và chỉnh ngưỡng trong Cài đặt.

---

## 4. Checklist trước khi live thật

- [ ] `go test ./...` pass (trên máy build)
- [ ] Cài đặt: chuỗi provider TTS/LLM + API key đã cấu hình
- [ ] Tài khoản TikTok: đã xác minh quyền Shop Creator + LIVE
- [ ] `DRY_RUN=false` — bật có chủ ý trong Cài đặt
- [ ] Telegram alert đã test
- [ ] Phiên live đầu tiên giám sát trực tiếp qua dashboard

**API key cần chuẩn bị** (khóa lưu trong `.env` đã gitignore, không bao giờ hiện trên UI hay log):

| Key | Lấy ở đâu | Ghi chú |
|---|---|---|
| `GEMINI_API_KEYS` (nhiều key, cách nhau bằng dấu phẩy) | aistudio.google.com | TTS + LLM, có xoay vòng + cooldown tự động |
| `TIKTOK_CLIENT_KEY` / `TIKTOK_CLIENT_SECRET` | developers.tiktok.com | Xem phần 5 |
| `TIKTOK_REDIRECT_URI` | — | `https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html` |
| `RTMP_KEY_<USERNAME>` | TikTok LIVE Center → Go LIVE | Một key mỗi account |

Không có Gemini key thì TTS tự rơi về Edge (giọng kém hơn). Không có app TikTok qua audit thì Posting API chỉ đăng ở chế độ riêng tư cho tối đa 5 tài khoản test.

---

## 5. Kết nối TikTok (một lần mỗi tài khoản)

1. **developers.tiktok.com → Manage apps → Connect an app.** Category: `Others`. Description: công cụ cá nhân đẩy video AI của chính bạn lên hộp thư nháp TikTok để duyệt trước khi đăng. Platforms: chỉ tick **Desktop**.
2. Add products: **Login Kit** + **Content Posting API** (Direct Post để TẮT). Ghi lại Client Key + Client Secret.
3. Terms/Privacy URL bắt buộc là link công khai: dán nội dung `docs/tiktok-app-site/terms.md` và `privacy.md` vào GitHub Gist public (thay `CONTACT_EMAIL`) hoặc Google Docs → Publish to web.
4. Redirect URI: `https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html` (trang trung chuyển tĩnh, tự đưa code về app local).
5. Ghi vào `.env` ở thư mục app (không gõ secret thẳng vào Terminal):

```bash
cat >> .env <<'EOF'
TIKTOK_CLIENT_KEY=...
TIKTOK_CLIENT_SECRET=...
TIKTOK_REDIRECT_URI=https://ninhlee99.github.io/ai-creator-os/tiktok-app-site/callback.html
TIKTOK_DRAFT_ONLY=1
EOF
chmod 600 .env
```

6. Mở app: `set -a; source .env; set +a; ./aicos-darwin-arm64`.
7. Trang **Đa nền tảng** → bấm **Kết nối TikTok** → đăng nhập đúng tài khoản creator → Allow. Token lưu vào `tiktok_token_<username>.json` (quyền 600).
8. Trang **Sản phẩm** → bật **"Tự đăng TikTok khi video xong"**. Video lên dạng **nháp trong Hộp thư TikTok**, không public.

---

## 6. Hậu kỳ mỗi video (bắt buộc làm tay)

API TikTok **không cho gắn giỏ hàng qua code**, nên mỗi video cần mấy phút tay:

1. Mở nháp từ thông báo Hộp thư TikTok trên điện thoại.
2. **Thêm liên kết → Sản phẩm** → tìm đúng sản phẩm affiliate → gắn.
3. Đổi nhạc sang **sound trending** trong thư viện TikTok nếu bản nháp chưa dùng.
4. Dán caption: Studio → Xem storyboard → Copy caption (hoặc `caption.txt` trong folder Drive).
5. Kiểm tra lần cuối (mặt mẫu, tay, sản phẩm đúng) → **Đăng**.

**Không bao giờ** tự đăng video lên TikTok hay nền tảng nào. App chỉ đẩy **nháp**.

### Tinh chỉnh CapCut (chỉ khi cần)

Video từ app đã có giật-giật theo nhịp + nhạc. Chỉ mở CapCut khi cần căn lại beat hoặc thay sound: project 9:16, snap điểm cắt vào nhịp trống, 2–3 giây đầu phải bùng nổ nhất, **không thêm chữ/voiceover/sticker**, export MP4 1080×1920 30fps bitrate ≥ 12 Mbps, tên file `<YYYY-MM-DD>_<ten-san-pham>_final.mp4`.

### QC trước khi đăng

- [ ] Mặt mẫu đồng nhất suốt video
- [ ] Tay tự nhiên, không dị dạng
- [ ] Sản phẩm đúng mẫu, không méo
- [ ] Không chữ / watermark / logo lạ
- [ ] Nhạc khớp nhịp cắt

Có thể chạy `scripts/postprod.sh qc <video> --final` để máy kiểm thông số kỹ thuật (độ phân giải, fps, thời lượng, bitrate, âm thanh).

### Lưu lên Google Drive

```bash
scripts/postprod.sh pack \
  --product "Túi kem quilted" \
  --final ~/Movies/CapCut/…/export.mp4 \
  --sound "Tên sound" --artist "Artist" --link "https://www.tiktok.com/music/…"
# thêm --dry-run để xem trước, không copy
```

Script tạo `My Drive/AICOS/videos/<YYYY-MM-DD>_<slug>/` chứa: video final, video gốc từ app, ảnh master 4K, file nhạc, `sound.txt`, `caption.txt`. Không ghi đè file trùng tên.

---

## 7. An toàn

- **Kill switch:** nút trên Trang chủ, hoặc `kill -USR1` — dừng toàn hệ thống trong vài giây, sổ cái vẫn giữ nguyên.
- **Dry-run:** bật trong Cài đặt để xem trước mọi hành động mà không tác động ra ngoài.
- Không xóa file trong `data/output/`. Hỏi trước khi ghi đè file trên Drive.
- Không đọc, in ra hay gửi đi API key/secret/token.

---

## 8. Backup

```bash
sqlite3 data/ledger.db ".backup data/backup/ledger-$(date +%F).db"
```

Giữ 30 ngày. Backup config cùng thư mục.
