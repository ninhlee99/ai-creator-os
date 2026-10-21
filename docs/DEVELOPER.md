# Developer Guide

Dành cho dev và AI code assistant (Claude, Agy, Codex) làm việc trên repo này. Người dùng đọc [`USER_GUIDE.md`](USER_GUIDE.md).

## Luật cứng (không thương lượng)

1. **App chỉ dùng API chính thức đã cấu hình trong Settings** (Gemini keys, v.v.). Tuyệt đối không dùng Claude/Codex/Agy subscription, không bóc tách API web muse.ai, không dùng Gemini/ChatGPT/Claude bản web làm backend trong app. Các công cụ đó chỉ để code/build app.
2. **Không bao giờ `git add -A`.** Thư mục `docs/assets/` có file cố ý untracked chờ quyết định giữ/xóa. Stage từng đường dẫn cụ thể.
3. **Mọi tính năng vận hành phải có UI.** Không quản lý hệ thống bằng CLI.
4. **Code không dùng nữa thì xóa.** Giữ codebase gọn.
5. **Fail-closed:** thiếu config/key → chặn và báo, không đoán, không tự bịa dữ liệu (doanh thu chỉ ghi từ API thật).

## Build & test

```bash
go build ./...              # build toàn repo
go build -o aicos ./cmd/aicos
go test -count=1 ./...      # toàn bộ test, dữ liệu giả, không chạm TikTok thật
go test -count=1 -tags parked ./...   # test vùng parked (agents/stream/affiliatehunter)
bash scripts/check-reachable.sh       # package mới không được "chết lặng"
```

- Ngôn ngữ: **100% Go**, 1 binary `aicos`. SQLite (pure Go, modernc driver). Không Docker, không Python runtime.
- Quy ước: `gofmt` trước khi commit. Binary release: `aicos-darwin-arm64` qua GitHub Release, đóng dấu version: `go build -ldflags "-X main.version=vX.Y.Z" ./cmd/aicos` (mặc định `dev`).
- CI (`.github/workflows/ci.yml`): build + vet + test (2 chế độ tag) + reachability + cross-build 5 đích. Bật branch protection cho `main` trên GitHub (Require status checks → `ci`).
- Test hiện tại: 576+ hàm pass 100% — cập nhật con số này mỗi khi thêm test mới.

## Cấu trúc repo

```
cmd/aicos/            điểm vào binary
internal/web/         dashboard: routes.go, server.go, templates/*.html, static/style.css
internal/growth/      Brand DNA, Trend Intelligence, Kế hoạch 30 ngày, YouTube Analytics
internal/accesstrade/ Hunter quét chiến dịch & datafeed, tạo link tracking, đối soát hoa hồng
internal/studio/      Studio AI: jobs (SQLite), video affiliate, video viral AI 9:16, story video 16:9
internal/automation/  Vòng lặp daemon: Autopilot, Growth, Reconcile, Sync, Story
internal/ledger/      Sổ cái SQLite WAL per-account & cài đặt hệ thống
internal/engines/     LLM (Gemini), TTS chain (Gemini/Edge)
internal/publishers/  TikTok Posting API, YouTube OAuth/upload, Facebook Page
docs/                 Tài liệu hệ thống và 18 tài liệu tính năng chi tiết
scripts/              Watchdog healthz, check-reachable, launchd service
```

## Thêm một trang dashboard mới

1. Tạo `internal/web/templates/<page>.html` (dùng `{{define "title"}}` + `{{define "content"}}`).
2. Thêm `"key": "<page>.html"` vào `pageFiles` trong `server.go`.
3. Thêm route `GET /key` trong `handlers.go` + handler.
4. Thêm link vào sidebar `templates/base.html` (đúng nav-group).
5. Thêm CSS vào `static/style.css` (đã embed vào binary — sửa là có hiệu lực ngay khi rebuild).
6. Thêm case vào `TestAllPagesRender` trong `web_test.go`.

Avatar agent: file JPEG 160px trong `internal/web/static/agents/`, serve qua `GET /static/agents/{file}` (embed). Gốc PNG ở `docs/assets/agents/`.

## Bản đồ kiến trúc (tóm tắt)

- **Orchestrator** điều phối; **AccountManager** vòng đời account; **PersonaEngine** gán persona; **Scheduler** xếp giờ live (max 2 live cùng lúc).
- **Đội ngũ (`/team`):** bảng sức khỏe của ba pipeline Affiliate / Video dọc / Story; trạng thái suy ra từ dữ liệu thật, không phải agent chạy riêng. Chi tiết: [`features/agent-team.md`](features/agent-team.md).
- **Provider chains:** TTS Gemini → Edge; LLM Gemini. Keyring xoay vòng, cooldown khi 429/quota, skip key hỏng. Chỉnh trong Settings.
- **Affiliate:** nguồn duy nhất Accesstrade datafeed (hunter tick → kho; TikTok Shop đã loại bỏ Đợt H1). Mặc định tạo từng shot bằng engine video; prompt cấm zoom, flash, flicker và rung. Nếu dùng ảnh tĩnh, dựng dissolve ổn định, không tạo chuyển động giả.

Chi tiết đầy đủ: [`ARCHITECTURE.md`](ARCHITECTURE.md) · pipeline affiliate: `ARCHITECTURE.md` §13

## 9. Ràng buộc & benchmark trên Mac của Ninh (MacBook Pro M1 Pro 32GB)

Trích từ audit lõi AI của chính dự án (2026-10-02, file audit điểm-thời đã
gộp vào đây và `docs/RESEARCH.md` §5–§6):

- **Cài đặt phía Mac:** `brew install ffmpeg` cho dựng video. LLM dùng Gemini;
  không cần cài model server local.
- **Mọi con số hiệu năng hiện tại là ƯỚC TÍNH, chưa phải số đo trên máy
  Ninh:** fps
  MuseTalk thật (ước 2.5–4 fps → render 60s ≈ 6–10 phút, ngoại suy từ M2
  Pro đo thật 1,6 fps — xem `docs/RESEARCH.md` §6), thời gian upscale 4K.
  Benchmark these đúng máy của chủ sở hữu là việc đang nợ; không được lấy
  số ước tính làm cam kết với người dùng.
- **Avatar local không realtime** (chốt: `docs/RESEARCH.md` §6): MuseTalk
  chạy offline qua sidecar; sidecar phải được cài riêng trên Mac (binary
  `avatar-server` trong `data/third_party/avatar-sidecar`, contract
  `/render`, `/stream/*` — xem `internal/engines/avatar/sidecar.go`).
  Encoder video ưu tiên `h264_videotoolbox` trên Mac, fallback libx264;
  upscale 4K là lanczos+unsharp, **không** phải chi tiết AI thật — tài
  liệu/mô tả phải nói đúng như vậy.

## Những gì đang treo (cần Mac/Ninh — xem `ARCHITECTURE.md` §12)

- Nối daemon → live (cổng dry-run/cadence)
- Stream engine phát avatar thật (đang phát test pattern)
- Benchmark MuseTalk trên Mac M1 Pro 32GB
- Cắm ADB hunter vào products/UI, chạy trên Android của Ninh
- QC thành agent độc lập
- E2E tài khoản thật + verdict mắt người về chất lượng mẫu
