# UI_UX_BLUEPRINT — Thiết kế UI/UX hiện tại + đích

> Cập nhật 2026-10-02 (sau Đợt A): sidebar 7 mục, live + phim đã park.
> Đích 8 mục theo `docs/PIVOT_REDESIGN.md` §1 sẽ tới dần ở các đợt B→F.
>
> Nguyên tắc xuyên suốt: ít chữ (card/modal/tooltip/toast), trạng thái = màu +
> icon trầm, palette nền xám ấm `#F5F4F1` + khung trắng/xám + chữ đen-xám ấm +
> nhấn chàm–slate `#525F8A`. Mỗi khả năng vận hành đều thao tác được từ UI
> (không CLI-only). Tiếng Việt ngắn gọn.

## 1. Sidebar hiện tại (7 mục — `internal/web/templates/base.html`)

| # | Mục | Route | Câu hỏi duy nhất nó trả lời |
|---|---|---|---|
| 1 | Trang chủ | `/` | "Hôm nay hệ thống có khỏe không, có gì đang chặn?" |
| 2 | Kênh | `/accounts` | "Có những kênh nào, đang ở trạng thái gì?" |
| 3 | Phát triển kênh | `/growth` | "Kênh đang tăng trưởng thế nào, luật nào đang giữ?" |
| 4 | Studio AI 🎬 | `/studio` | "Tạo một video affiliate mới" (+ video chữ động, jobs, trends) |
| 5 | Affiliate | `/products` | "Kho có sản phẩm nào dùng được?" (rework Accesstrade ở đợt B/C) |
| 6 | Đa nền tảng | `/publishers` | "Kênh nào đã nối được nền tảng nào?" |
| 7 | Cài đặt | `/settings` | 4 trang con (dưới) |

Không link chết: `/schedule` đã gỡ khỏi sidebar (park → 404). `/team` đã định nghĩa lại ở đợt H2 (Đội ngũ — 3 pipeline).

## 2. Cài đặt — 4 trang con (`internal/web/templates/settings/`)

| Trang | Câu hỏi duy nhất |
|---|---|
| Hệ thống (`he-thong`) | "App đang chạy chế độ gì?" (version, chi phí API, ngân sách, env, master switch, API nội bộ, sao lưu) |
| Nhà cung cấp (`nha-cung-cap`) | "AI dùng nhà cung cấp nào trước, key nào còn sống?" (chuỗi TTS/LLM + keyring) |
| Model local (`model-local`) | "Model local đã sẵn sàng chưa?" (VieNeu, khả năng vẽ ảnh, công cụ hệ thống) |
| An toàn (`an-toan`) | "Có đang ở chế độ an toàn không?" (dry-run, kill switch) |

Tab Nhân vật AI đã gỡ (park theo avatar live).

## 3. Đích 8 mục (PIVOT_REDESIGN.md §1) — lộ trình

| Mục đích | Từ | Khi |
|---|---|---|
| Affiliate (`/products` cải tiến sâu) | hiện tại | Đợt B/C: tab Chiến dịch AT · Sản phẩm · Link · Đối soát |
| **Reup** (`/reup` mới) | chưa có | Đợt D/E |
| **Kể chuyện** (`/stories` mới) | chưa có | Đợt F |
| Studio AI thu gọn | hiện tại | Đợt F: 2 mode (affiliate + kể chuyện) |
| Agent Team định nghĩa lại | ✅ đợt H2 | `/team` — Đội ngũ 3 pipeline |

## 4. Quy ước component

- Template: `templates/<page>.html` (`{{define "title"}}` + `{{define "content"}}`),
  route trong `routes.go`, handler theo miền, `static/app.js` chung (toast,
  modal, confirm), `static/style.css` chung.
- Template không dùng nữa → `internal/web/templates_parked/` (ngoài `go:embed`).
- Badge trạng thái trung thực: đọc thật từ code (probe/exec/file tồn tại),
  không đoán. Số "ước tính chưa kiểm chứng" phải ghi rõ.
- Mọi POST lỗi → redirect về đúng trang kèm `?err=` + toast (không redirect câm).
