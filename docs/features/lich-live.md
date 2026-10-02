# 🅿️ PARKED — Lịch live (`/schedule`)

**Trạng thái: đã park từ Đợt A (commit `efa218b`, 2026-10-02).** Route
`/schedule` không còn trong binary → mở trang này trả **404**. Ninh chốt bỏ
live ngày 2026-10-02 (xem `docs/PIVOT_REDESIGN.md` §0).

## Code còn lại ở đâu

- Handler: `internal/web/schedule.go` — có `//go:build parked` (không biên dịch).
- Template: `internal/web/templates_parked/schedule.html` (ngoài `go:embed`).
- Stream engine: `internal/stream/` — `//go:build parked`.
- Avatar: `internal/engines/avatar/` — `//go:build parked`.
- Daemon mạng (`internal/network/daemon.go`) vẫn tick 60s nhưng **không nối
  `OnStartLive`** (chỉ test mới nối) → không mở live thật.
- Test vùng parked: `go test -tags parked ./...`.

## Logic cũ (để tham khảo khi cần lôi lại)

Trước khi park: `ensureTodaySchedule()` tự build lịch live mỗi ngày khi mở
Trang chủ; `buildTodaySchedule()` xếp slot cho tài khoản `live_ready`/`live`
vào bảng `live_slots`; daemon mạng tick 60s quyết định slot nào lên live dưới
cổng master/kill/dry-run. Chi tiết đầy đủ nằm trong git history
(`git log -- internal/web/schedule.go`).
