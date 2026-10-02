# Hướng dẫn chung — quay video demo aicos

> Kịch bản đọc theo app thật tại commit `c457999` (sau Đợt G).
> Mỗi tính năng = 1 video, độ dài 2–5 phút.

## 1. Chuẩn bị máy quay (làm 1 lần)

1. Build binary: `go build -o aicos ./cmd/aicos` (macOS ARM: `GOARCH=arm64`; Windows: thêm `.exe`).
2. Data dir sạch riêng cho demo: `./aicos -data ./demo-data` → mở `http://localhost:8080`.
3. Lần đầu → wizard `/onboard` → chọn chế độ **thử (dry-run)** → vào app.
4. **BẬT dry-run** ở Cài đặt · An toàn trước khi quay: mọi nút "Đăng"/"Đồng bộ" sẽ chỉ mô phỏng, không thao tác thật.
5. Chuẩn bị sẵn để demo mượt:
   - 1 file `GEMINI_API_KEYS` (free tier) trong Cài đặt · Nhà cung cấp — nếu không có, các job AI sẽ fail-closed với lỗi trung thực "media-gen chưa sẵn sàng" (vẫn quay được, nhưng video sẽ chỉ thấy trạng thái lỗi — khuyến nghị có key thật).
   - 1 file nhạc `autopilot-music.m4a` (licensed) để sẵn trong `demo-data/` cho các video dựng.
   - 1 Accesstrade access key thật (nếu muốn demo "Tải chiến dịch" chạy thật).
   - YouTube OAuth **chưa cần** — demo ở mức fail-closed "chờ kết nối" là đúng.

## 2. Thứ tự quay (theo sidebar)

| # | File | Video | Ước lượng |
|---|---|---|---|
| 0 | `00-huong-dan-chung.md` | (file này) | — |
| 1 | `01-trang-chu.md` | Trang chủ | 2–3′ |
| 2 | `02-kenh.md` | Kênh | 3–4′ |
| 3 | `03-phat-trien-kenh.md` | Phát triển kênh | 4–5′ |
| 4 | `04-studio-ai.md` | Studio AI | 4–5′ |
| 5 | `05-affiliate.md` | Affiliate (Accesstrade) | 4–5′ |
| 6 | `06-reup.md` | Reup Douyin | 4–5′ |
| 7 | `07-ke-chuyen.md` | Kể chuyện YouTube | 4–5′ |
| 8 | `08-da-nen-tang.md` | Đa nền tảng | 2–3′ |
| 9 | `09-cai-dat.md` | Cài đặt (6 tab) | 4–5′ |

## 3. Quy tắc khi đọc thuyết minh

- Đọc đúng chữ trong ngoặc kép của kịch bản; thao tác màn hình đúng thứ tự.
- Gặp trạng thái "chờ key" / "chờ số liệu" / "chờ kết nối": **đọc to câu trong kịch bản**, không bỏ qua — đó là tính trung thực của app.
- Số liệu trên màn hình demo: nói rõ "đây là dữ liệu demo" khi không phải số thật.
- Tuyệt đối không nói: "đảm bảo 100% không bản quyền", "đảm bảo lên xu hướng", "đảm bảo thu nhập".
- Mỗi video kết thúc bằng 1 câu: tính năng này tự chạy tiếp thế nào khi bạn chỉ mở máy + start app.
