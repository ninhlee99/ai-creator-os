# MiuNova — Hướng dẫn phát triển

> Nguồn yêu cầu sản phẩm: [PRODUCT_SPEC](PRODUCT_SPEC.md). Chính sách hard
> gate: [POLICY_AND_SAFETY](POLICY_AND_SAFETY.md). Trạng thái code đã đối chiếu:
> [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md). Mô tả as-built theo miền:
> [features/README](features/README.md).

## 1. Khởi chạy từ source

Yêu cầu phiên bản Go theo `go.mod` (hiện khai báo Go 1.26.0). Build và chạy:

```bash
go build -o aicos ./cmd/aicos
./aicos
```

FFmpeg là công cụ ngoài cần cho tác vụ dựng video. Cấu hình môi trường/secret
qua Settings hoặc cơ chế deploy được dự án hỗ trợ; không commit credential.

## 2. Thứ tự đọc trước khi sửa

1. Đọc `PRODUCT_SPEC.md` để biết hành vi đích.
2. Đọc `PRODUCT_DECISIONS.md` để phân biệt quyết định người dùng với giả định.
3. Đọc `IMPLEMENTATION_STATUS.md` và source liên quan để biết khả năng hiện tại.
4. Đọc `POLICY_AND_SAFETY.md` trước khi thay đổi account, provider, nội dung,
   xuất bản hoặc quota.
5. Nếu hành vi hiện tại và mục tiêu khác nhau, cập nhật evidence trong
   `IMPLEMENTATION_STATUS.md`; không ghi feature as-built như thể mục tiêu đã
   hoàn tất.

## 3. Ranh giới cứng

- Chỉ dùng nguồn miễn phí được phép; provider cần billing hoặc giá không rõ
  không được gọi. Fallback giữa account chỉ khi account đã được ủy quyền và
  provider cho phép; không né quota.
- Xuất bản qua API/quyền hợp lệ. TikTok cần consent riêng cho từng bài theo
  PRODUCT_SPEC; không bỏ consent để tiện tự động hóa.
- Không bịa sales, rating, metrics, claim hoặc trạng thái publish. Giữ provenance
  và freshness của dữ liệu; thiếu field phải biểu diễn là unknown ở sản phẩm đích.
- OAuth/API credential không được render hoặc ghi log.
- Web AI remote login dùng screenshot/input bridge qua app; CDP chỉ bind
  loopback. VPS live login vẫn cần xác minh theo
  `features/remote-webai-login.md`; không tuyên bố VPS đã xác nhận nếu chưa có
  bằng chứng chạy thật.

## 4. Bản đồ source

| Thư mục | Trách nhiệm chính |
|---|---|
| `cmd/aicos` | Entrypoint và wiring ứng dụng |
| `internal/web` | HTTP routes, handlers, templates, static assets và dashboard |
| `internal/automation`, `internal/ticksup` | Ticks, lịch và tác vụ nền |
| `internal/harness`, `internal/growth` | Điều phối quan sát/quyết định và kế hoạch tăng trưởng |
| `internal/products`, `internal/accesstrade`, `internal/affiliatehunter` | Catalog, offer discovery, affiliate links và đối soát |
| `internal/studio`, `internal/viralradar` | Job media, shot plan, QC và radar |
| `internal/publishers`, `internal/tiktok` | Adapter xuất bản theo nền tảng |
| `internal/webai` | Account/session AI web, login capture và provider flows |
| `internal/ledger`, `internal/backup` | Trạng thái bền vững, settings, backup/restore |

Các vai trò trong [SYSTEM_BLUEPRINT](SYSTEM_BLUEPRINT.md) là mô hình công việc
mục tiêu. Không suy ra mỗi vai trò tương ứng với một process hoặc agent độc lập;
xem harness/worker wiring thật trong source.

## 5. Quy trình thay đổi

- Giữ thay đổi ở đúng layer; ưu tiên dùng adapter/interface hiện có.
- Cập nhật tài liệu tính năng nếu hành vi as-built đổi; cập nhật spec/decisions
  nếu yêu cầu hoặc lựa chọn sản phẩm đổi.
- Khi thêm route, provider, trạng thái job hoặc nguồn metrics, ghi quyền, lỗi,
  quota, retry/idempotency và cách người vận hành nhìn thấy kết quả.
- Dùng `gofmt` cho Go. Chạy kiểm tra liên quan theo yêu cầu của task; các lệnh
  thông dụng là `go test ./...` và `go test -tags parked ./...`.
- Kiểm tra `git diff --check`; không đưa credential, dữ liệu session, profile
  browser hay artifact người dùng vào git.

## 6. Tài liệu chuẩn

- [SYSTEM_BLUEPRINT](SYSTEM_BLUEPRINT.md): kiến trúc mục tiêu, vai trò và job flow.
- [README](../README.md): tóm tắt sản phẩm và cách chạy.
- [USER_GUIDE](USER_GUIDE.md): vận hành theo khả năng hiện tại.
- [ARCHITECTURE](ARCHITECTURE.md): ghi chú kiến trúc code kế thừa; đối chiếu
  source và IMPLEMENTATION_STATUS trước khi dùng làm bằng chứng.
