# AI Creator OS

**Hệ điều hành cho mạng lưới AI creator tự vận hành — mỗi tài khoản TikTok là một creator AI với persona riêng: tự tìm niche, tự sản xuất video, tự livestream, tự đăng đa nền tảng. Bạn chỉ bật ON và theo dõi.**

![Tổng quan mạng lưới AI creator](docs/assets/hero-network.webp)

> 🖼️ **Ghi chú trung thực:** ảnh concept do AI tạo để minh họa ý tưởng — không phải ảnh chụp sản phẩm thật. Các video demo bên dưới được **dựng thật bằng pipeline FFmpeg + lồng tiếng AI** của dự án.

---

## 🎬 Xem nhanh

**Quy trình tự động — từ săn sản phẩm đến đăng video**

<video src="docs/assets/demo-pipeline.mp4" controls width="100%"></video>

**Video affiliate demo — sản phẩm AI làm từ A–Z** (kịch bản + lồng tiếng + phụ đề, đúng chuẩn pipeline Content)

<video src="docs/assets/demo-affiliate-video.mp4" controls width="360"></video>

**Video concept 1 phút — do Milo (AI assistant) dựng** (6 clip AI × 10s, ghép bằng FFmpeg — minh họa khả năng B-roll cho video ngắn)

<video src="docs/assets/demo-1min.mp4" controls width="100%"></video>

**Dashboard điều khiển** — mọi quản trị qua UI web, không dùng CLI:

![Dashboard](docs/assets/dashboard-mockup.webp)

`./aicos` → mở `http://localhost:8080`

---

## 🧭 Mô hình chốt: AI Creator Network

**Một hệ thống, N tài khoản TikTok.** Mỗi account là một "AI creator" với persona riêng — tự live giải trí theo giờ được phân bổ, thu **quà tặng LIVE**, bán **affiliate qua video ngắn**, nhạc AI tự sáng tác thì phát hành lấy **royalty**.

| Persona (mỗi account 1 cái) | Live làm gì | Nguồn thu |
|---|---|---|
| 📖 Storyteller | Kể chuyện, phim ngắn AI | Gift + affiliate |
| 🎓 AI Teacher | Dạy tiếng Anh qua truyện | Gift + affiliate |
| 🎮 Game Master *(phase 2)* | AI tự chơi game được phép | Gift + affiliate gear |
| 💻 AI Coder *(phase 2)* | Live code game/mini-app theo yêu cầu viewer | Gift |
| 🎤 AI Musician *(phase 3)* | Hát nhạc AI tự sáng tác | Gift + royalty |
| 💃 Dancer *(phase 4)* | Nhảy theo trend | Gift |

Chi tiết: [`docs/MODEL.md`](docs/MODEL.md)

**Bạn làm 3 việc trên dashboard web:** bật/tắt master switch · thêm account (AI tự research niche, gán persona) · theo dõi. **Hệ thống tự làm phần còn lại:** research → đăng video cày đủ 1.000 follow → xếp lịch live giờ vàng (tối đa 2 live cùng lúc) → live → đối soát → tối ưu.

### 🤖 Agent Team

Trang **Agent Team** trong dashboard vẽ cây trạng thái **suy ra từ tiến độ job Studio** (đang làm · đang nhận việc · xong · lỗi) — trung thực, không bịa agent đang chạy. 7 package agent/stream cũ (analyst, content, governance, hunter, streamer, affiliatehunter, stream) đã **park** sau tag build `parked` ngày 2026-10-02: không vào binary, test vẫn chạy bằng `go test -tags parked ./...` (chi tiết: `docs/ARCHITECTURE.md` §12).

![Đội agent và persona](docs/assets/agents/contact-sheet.png)

Thiết kế đầy đủ: [`docs/AGENT_TEAM.md`](docs/AGENT_TEAM.md)

---

## ⚙️ Chạy thử trong 60 giây

```bash
git clone https://github.com/ninhlee99/ai-creator-os && cd ai-creator-os
go build -o aicos ./cmd/aicos
./aicos   # mở http://localhost:8080
```

100% Go trong 1 binary (~21–22MB, không Python/venv/pip). SQLite WAL cho sổ cái. Model local (VieNeu-TTS v3, llama-server) tải bằng một nút trong trang Settings.

```bash
go test ./...   # toàn bộ test chạy với dữ liệu giả, không chạm TikTok thật
```

---

## 📌 Trạng thái thật của dự án

| Phần | Trạng thái |
|---|---|
| Dashboard web (9 trang: Trang chủ, Tài khoản, Lịch live, Phát triển kênh, Sản phẩm, Studio AI, Agent Team, Đa nền tảng, Cài đặt) | ✅ Code chạy thật |
| Studio AI — tạo video affiliate/phim ngắn từ dashboard | ✅ Code xong, chưa có verdict mắt người |
| Multi-Gemini API key rotation (round-robin, cooldown khi hết quota) | ✅ |
| Sổ cái per-account, Governance luật cứng, Scheduler, kill switch | ✅ |
| Agent Team: tree UI, trạng thái suy ra từ tiến độ job Studio (không phải agent thật đang chạy) | ✅ |
| Stream engine phát avatar thật | ⏳ Đang phát test pattern — cần benchmark trên Mac |
| Nối daemon → live (cổng dry-run/cadence) | ⏳ |
| ADB Hunter — quét Product Marketplace trên Android của bạn | ⏳ Cần Mac + máy của bạn |
| TikTok Shop API chính thức | ❌ Đã loại theo quyết định của bạn |
| Test E2E trên tài khoản thật | ⏳ Cần bạn |

**Test:** 298 test function / 14 package pass (không tag; 20 package với `-tags parked`), kiểm tra lần cuối 2026-10-02. Lệnh kiểm chứng:

```bash
go build ./... && go vet ./... && go test -count=1 ./...          # runtime chính
go test -count=1 -tags parked ./...                              # vùng parked
bash scripts/check-reachable.sh                                  # không package nào bị bỏ rơi
```

---

## 📚 Tài liệu

| Ai đọc | File |
|---|---|
| Người dùng (bạn) | [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md) — cài đặt, chạy trên Mac, từng trang dashboard, vận hành hằng ngày, hậu kỳ tay, kill switch, backup |
| Dev / AI code assistant (Claude, Agy, Codex) | [`docs/DEVELOPER.md`](docs/DEVELOPER.md) — cấu trúc repo, build/test, quy ước code, bản đồ kiến trúc |
| Mô hình kinh doanh | [`docs/MODEL.md`](docs/MODEL.md) |
| Thiết kế Agent Team | [`docs/AGENT_TEAM.md`](docs/AGENT_TEAM.md) |
| Kiến trúc kỹ thuật | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| Thiết kế phát triển kênh | [`docs/CHANNEL_GROWTH.md`](docs/CHANNEL_GROWTH.md) |
| Review dự án + spec các đợt sửa (đang điều phối) | [`docs/PROJECT_REVIEW.md`](docs/PROJECT_REVIEW.md) |
| Thiết kế UI/UX | [`docs/UI_UX_BLUEPRINT.md`](docs/UI_UX_BLUEPRINT.md) |
| Chính sách & an toàn | [`docs/POLICY_AND_SAFETY.md`](docs/POLICY_AND_SAFETY.md) |
| Kiến thức nền đã kiểm chứng (chính sách nền tảng, kiếm tiền, TTS, avatar, API) | [`docs/RESEARCH.md`](docs/RESEARCH.md) |

---

## 📦 Phát hành 1-binary

```bash
# Build phát hành có đóng dấu version (mặc định là "dev"):
go build -ldflags "-X main.version=v1.0.0" -o aicos ./cmd/aicos
./aicos -version   # aicos v1.0.0
```

- **1 binary / 1 nền tảng** (Windows/macOS/Linux, ~15MB khi strip): `GOOS=windows GOARCH=amd64 go build ./cmd/aicos`… — CI cross-build cả 5 đích tự động (`.github/workflows/ci.yml`).
- **Dữ liệu nằm gọn trong 1 thư mục** (`-data`, mặc định `./data`): 3 database SQLite, `content_jobs.json`, `tokens/` (OAuth TikTok/YouTube — app tự di trú file token cũ từ thư mục làm việc vào đây lần đầu chạy), `output/`, `avatars/`.
- **Lần đầu chạy** thư mục dữ liệu trống → app mở wizard `/onboard` (chọn chế độ thử, xong mới vào app).
- **Sao lưu/khôi phục** ở Cài đặt · Hệ thống: tải `.zip` ảnh chụp dữ liệu; khôi phục upload zip → kiểm chứng → khởi động lại app để áp dụng.
- **API `/api/*` mặc định TẮT** (bật bằng công tắc ở Cài đặt · Hệ thống khi cần).
- CI: build + vet + test (cả chế độ `parked`) + `scripts/check-reachable.sh` + cross-build 5 đích. Trên GitHub, bật *branch protection* cho `main` (Require status checks → `ci`) để PR fail không merge được.

## ⚖️ Giấy phép

All rights reserved — xem [`LICENSE`](LICENSE). Mọi quyền được bảo lưu; chưa cho phép sao chép/phân phối/sửa đổi khi chưa có văn bản đồng ý.
