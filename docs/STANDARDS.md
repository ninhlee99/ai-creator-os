# STANDARDS — Chuẩn tuân thủ nghiêm ngặt toàn dự án

Mọi code, README, kiến trúc, cấu trúc, tài liệu, UI đều phải tuân thủ tài liệu
này. Vi phạm = làm lại. (Ninh chốt 2026-10-02.)

## 1. Kiến trúc

- **1 binary duy nhất** cho mỗi OS/arch (`aicos`). Pure Go — cấm cgo mới,
  cấm runtime Python/Node đi kèm. Không yêu cầu model runtime local; tiến trình
  phụ chỉ dùng trong vùng build `parked`.
- **Package độc lập**: mỗi package một nhiệm vụ rõ, không import vòng.
  Package chưa dùng → park bằng build tag `parked`, không để code chết trong
  build thường. `scripts/check-reachable.sh` phải OK.
- **SQLite WAL**, một DB một nhiệm vụ (`ledger`, `studio`, …). Migration qua
  `PRAGMA user_version`, có test round-trip. Không bịa dữ liệu: thiếu nguồn →
  fail-closed + ghi lý do.
- **Config một nguồn**: ledger settings + env overlay; lưu qua UI có hiệu lực
  ngay, không restart.

## 2. Cấu trúc

- Go: `cmd/aicos` (mỏng) → `internal/<domain>` (logic) → `internal/web`
  (HTTP). File >700 dòng phải tách theo miền.
- Web: `templates/<page>.html` (`{{define "title"}}` + `{{define "content"}}`),
  route trong `routes.go`, handler theo miền, `static/app.js` chung (toast,
  modal, confirm), `static/style.css` chung.
- Template/asset không dùng nữa: chuyển vào thư mục `parked/` ngoài
  `go:embed`, không để file chết trong embed.
- Không file test đặt tên chung chung; test phải chứng minh hành vi thật
  (render FFmpeg thật, DB thật), không test mock rỗng.

## 3. Tài liệu (README, docs)

- **README.md chính**: mô tả đúng trạng thái hiện tại của app (tính năng,
  cách chạy, yêu cầu). Không được mô tả tính năng đã park như còn hoạt động.
- **`docs/features/<feature>.md`**: mỗi tính năng một file — cách kích hoạt,
  luồng code chính xác (tên file/hàm), tương tác SQLite/API, xử lý fail-closed,
  key rotation liên quan. Viết bằng cách **đọc code**, không bịa.
- **Quy tắc "docs đi cùng code"**: wave nào sửa code thì wave đó cập nhật docs
  liên quan, commit riêng. Docs sai so với code = bug, ưu tiên sửa như bug code.
- Sơ đồ kiến trúc (`ARCHITECTURE.md`, SVG) phải khớp package/route thực tế.

## 4. UI/UX

- Sidebar theo `docs/PIVOT_REDESIGN.md` §1 (10 mục: Trang chủ, Kênh, Phát triển
  kênh, Đội ngũ, Studio AI, Affiliate, Reup, Kể chuyện, Đa nền tảng, Cài đặt).
  Không link chết.
- Ít chữ: card/modal/tooltip/toast; trạng thái = màu + icon (trầm, không chói).
- Palette: nền xám ấm `#F5F4F1`, khung trắng/xám, chữ đen-xám ấm, nhấn
  chàm–slate trầm `#525F8A`. Mỗi trụ một màu nhận diện nhẹ ở badge/icon.
- Tiếng Việt, ngắn gọn, đúng chính tả. Không đoạn giải thích dài trong UI.
- Mọi khả năng vận hành đều thao tác được từ UI (không CLI-only).
- Số liệu ước tính chưa đo phải ghi "ước tính chưa kiểm chứng".

## 5. Quy trình làm việc

- KHÔNG BAO GIỜ `git add -A` trong repo (docs/assets/ cố ý untracked).
- Commit riêng từng phần đã verify (build + test xanh), stage đường dẫn
  explicit, push ngay. Commit message tiếng Việt, ghi rõ đã tự review gì.
- Không tuyên bố xong khi chưa verify độc lập (build/vet/test/boot thử).

## 6. Vận hành (zero-touch)

- Ninh không can thiệp vận hành: mọi quyết định (provider, fallback, lịch,
  kill/double-down) do hệ thống tự quyết theo rule đã định.
- Fail-closed + dry-run + master switch + kill switch luôn hoạt động.
- **Free tuyệt đối**: key Gemini free-only, không billing, không thử Veo.
  Quota: xoay key, cooldown 60s→5m→15m, resume khi hết quota ngày.
- Không dùng Claude/Codex/Agy subscription của Ninh trong app hay thay app
  điều khiển máy.
- Trung thực với Ninh: báo kết quả đã verify; không hứa điều không chắc
  (VD: "đảm bảo 100% không bị đánh bản quyền" là cấm).

## 7. Audit

Mỗi đợt (wave) kết thúc bằng audit theo checklist `~/AGENTS.md` + tài liệu này.
Đợt G có audit toàn dự án: kiến trúc, README, docs, UI, cấu trúc — chấm điểm,
<85/100 làm lại.

## 8. Chuẩn Video Có Hồn & Quay Phim Tự Nhiên (Natural Cinematic Standard)

Tuân thủ nghiêm ngặt theo tài liệu Open-Spec: `docs/OPEN_SPEC.md`.
- **Chống méo hình học (Anti-Distortion)**: 100% video xuất ra phải đạt chuẩn 9:16 (`1080x1920`) hoặc 16:9 (`1920x1080`) qua thuật toán Scale & Crop Lanczos (`force_original_aspect_ratio=increase,crop=W:H,setsar=1`). Tuyệt đối cấm co dãn méo mặt/vật thể.
- **Chuyển động người thật (Living Subject)**: Affiliate video ưu tiên video lookbook chuyển động thật và ghép mặt InsightFace. Tuyệt đối không dán ảnh tĩnh sản phẩm với thẻ chữ to dày rồi zoompan lắc máy thô thiển.
- **Affiliate editing**: Footage mặc định là video chuyển động thật, camera ổn định. Cấm zoom giả, flash, flicker, whip-pan và rung cơ học; ảnh tĩnh chỉ dùng dissolve nhẹ.
- **Nguồn vũ đạo triệu view**: Video nhảy viral lấy trực tiếp từ TikTok Việt Nam và Douyin Trung Quốc (抖音), bắt đúng nhạc hot trend.
- **Truyện Audio điện ảnh**: Truyện dài 30-60 phút phải có visual canvas đa cảnh liên tục (8-15s/cảnh), hòa âm 3 lớp (giọng đọc Gemini/Edge + Foley môi trường + nhạc nền). Tuyệt đối cấm để video nền đen.

