# Hướng dẫn sử dụng (dành cho Ninh)

> Cập nhật theo app thật (3 trụ: Affiliate Accesstrade,
> Video Viral AI 100% Gốc, YouTube kể chuyện). Mọi quản trị đều qua **dashboard web** —
> không cần chạm CLI.

## 1. Cài đặt

**Cách nhanh nhất:** tải binary đúng máy tại **GitHub Releases**
(`github.com/ninhlee99/ai-creator-os` → Releases), chạy là dùng — không cài
thêm gì (Go runtime không cần).

| Máy | File | Chạy |
|---|---|---|
| Mac M1/M2/M3 | `aicos-darwin-arm64` | `chmod +x aicos-darwin-arm64 && ./aicos-darwin-arm64` |
| Mac Intel | `aicos-darwin-amd64` | tương tự |
| Windows | `aicos-windows-amd64.exe` | nhấp đúp |
| Linux | `aicos-linux-amd64` | `chmod +x` rồi chạy |
| Linux ARM64 | `aicos-linux-arm64` | `chmod +x` rồi chạy |

Mở trình duyệt: `http://127.0.0.1:8080`. Lần đầu app hỏi thiết lập 1 bước
(dry-run bật sẵn cho an toàn).

**Công cụ ngoài** (trang Cài đặt · Hệ thống tự kiểm tra và báo thiếu, hướng dẫn
cài theo đúng hệ điều hành của máy):

| Công cụ | Để làm gì | Cài |
|---|---|---|
| FFmpeg | dựng video (bắt buộc) | Mac: `brew install ffmpeg` · Win: `winget install Gyan.FFmpeg` · Linux: `sudo apt install ffmpeg` |

**Build từ source** (nếu cần): cài Go 1.26 rồi `go build -o aicos ./cmd/aicos`.

## 2. Các trang trong dashboard

| Trang | Route | Làm gì |
|---|---|---|
| Tổng quan | `/` | Bảng điều khiển, thẻ số liệu doanh thu thật, feed quyết định, kill switch |
| Agent Harness 🧠 | `/harness` | Bộ não tự hành cấp cao: Vòng lặp nhận thức OODA, ký ức kinh nghiệm (Episodic Memory) |
| Kênh | `/accounts` | Tài khoản TikTok/YouTube/Facebook, hồ sơ Brand DNA từng kênh |
| Phát triển kênh | `/growth` | Kế hoạch 30 ngày, canary 7 ngày, luật kill/nhân đôi |
| Đội ngũ | `/team` | Pipeline Affiliate / Video Viral AI / Kể chuyện từ dữ liệu thật |
| Studio AI 🎬 | `/studio` | Dàn dựng video bán hàng, video đu trend showbiz 9:16, audio story 16:9 |
| Affiliate | `/products` | Chiến dịch Accesstrade, săn deal, tạo link, kệ hàng bio |
| Tạo video dọc | `/viral` | Nhập chủ đề, chọn ảnh nhân vật, thêm link tham chiếu tùy chọn; theo dõi job ở `/studio/jobs` |
| Kể chuyện | `/stories` | Truyện audio YouTube: hàng đợi chủ đề, job, tự động đăng private |
| Đa nền tảng | `/publishers` | Kết nối OAuth TikTok/YouTube/Meta Facebook & Instagram |
| Cài đặt | `/settings` | 4 trang tinh gọn: Hệ thống (Quản lý Model Gemini Auto-Update 24h & **Kho Tài Khoản Web AI**), Accesstrade, Thứ tự AI, An toàn |

### 2b. Kết nối tài khoản AI qua Trình duyệt Tự Động (CDP)
Hệ thống cho phép bạn dùng ChatGPT, Claude, Gemini, Grok, Muse AI hoàn toàn tự động mà **không cần sao chép cookie bằng tay**:
1. Vào **Cài đặt · Hệ thống** (`/settings/he-thong`) → bấm tab **🤖 Tài khoản AI**.
2. Tại thẻ của nền tảng bạn muốn kết nối (ví dụ: Grok hoặc Claude), bấm **"🌐 Đăng Nhập Tự Động"**.
3. Một cửa sổ trình duyệt sạch (profile cô lập hoàn toàn) sẽ tự mở lên. Bạn đăng nhập tài khoản bình thường.
4. Ngay khi đăng nhập thành công, app sẽ tự động nhận diện email, lưu phiên làm việc an toàn và tự đóng trình duyệt.
5. Hỗ trợ xoay vòng nhiều tài khoản (multi-account) cho từng nền tảng, tự động chuyển tài khoản khi gặp giới hạn (rate limit).

### 2c. Khóa Nhận Diện Khuôn Mặt Nhân Vật Theo Kênh (Character Identity Lock)
Tạo nhân vật ảo / KOC ảo với khuôn mặt nhất quán 100% xuyên suốt mọi video clip:
1. Vào **Kênh Sáng Tạo** (`/accounts`) hoặc tạo kênh mới (`/accounts/new`).
2. Chọn ngay 1 trong 10 nhân vật mẫu có sẵn (Thần Tiên Tỉ Tỉ, KOC Công Nghệ, Beauty Blogger...) hoặc tải lên ảnh đại diện riêng. Nếu để trống, hệ thống sẽ tự động gán ngẫu nhiên 1 nhân vật mẫu.
3. Quản lý và bổ sung ảnh góc mặt của nhân vật tại tab **👤 Nhân vật & Khuôn mặt AI** trong trang chi tiết kênh (`/accounts/{id}?tab=nhan-vat`). Hệ thống lưu diện mạo chuẩn và tự động map vào mọi video Affiliate Lookbook hoặc Vũ Đạo Viral.

### 2d. Cập Nhật Model Gemini Trên Toàn Bộ Adapter (On-demand Updater)
Cập nhật theo yêu cầu chỉ với 1 click tại **Cài đặt · Hệ thống** (`/settings/he-thong`):
- **Văn bản & Kịch bản**: `gemini-3.8-flash` (tự động nâng cấp khi Google phát hành bản mới hơn).
- **Tạo ảnh & Phân cảnh**: `gemini-nano-banana-2.1` (dòng Nano Banana chất lượng cao chuyên tạo nhân vật Thần tiên tỉ tỉ, sản phẩm affiliate).
- **Giọng đọc kể chuyện**: `gemini-3.8-flash-tts` (đa cảm xúc với các tag `[trầm lặng]`, `[cao trào]`, `[sôi nổi]`, `[tức giận]`, `[buồn bã]`, `[vui vẻ]`, `[thì thầm]`).
- **Tạo video chuyển động**: `veo-3.1-generate-preview` (Google Veo 3.1).
- Bấm nút **"⚡ Cập Nhật Tất Cả Model Gemini"** để quét Google API và nâng cấp tức thì mà không cần restart máy chủ. Không chạy tiến trình định kỳ ngầm 24h.

### 2e. Tiêu Chuẩn Thời Lượng Video Xuất Bản
- **Video Affiliate Lookbook**: Thời lượng **30s – 60s**, giữ nhất quán chi tiết trang phục và người mẫu theo phong cách điện ảnh chân thực.
- **Video Vũ Đạo Viral**: Thời lượng **từ 60s trở lên** (chuẩn 1 phút+ TikTok bật kiếm tiền), bắt đầu tức thì tại 0.0s (triệt tiêu hoàn toàn màn hình đen 5s đầu), visual nhân vật "Thần tiên tỉ tỉ".
- **Video Truyện Audio**: Thời lượng **30 đến 60 phút**, kịch bản chân thật, kịch tính, lôi cuốn có cao trào, trầm lặng, sôi nổi, hào hứng, buồn bã, lặng lẽ với Gemini TTS.
1. Vào **Kênh** (`/accounts`) → chọn kênh cần thiết lập → bấm tab **👤 Nhân vật & Khuôn mặt AI** (`/accounts/{id}?tab=nhan-vat`).
2. Tải lên 1 hoặc nhiều ảnh chân dung góc chụp rõ nét của nhân vật.
3. Khi tạo video tại **Video Viral AI** (`/viral`) hoặc **Studio Ad Creative** (`/studio`), hệ thống tự động nhận diện khuôn mặt nhân vật đã ghim của kênh và áp dụng kỹ thuật Character Reference Injection vào AI MediaGen để giữ nguyên thần thái, diện mạo nhân vật trong từng chuyển động, vũ đạo và biểu cảm.
4. Bạn cũng có thể tải ảnh nhân vật tùy chỉnh trực tiếp ngay tại form tạo video `/viral/create`.

### 2d. Sản Xuất Video Affiliate Hoa Hồng Cao (≥ 20%) & Đăng TikTok
1. Vào **Sản phẩm & Kệ hàng** (`/products`) → tab **Kệ Hàng Sản Xuất AI** (`/products?tab=ke`). Các sản phẩm có hoa hồng $\ge 20\%$ được tự động gắn huy hiệu **🔥 Hoa hồng cao**.
2. Bấm **🎬 Dựng video AI** bên cạnh sản phẩm để tạo video quảng bá với định dạng chuẩn dọc 9:16 (1080×1920).
3. Sau khi video hoàn tất tại **Studio Jobs** (`/studio/jobs`), bấm **📋 Copy Post Kit** để lấy trọn bộ Tiêu đề, Hook 3s, Chú thích chốt sale và Hashtags thịnh hành (#thoitrangnu #phukienthoitrang #xuhuong #dealhot) kèm link Bio Storefront để đăng kênh TikTok ngay.
4. Bấm **🔗 Ghim vào Kệ Bio** để sản phẩm tự động xuất hiện trên trang Bio Link (`/bio`) của kênh với cơ chế tracking lượt click tự động.

### 2e. Tạo Truyện Kể YouTube 60 Phút & Tự Động Cắt Shorts (9:16)
1. Tại **Studio AI** (`/studio`) hoặc qua API `/studio/story`, nhập kịch bản cốt truyện hấp dẫn, kịch tính (ví dụ: Hôn nhân hợp đồng, tổng tài, tình cảm nam nữ kịch tính).
2. Hệ thống tự động phân tách kịch bản thành các hồi và phân cảnh điện ảnh, render video Full HD 1080p (1920×1080, tỉ lệ 16:9) kèm giọng đọc truyền cảm và nhạc nền du dương.
3. **Bộ cắt tự động (Auto-Clipper)** tích hợp sẵn sẽ quét các phân cảnh cao trào nhất và tự động cắt trích đoạn 9:16 (1080×1920) kèm phụ đề để làm video ngắn Shorts / TikTok / Reels kéo view về video dài YouTube.
4. Tại `/studio/jobs`, video dài 16:9 và bản cắt Shorts 9:16 đều có player phóng to và nút tải riêng biệt.

### 2f. Tạo video dọc
1. Mở `/viral` và nhập chủ đề video.
2. Chọn ảnh nhân vật từ thư viện kênh hoặc tải ảnh mới. Có thể dán link TikTok/Douyin làm video tham chiếu.
3. Chọn engine và thời lượng rồi bấm **Tạo video**. Theo dõi trạng thái và tải kết quả tại `/studio/jobs`.

Trang này hiện không cung cấp radar video realtime hay chủ đề mẫu tự động.

## 3. Vận hành hằng ngày (hands-off)

1. **Mở máy + mạng + start app** — đó là toàn bộ việc của Ninh. Mọi quyết định
   vận hành (chọn provider, fallback khi hết quota, lịch đăng) do app tự quyết
   theo nguyên tắc "free trước".
2. Telegram báo khi: lỗi/quota/có sự kiện kill (bật ở Cài đặt · Hệ thống).
3. Muốn app tự chạy lại khi crash/restart máy (Mac): làm theo
   `deploy/README.md` (watchdog launchd).
4. Kiểm tra sức khỏe định kỳ: `scripts/watch-health.sh` (cron mỗi 5 phút) —
   phát hiện app treo mà process chưa chết.

## 4. Cập nhật app

1. Tải binary mới từ GitHub Releases, thay file cũ.
2. Restart app (watchdog launchd tự chạy lại bản mới).
3. Xem version đang chạy ở Cài đặt · Hệ thống (thẻ "Phiên bản app").

## 5. An toàn

- **Kill switch:** nút trên thanh topbar — dừng mọi hoạt động tự động trong vài giây.
- **Dry-run:** bật ở Cài đặt · An toàn để xem trước mọi hành động mà không tác động ra ngoài.
- **Mật khẩu dashboard:** đặt ở Cài đặt · Hệ thống → "Mật khẩu truy cập".
  Bắt buộc đặt trước khi mở app ra mạng LAN/VPS. Quên mật khẩu: dừng app,
  chạy lại với `AICOS_RESET_AUTH=1` (hoặc xóa key `auth.password_hash` bằng
  sqlite3 trên Mac/Linux), mở app lại và đặt mật khẩu mới.
- Không đọc, in ra hay gửi đi API key/secret/token.

## 6. Backup & khôi phục

App **tự sao lưu mỗi ngày** vào `backups/` (giữ 7 bản mới nhất) — đủ 5 database
(ledger, studio, products, accesstrade, webai). Trước khi lưu, app kiểm tra
toàn vẹn từng database; bản hỏng bị loại, giữ bản cũ dùng được. Muốn có bản
ở chỗ khác (phòng chết đĩa): nhập thư mục USB/ổ mạng ở Cài đặt · Hệ thống →
"Sao lưu & khôi phục" → "Bản sao dự phòng (offsite)" — app tự copy bản mới
nhất sang đó mỗi ngày. Khôi phục: dừng app, giải nén bản muốn về, chép đè các
file `.db` vào `data/`, mở app lại. Chi tiết:
`docs/features/sao-luu-khoi-phuc.md`.

- **Kênh cảnh báo dự phòng:** ngoài Telegram, có thể nhập "Webhook dự phòng" ở
  Cài đặt · Hệ thống → "Thông báo Telegram" — khi Telegram gửi lỗi, cảnh báo
  tự gửi tới địa chỉ này.
