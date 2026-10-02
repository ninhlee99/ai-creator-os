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
go test ./...               # toàn bộ test, dữ liệu giả, không chạm TikTok thật
```

- Ngôn ngữ: **100% Go**, 1 binary `aicos`. SQLite (pure Go, modernc driver). Không Docker, không Python runtime.
- Quy ước: `gofmt` trước khi commit. Binary release: `aicos-darwin-arm64` qua GitHub Release.

## Cấu trúc repo

```
cmd/aicos/            điểm vào binary
internal/web/         dashboard: handlers.go (routes), server.go, templates/*.html,
                      static/style.css — template embed qua //go:embed
internal/agents/      hunter, content, streamer, analyst...
internal/studio/      Studio AI: jobs (SQLite), storyboard, render
internal/orchestrator/daemon   vòng lặp agents
internal/ledger/      sổ cái SQLite WAL per-account
internal/governance/  luật cứng — code xác định, không LLM
internal/mediagen/    Gemini image/video gen (xoay key)
internal/trends/      trending sounds (Kworb, cache 6h)
internal/tiktok/      Shop API signing, Posting API client
docs/                 tài liệu (xem bảng ở README)
docs/assets/agents/   avatar từng agent/persona (PNG 512px)
vercel-preview/       bản preview tĩnh để test UI/UX trên Vercel
scripts/postprod.sh   QC + đóng gói Drive (chạy trên Mac)
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
- **Agent Team:** mỗi nhiệm vụ = 1 TeamInstance (taskID riêng, blackboard, ngân sách cứng). QC 3 lớp chấm mù, tối đa 2 vòng sửa. Publisher là cửa ghi duy nhất. Chi tiết: [`AGENT_TEAM.md`](AGENT_TEAM.md).
- **Provider chains:** TTS Gemini → VieNeu-TTS v3 local → Edge; LLM Gemini → llama-server local. Keyring xoay vòng, cooldown khi 429/quota, skip key hỏng. Chỉnh trong Settings.
- **Affiliate:** ADB quét Product Marketplace trên Android (không dùng TikTok Shop API — đã loại theo quyết định user). Video 30s = 3–5 ảnh 4K cùng 1 địa điểm, zoom nảy theo BPM, không chữ không voiceover.

Chi tiết đầy đủ: [`ARCHITECTURE.md`](ARCHITECTURE.md) · pipeline affiliate: `ARCHITECTURE.md` §13

## 9. Ràng buộc & benchmark trên Mac của Ninh (MacBook Pro M1 Pro 32GB)

Trích từ audit lõi AI của chính dự án (2026-10-02, file audit điểm-thời đã
gộp vào đây và `docs/RESEARCH.md` §5–§6):

- **Cài đặt phía Mac (guide người dùng chưa liệt kê đủ trước đây):** ngoài
  `brew install ffmpeg`, tầng local cần `brew install llama.cpp` (bản
  Metal) cho llama-server và `uv` cho VieNeu-TTS. Thiếu cái nào thì tầng
  đó **tắt lặng** — chỉ ghi log, không báo UI. Đừng để giới thiệu sai cho
  người dùng rằng local "có sẵn".
- **Lệnh khởi chạy llama-server phải có `-ngl 99`** để offload lên GPU
  Metal; thiếu là chạy CPU 100% (ước 8–15 tok/s thay vì 25–45 tok/s, chưa
  đo thật trên máy Ninh). Đã sửa, đừng hồi quy.
- **Mọi con số hiệu năng hiện tại là ƯỚC TÍNH, chưa phải số đo trên máy
  Ninh:** llama tok/s, RTF VieNeu fp32/int8 (chờ A/B nghe thử), fps
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
