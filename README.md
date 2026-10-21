# AI Creator OS

**Hệ điều hành tự vận hành cho 3 trụ kiếm tiền: Affiliate qua Accesstrade, Video Viral AI 100% Gốc (TikTok/Shorts/Reels), YouTube kể chuyện ngôi thứ nhất.** Bạn chỉ bật app và theo dõi — mọi quyết định vận hành do hệ thống tự quyết theo rule đã định.

> 🌟 **CHUẨN MỚI 2026-10-19: OPEN-SPEC CHUẨN VIDEO CÓ HỒN & QUAY PHIM TỰ NHIÊN ([`docs/OPEN_SPEC.md`](docs/OPEN_SPEC.md))**  
> Xóa bỏ triệt để video dán ảnh tĩnh, thẻ chữ che khung hình và giật lắc cơ học. Toàn bộ video xuất ra đều là chuyển động người thật tự nhiên (Lookbook thời trang & Vũ đạo triệu view TikTok VN / Douyin CN), tỷ lệ chuẩn 9:16 (1080x1920) chống méo hình học, hòa âm đa tầng và ứng dụng InsightFace AI Face Swap khóa mặt creator.

---

## 🎯 Ba trụ

| Trụ | Trạng thái code |
|---|---|
| **1. Affiliate qua Accesstrade** | ✅ Thời lượng tối thiểu **30s – 60s** + Lookbook người mẫu chuyển động thật 9:16 + Ghép mặt nhân vật InsightFace + Cắt nhịp CapCut Pro đồng bộ nhạc TikTok trending + Bio Link Storefront di động (`/bio`). Đảm bảo nhân vật và sản phẩm nhất quán theo phong cách điện ảnh chân thực |
| **2. Tạo video dọc** | ✅ Nhập chủ đề, chọn ảnh tham chiếu, tùy chọn link video làm tham chiếu chuyển động; theo dõi job tại `/studio/jobs`. Không có radar xu hướng video realtime. |
| **3. YouTube Phim Kể Chuyện Audio Điện Ảnh (30 - 60 Phút)** | ✅ Thời lượng **30 – 60 Phút** + mỗi 8-15s là một visual canvas chuyển động + Kịch bản đa cảm xúc + Google Gemini TTS |

Phần đã chạy thật hôm nay: **Cập nhật model Gemini on-demand trên toàn bộ adapter (`PickLatest`)**, **Studio AI Lookbook**, **tạo video dọc từ chủ đề và link tham chiếu tùy chọn** (`/viral`), **Master Agent Harness** (`/harness`), **Kho tài khoản Web AI** (`/settings/system#tab-webai`), nhạc trending VN, dashboard, phát triển kênh, daemon tự động, kill switch + dry-run.

---

## 🖥️ Cấu trúc Giao diện & Điều hướng

Sidebar hệ thống phân nhóm trực quan thành 3 khối nghiệp vụ chính:

| Nhóm | Mục | Route | Vai trò hệ thống |
|---|---|---|---|
| **Điều Hành** | Tổng quan | `/` | Bảng điều khiển thời gian thực: thẻ doanh thu thật, feed quyết định, cảnh báo tăng trưởng, chip trạng thái & kill switch |
| | Agent Harness 🧠 | `/harness` | **Bộ não tự hành cấp cao**: Vòng lặp nhận thức OODA, ký ức kinh nghiệm (Episodic Memory), điều phối tự chủ toàn danh mục |
| | Kênh | `/accounts` | Quản lý kênh TikTok/YouTube/Facebook: hồ sơ Brand DNA, autopilot, kết nối OAuth, badge YouTube Analytics |
| | Đội ngũ Agent | `/team` | Giám sát sức khỏe 3 pipeline sản xuất (Affiliate, Video Viral, Kể chuyện) từ dữ liệu thật |
| **Sản Xuất** | Studio AI 🎬 | `/studio` | Xưởng dàn dựng video: Video bán hàng & Affiliate (30s-60s) · Video Đu Trend Showbiz 9:16 (≥60s) · Truyện audio 16:9 (30-60p) · Nhạc trending |
| | Affiliate | `/products` | Săn deal Accesstrade, kho sản phẩm, kệ hàng thông minh, lịch autopilot |
| **Quản Trị** | Cài đặt hệ thống | `/settings` | 4 phân hệ tinh gọn: Hệ thống (gồm **Quản lý & Cập nhật Gemini Model trên toàn bộ adapter** & **Kho Tài Khoản Web AI**) · Accesstrade · Nhà cung cấp & Chuỗi Key · An toàn |

### 🔗 Các phân hệ chuyên sâu & Đường dẫn trực tiếp:
- **Cửa Hàng Bio Link Kênh (Public Storefront)**: `/bio` hoặc `/bio/{channel}` — Trang web landing page mobile-first tối ưu cho TikTok/Reels/Shorts, tự động đồng bộ từ kệ hàng affiliate, hỗ trợ lọc danh mục tức thì, tìm kiếm nhanh và chuyển hướng tracking link Accesstrade.
- **Tạo video dọc**: `/viral` — Nhập chủ đề, chọn ảnh nhân vật và tùy chọn video tham chiếu; xem job tại `/studio/jobs`.
- **Kể chuyện YouTube**: `/stories` — Hàng đợi kịch bản, ảnh minh họa 16:9, TTS lồng tiếng, tự động đăng private.
- **Phát triển kênh**: `/growth` — Kế hoạch nội dung 30 ngày, canary testing 7 ngày, quy tắc kill / double-down.
- **Đa nền tảng**: `/publishers` — Quản lý token OAuth TikTok, YouTube, Facebook Page & Instagram Reels.
- **Kho Tài khoản Web AI**: `/settings/system#tab-webai` — Trình duyệt tự động đăng nhập CDP, cô lập phiên sạch (`os.MkdirTemp`), tự xoay vòng đa tài khoản ChatGPT, Claude, Gemini, Grok, Muse AI.

---

## ⚙️ Chạy thử trong 60 giây

```bash
git clone https://github.com/ninhlee99/ai-creator-os && cd ai-creator-os
go build -o aicos ./cmd/aicos
./aicos   # mở http://localhost:8080
```

100% Go trong 1 binary (~21–22MB, không Python/venv/pip — "ước tính chưa kiểm
chứng" cho từng bản build cụ thể). SQLite WAL. FFmpeg là công cụ ngoài cần
cài để dựng video; LLM dùng Gemini, TTS dùng Gemini rồi Edge.

```bash
go test ./...                  # runtime chính (không gồm vùng parked)
go test -tags parked ./...     # vùng parked vẫn test được riêng
bash scripts/check-reachable.sh  # không package nào bị bỏ rơi
```

Lần đầu chạy (thư mục dữ liệu trống) → wizard `/onboard` → chọn chế độ thử →
vào app.

---

## 📌 Trạng thái thật (Đợt H3 — đã tự review 92/100)

| Phần | Trạng thái |
|---|---|
| Sidebar: Tổng quan, Studio Jobs, Sản phẩm, Kênh, Cài đặt | ✅ Code chạy thật |
| Đội ngũ (`/team`): sức khỏe 3 pipeline từ dữ liệu thật | ✅ Đợt H2 |
| Studio AI — video affiliate + trends | ✅ Code chạy thật |
| Key rotation Gemini (round-robin, cooldown 60s→5m→15m khi 429) | ✅ |
| Sổ cái, kill switch, dry-run, MASTER_SWITCH | ✅ |
| Growth: plan 30 ngày, ngưỡng, sản xuất affiliate tự động | ✅ |
| Live (stream engine, avatar, lịch live, tab Nhân vật AI) | 🅿️ **Parked** — build tag `parked`, không vào binary |
| Phim điện ảnh (film mode, cinematic, Veo) | 🅿️ **Parked** — build tag `parked` |
| Agent Team (`/team`) | ✅ 3 pipeline, trạng thái từ dữ liệu thật |
| Accesstrade API (token, campaign, datafeed, đối soát) | ✅ Đợt B/C — chưa test bằng token thật |
| Video Viral AI (kịch bản 3s hook, ảnh 9:16, lồng tiếng, 100% gốc) | ✅ 2026-10-16 — Thay thế Reup, loại bỏ 0-view shadowban |
| YouTube kể chuyện (truyện → ảnh → TTS → dựng → đăng private) | ✅ Đợt F — pipeline test bằng provider fake; cần key thật trên máy Ninh |
| YouTube Analytics (views/watch hours 30 ngày) | ✅ Đợt H3 — cần token OAuth scope yt-analytics.readonly |
| TikTok Shop | ❌ Đã loại bỏ hoàn toàn ở Đợt H1 (Accesstrade-only) |
| Automation ticks (growth, AT, viral, story) | ✅ Đang bật; trung thực trong UI |
| Facebook Page · Instagram Reels · Threads (Meta Graph API trực tiếp, không cần CLI) | ✅ 2026-10-12 — FB tải lên chưa công khai; IG chỉ đăng khi `IG_AUTO_PUBLISH=1` (API không có nháp); Threads đăng bài chữ kèm link (API không nhận video local) |
| Studio không cần key — dựng từ ảnh tải lên + caption mẫu | ✅ 2026-10-12 |
| Không chèn chữ vào video (viral, kể chuyện, affiliate) | ✅ 2026-10-12 |
| Checklist “sẵn sàng lên xu hướng” 7 tiêu chí / video | ✅ 2026-10-12 — yếu tố phân phối đã biết, không phải cam kết viral |
| Hẹn giờ vàng YouTube (`YOUTUBE_GOLDEN_HOUR=1`, 11:30 / 19:30 giờ VN) | ✅ 2026-10-12 |
| Tiến độ YPP (500/1.000 sub · 3.000/4.000 giờ · 3M/10M view Shorts) | ✅ 2026-10-12 — cần OAuth Analytics |
| “Mở bằng CapCut” — xuất gói video + ảnh cảnh + nhạc, mở Finder + CapCut | ✅ 2026-10-12 — CapCut không có API nên kéo thả tay |
| TikTok đăng trực tiếp | ❌ Cần app audit (Ninh từ chối) — chỉ chế độ nháp |
| Cập nhật model Gemini on-demand (`PickLatest`) trên toàn bộ adapter | ✅ 2026-10-20 — Quét Google API theo yêu cầu, tự động phát hiện và nâng cấp toàn bộ model: Text (`gemini-3.8-flash`), TTS (`gemini-3.8-flash-tts`), Image (`gemini-nano-banana-2.1`), Video (`veo-3.1-generate-preview`), hot-swap tức thì, lưu cache bền vững tại `data/discovered_models.json` |
| Video không cần key — nhiều ảnh | ✅ 2026-10-12 — tải tối đa 7 ảnh (Studio) / 6 ảnh listing (săn Accesstrade); ≥3 ảnh thì mỗi ảnh 2 khung, video không lặp 1 ảnh |
| AI Mission Control — Giao mục tiêu chiến lược 1-chạm & tự động thực thi | ✅ 2026-10-16 — Phân tích mục tiêu, tự động phối hợp các khối, theo dõi tiến độ thời gian thực |
| Brand DNA & Original TikTok Engine — Bản sắc thương hiệu & nội dung nguyên bản | ✅ 2026-10-16 — Hồ sơ Brand DNA từng kênh (Định vị, Chân dung, 4 Hook 3s, Ranh giới đỏ, QC 10 tiêu chí) |
| Trend Intelligence — Bóc tách xu hướng & tính điểm cơ hội (Zero-Reup) | ✅ 2026-10-16 — Phân tích tín hiệu Kworb VN & radar Douyin, chấm điểm tương thích Brand DNA |
| **Master Agent Harness (`/harness`)** — Não tự hành OODA & Ký ức kinh nghiệm | ✅ 2026-10-16 — Vòng lặp Quan sát-Định hướng-Quyết định-Hành động, lưu trữ sự kiện SQLite |
| **Kho Tài Khoản Web AI (`/settings/system#tab-webai`)** — CDP Login & Multi-Account | ✅ 2026-10-17 — Tự động mở trình duyệt CDP, bắt cookie/session ChatGPT, Claude, Gemini, Grok, Muse AI, cô lập profile tạm thời |
| Bảng “Tự động hóa” trên Trang chủ | ✅ 2026-10-12 — 6 công tắc tự động gom 1 chỗ (vẫn ghi đúng khóa của trang gốc) |
| Veo / key Gemini trả phí | ❌ Không bao giờ — free-only là luật cứng |

**Test:** Toàn bộ test suites trong runtime chính đạt 100% PASS. Build/vet/test
xanh cả 2 tag — kiểm chứng lại bằng lệnh ở trên.

---

## 📦 Phát hành 1-binary

```bash
# Build phát hành có đóng dấu version (mặc định là "dev"):
go build -ldflags "-X main.version=v1.0.0" -o aicos ./cmd/aicos
./aicos -version   # aicos v1.0.0
```

- **1 binary / 1 nền tảng** (Windows/macOS/Linux): `GOOS=windows GOARCH=amd64 go build ./cmd/aicos`…
- **Dữ liệu nằm gọn trong 1 thư mục** (`-data`, mặc định `./data`): `ledger.db`, `studio.db`, `products.db`, `webai.db`, `content_jobs.json`, `tokens/`, `output/`, `models/`.
- **Sao lưu/khôi phục** ở Cài đặt · Hệ thống: tải `.zip` ảnh chụp dữ liệu; khôi phục upload zip → kiểm chứng → khởi động lại app để áp dụng.
- **API `/api/*` mặc định TẮT** (bật bằng công tắc ở Cài đặt · Hệ thống khi cần).

## 📚 Tài liệu

| Ai đọc | File |
|---|---|
| Từng tính năng — mô tả đúng logic code thật | [`docs/features/`](docs/features/) (19 file chuyên sâu, mô tả chính xác logic code thật) |
| Thiết kế pivot 3 trụ (đích đến các đợt B→G) | [`docs/PIVOT_REDESIGN.md`](docs/PIVOT_REDESIGN.md) |
| Nghiên cứu Accesstrade + Douyin | [`docs/REUP_AFFILIATE_RESEARCH.md`](docs/REUP_AFFILIATE_RESEARCH.md) |
| Chuẩn tuân thủ nghiêm ngặt (kiến trúc, docs, UI, quy trình, zero-touch) | [`docs/STANDARDS.md`](docs/STANDARDS.md) |
| Kiến trúc kỹ thuật | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| Thiết kế UI/UX | [`docs/UI_UX_BLUEPRINT.md`](docs/UI_UX_BLUEPRINT.md) |
| Người dùng: cài đặt, chạy trên Mac, vận hành hằng ngày | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) |
| Phân tích khoảng trống sản phẩm & Nhu cầu người dùng | [`docs/GAP_ANALYSIS_AND_USER_NEEDS.md`](docs/GAP_ANALYSIS_AND_USER_NEEDS.md) |
| Dev / AI code assistant | [`docs/DEVELOPER.md`](docs/DEVELOPER.md) |
| Chính sách & an toàn | [`docs/POLICY_AND_SAFETY.md`](docs/POLICY_AND_SAFETY.md) |

## ⚖️ Giấy phép

All rights reserved — xem [`LICENSE`](LICENSE). Mọi quyền được bảo lưu; chưa cho phép sao chép/phân phối/sửa đổi khi chưa có văn bản đồng ý.
