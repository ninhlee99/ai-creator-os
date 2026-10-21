# UI_UX_BLUEPRINT — Thiết kế UI/UX hiện tại

> Cập nhật 2026-10-20: khớp sidebar và route đang đăng ký trong `internal/web/routes.go`.
>
> Nguyên tắc xuyên suốt: ít chữ (card/modal/tooltip/toast), trạng thái = màu +
> nhãn rõ, palette slate trung tính `#F8FAFC` + khung trắng + chữ than `#0F172A` +
> nhấn xanh `#2563EB`. Mỗi khả năng vận hành đều thao tác được từ UI
> (không CLI-only). Tiếng Việt ngắn gọn.

## 1. Sidebar hiện tại (`internal/web/templates/base.html`)

| # | Mục | Route | Câu hỏi duy nhất nó trả lời |
|---|---|---|---|
| 1 | Tạo nội dung | `/` | Tạo nhanh hoặc chọn loại video |
| 2 | Video | `/studio/jobs` | Theo dõi bản dựng và file |
| 3 | Sản phẩm | `/products` | Tìm deal và quản lý kệ hàng |
| 4 | Kênh | `/accounts` | Quản lý kênh và kết nối |
| 5 | Cài đặt | `/settings` | Cấu hình app |

Affiliate, video dọc và kể chuyện mở từ trang Tạo nội dung. Các trang vận hành
chuyên sâu như `/growth`, `/harness`, `/team` không chiếm vị trí điều hướng chính.

## 2. Cài đặt (`internal/web/templates/settings/`)

| Trang | Câu hỏi duy nhất |
|---|---|
| Hệ thống (`system`) | App đang chạy chế độ gì? (version, ngân sách, env, API, sao lưu, Web AI) |
| Nhà cung cấp (`providers`) | AI dùng nhà cung cấp nào trước, key nào còn sống? |
| An toàn (`safety`) | Có đang ở chế độ an toàn không? (dry-run, kill switch) |
| Accesstrade (`/settings/accesstrade`) | Kết nối và tự động hóa Accesstrade thế nào? |

Tab Nhân vật AI và lịch live đã park; template cũ nằm ngoài `go:embed`.
Phim điện ảnh cũng đã park, không phải pipeline trong binary mặc định.

## 4. Quy ước component

- Template: `templates/<page>.html` (`{{define "title"}}` + `{{define "content"}}`),
  route trong `routes.go`, handler theo miền, `static/app.js` chung (toast,
  modal, confirm), `static/style.css` chung.
- Template không dùng nữa → `internal/web/templates_parked/` (ngoài `go:embed`).
- Badge trạng thái trung thực: đọc thật từ code (probe/exec/file tồn tại),
  không đoán. Số "ước tính chưa kiểm chứng" phải ghi rõ.
- Mọi POST lỗi → redirect về đúng trang kèm `?err=` + toast (không redirect câm).
