# AI Creator OS

**Hệ điều hành tự vận hành cho 3 trụ kiếm tiền: Affiliate qua Accesstrade, Reup video Douyin, YouTube kể chuyện ngôi thứ nhất.** Bạn chỉ bật app và theo dõi — mọi quyết định vận hành do hệ thống tự quyết theo rule đã định.

> **PIVOT 2026-10-02 (Ninh chốt):** bỏ live avatar và bỏ làm phim điện ảnh.
> Code live/phim đã **park bằng build tag `parked`** (không biên dịch vào
> binary, vẫn nằm trong git để lôi lại khi cần). Thiết kế đầy đủ:
> [`docs/PIVOT_REDESIGN.md`](docs/PIVOT_REDESIGN.md).

---

## 🎯 Ba trụ

| Trụ | Trạng thái code (sau Đợt G, commit `c457999`) |
|---|---|
| **1. Affiliate qua Accesstrade** | ✅ Đợt B/C xong: client API official (`Authorization: Token`) + tab key trong Cài đặt + trang Affiliate (tải chiến dịch, tạo tracking link, săn sản phẩm từ datafeed, đối soát pending/approved/rejected) + tick hunter/order-sync/campaign-check. Fail-closed khi chưa có key — **chưa test bằng token thật** |
| **2. Reup video Douyin** | ✅ Đợt D/E xong: downloader (yt-dlp do app tự quản + TikWM fallback) + dedupe 2 lớp + QC + tick 6h tìm video viral theo play_count; transform 2 mức (zoom động, tốc độ ±5%, voiceover bình luận tiếng Việt, nhạc licensed, compilation 3 clip) + kill rule 0-view (5 video liên tiếp → dừng đăng + báo động) |
| **3. YouTube kể chuyện ngôi thứ nhất** | ✅ Đợt F xong: truyện → chia cảnh → ảnh minh họa 16:9 → TTS từng cảnh → dựng 16:9 + subtitle → QC → đăng private-first, mặc định chờ Ninh duyệt + tick 24h lấy chủ đề từ hàng đợi |

Phần đã chạy thật hôm nay: **Studio AI** (video affiliate: ảnh sản phẩm + nhạc, không chữ không voiceover — format Ninh chốt), nhạc trending VN, dashboard, phát triển kênh, daemon tự động, kill switch + dry-run.

> ⚠️ **Trung thực về reup:** không có transform nào đảm bảo 100% không bị
> đánh bản quyền. Hệ thống chỉ giảm rủi ro (voiceover Việt gốc, nhạc bản
> quyền thay thế, restructure/compilation, crop/zoom động…), và rủi ro thực tế
> lớn nhất có thể là **0 view / bị giảm phân phối**, không chỉ là strike.

---

## 🖥️ Giao diện hiện tại (sidebar 9 mục)

| Mục | Route | Vai trò hiện tại |
|---|---|---|
| Trang chủ | `/` | Thẻ số liệu, feed 8 quyết định, cảnh báo growth, trạng thái runtime local, chip MASTER_SWITCH |
| Kênh | `/accounts` | Quản lý kênh TikTok/YouTube/Facebook: trạng thái, autopilot, kết nối OAuth |
| Phát triển kênh | `/growth` | Kế hoạch nội dung 30 ngày, ngưỡng kill/double-down, sản xuất tự động |
| Studio AI 🎬 | `/studio` | **Video affiliate** (ảnh + nhạc) · Video chữ động · Jobs · Nhạc trending. Phim điện ảnh đã park |
| Affiliate | `/products` | Kho sản phẩm + kệ hàng + lịch autopilot (rework Accesstrade ở đợt B/C) |
| Reup | `/reup` | Nguồn Douyin · Hàng đợi tải · Transform 2 mức · Xem trước before/after · Metrics · Kill rule 0-view |
| Kể chuyện | `/stories` | Truyện ngôi thứ nhất → ảnh minh họa từng cảnh → TTS → dựng 16:9 → đăng YouTube (chờ duyệt) |
| Đa nền tảng | `/publishers` | Trạng thái kết nối TikTok/YouTube/Facebook/RTMP từng kênh |
| Cài đặt | `/settings` | 6 trang: Hệ thống · Accesstrade · Reup · Nhà cung cấp · Model local · An toàn. Tab Nhân vật AI đã park |

`/schedule` (Lịch live) và `/team` (Agent Team bản live) đã park → **404**.
Agent Team sẽ được định nghĩa lại quanh 3 pipeline ở đợt sau.

---

## ⚙️ Chạy thử trong 60 giây

```bash
git clone https://github.com/ninhlee99/ai-creator-os && cd ai-creator-os
go build -o aicos ./cmd/aicos
./aicos   # mở http://localhost:8080
```

100% Go trong 1 binary (~21–22MB, không Python/venv/pip — "ước tính chưa kiểm
chứng" cho từng bản build cụ thể). SQLite WAL. Model local (VieNeu-TTS v3,
llama-server) tải bằng một nút trong Cài đặt · Model local.

```bash
go test ./...                  # runtime chính (không gồm vùng parked)
go test -tags parked ./...     # vùng parked vẫn test được riêng
bash scripts/check-reachable.sh  # không package nào bị bỏ rơi
```

Lần đầu chạy (thư mục dữ liệu trống) → wizard `/onboard` → chọn chế độ thử →
vào app.

---

## 📌 Trạng thái thật (Đợt G, commit `c457999` — đã tự review 91/100)

| Phần | Trạng thái |
|---|---|
| Sidebar 9 mục, 6 tab Cài đặt, không link chết | ✅ Code chạy thật |
| Studio AI — video affiliate + video chữ động + trends | ✅ Code chạy thật |
| Key rotation Gemini (round-robin, cooldown 60s→5m→15m khi 429) | ✅ |
| Sổ cái, kill switch, dry-run, MASTER_SWITCH | ✅ |
| Growth: plan 30 ngày, ngưỡng, sản xuất affiliate tự động | ✅ |
| Live (stream engine, avatar, lịch live, tab Nhân vật AI) | 🅿️ **Parked** — build tag `parked`, không vào binary |
| Phim điện ảnh (film mode, cinematic, Veo) | 🅿️ **Parked** — build tag `parked` |
| Agent Team (`/team`) | 🅿️ Parked — sẽ định nghĩa lại quanh 3 pipeline |
| Accesstrade API (token, campaign, datafeed, đối soát) | ✅ Đợt B/C — chưa test bằng token thật |
| Reup Douyin (download, transform, kill rule 0-view) | ✅ Đợt D/E — chưa tải video thật (cần máy Ninh có mạng) |
| YouTube kể chuyện (truyện → ảnh → TTS → dựng → đăng private) | ✅ Đợt F — pipeline test bằng provider fake; cần key thật trên máy Ninh |
| 8 tick automation (growth, AT, reup ×4, story) | ✅ Đợt G — đang bật; AT "chờ key", kill rule "chờ số liệu" (trung thực trong UI) |
| TikTok Shop API chính thức | ❌ Đã loại theo quyết định của Ninh (2026-10-01) |
| Veo / key Gemini trả phí | ❌ Không bao giờ — free-only là luật cứng |

**Test:** 524 hàm test trong 82 file (không tính vùng parked; 14 file test
parked chạy riêng bằng `-tags parked`). Build/vet/test xanh cả 2 tag ở commit
`c457999` — kiểm chứng lại bằng lệnh ở trên.

---

## 📦 Phát hành 1-binary

```bash
# Build phát hành có đóng dấu version (mặc định là "dev"):
go build -ldflags "-X main.version=v1.0.0" -o aicos ./cmd/aicos
./aicos -version   # aicos v1.0.0
```

- **1 binary / 1 nền tảng** (Windows/macOS/Linux): `GOOS=windows GOARCH=amd64 go build ./cmd/aicos`…
- **Dữ liệu nằm gọn trong 1 thư mục** (`-data`, mặc định `./data`): `ledger.db`, `studio.db`, `products.db`, `content_jobs.json`, `tokens/`, `output/`, `models/`.
- **Sao lưu/khôi phục** ở Cài đặt · Hệ thống: tải `.zip` ảnh chụp dữ liệu; khôi phục upload zip → kiểm chứng → khởi động lại app để áp dụng.
- **API `/api/*` mặc định TẮT** (bật bằng công tắc ở Cài đặt · Hệ thống khi cần).

## 📚 Tài liệu

| Ai đọc | File |
|---|---|
| Từng tính năng — mô tả đúng logic code thật | [`docs/features/`](docs/features/) (16 file, mỗi tính năng một file) |
| Thiết kế pivot 3 trụ (đích đến các đợt B→G) | [`docs/PIVOT_REDESIGN.md`](docs/PIVOT_REDESIGN.md) |
| Nghiên cứu Accesstrade + Douyin | [`docs/REUP_AFFILIATE_RESEARCH.md`](docs/REUP_AFFILIATE_RESEARCH.md) |
| Chuẩn tuân thủ nghiêm ngặt (kiến trúc, docs, UI, quy trình, zero-touch) | [`docs/STANDARDS.md`](docs/STANDARDS.md) |
| Kiến trúc kỹ thuật | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| Thiết kế UI/UX | [`docs/UI_UX_BLUEPRINT.md`](docs/UI_UX_BLUEPRINT.md) |
| Người dùng: cài đặt, chạy trên Mac, vận hành hằng ngày | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) |
| Dev / AI code assistant | [`docs/DEVELOPER.md`](docs/DEVELOPER.md) |
| Chính sách & an toàn | [`docs/POLICY_AND_SAFETY.md`](docs/POLICY_AND_SAFETY.md) |
| Lịch sử: nghiên cứu phim (đã park) | `docs/FILM_REALISM_UPGRADE.md`, `docs/STUDIO_FILM.md`, `docs/FILM_RULES.md` |

## ⚖️ Giấy phép

All rights reserved — xem [`LICENSE`](LICENSE). Mọi quyền được bảo lưu; chưa cho phép sao chép/phân phối/sửa đổi khi chưa có văn bản đồng ý.
