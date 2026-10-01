# Operations

## Install (Mac M1 Pro)

```bash
# 1. deps (chỉ FFmpeg là bắt buộc; Go chỉ cần khi build từ source)
brew install ffmpeg

# 2. chạy — 1 binary duy nhất, không cài Python/venv/pip
./aicos                       # mở http://localhost:8080
# model local (VieNeu-TTS v3, LLM GGUF): bấm "tải" trong trang Settings
```

Build từ source (nếu cần):

```bash
brew install go
go build -o aicos ./cmd/aicos
```

## Go-live checklist (all must be true)

- [ ] `go test ./...` passes
- [ ] Dashboard Settings: TTS/LLM provider chain + API keys đã cấu hình
- [ ] TikTok account: Shop Creator + LIVE permission verified
- [ ] `DRY_RUN=false` set deliberately (trong Settings)
- [ ] Telegram alerting tested (`TELEGRAM_BOT_TOKEN`/`CHAT_ID`)
- [ ] AI disclosure text reviewed (`AI_DISCLOSURE_TEXT`)
- [ ] First live session supervised via dashboard at :8080

## Credential setup (human steps — code is ready, keys are not)

Code status: TTS chain wired (Gemini→VieNeu-TTS v3→Edge), LLM chain
(Gemini→llama-server), Posting API client complete, Shop API signing
complete. Provider order, keys và retry policy chỉnh trong dashboard
Settings (lưu vào bảng `settings`, áp dụng ngay không cần restart).
These need your accounts/keys:

| # | What | Where to get it | Env vars | Lead time |
|---|---|---|---|---|
| 1 | **Gemini API key** (AI Studio) — expressive Vietnamese TTS | aistudio.google.com | `TTS_API_KEY` | minutes |
| 2 | **TikTok Shop Partner Center app** (Affiliate type) | partner.tiktokshop.com | `TIKTOK_SHOP_APP_KEY`, `TIKTOK_SHOP_APP_SECRET` | app review: days–weeks |
| 3 | Creator OAuth → access token + shop cipher | Seller Center / TikTok app authorization | `TIKTOK_SHOP_ACCESS_TOKEN`, `TIKTOK_SHOP_CIPHER` | with #2 |
| 4 | Verify affiliate endpoint paths in sandbox | TikTok Shop Partner Center sandbox | fill `ENDPOINTS` in `internal/tiktok` shop client | 1 session |
| 5 | **TikTok Developers app** (Login Kit + Content Posting API) | developers.tiktok.com | `TIKTOK_CLIENT_KEY`, `TIKTOK_CLIENT_SECRET` | audit ~3–4 weeks for public Direct Post |
| 6 | OAuth per creator account → refresh token | dashboard **Đa nền tảng → Kết nối TikTok** (xem `docs/POSTPROD_RUNBOOK.md`) | `tiktok_token_<account>.json` (gitignored) | minutes/account |
| 7 | **RTMP server + stream key per account** | TikTok LIVE Center → Go LIVE | `RTMP_KEY_<USERNAME>` (env only, never in code) | minutes/account |

Notes:
- Without #1, TTS falls back to Edge (no key, unofficial endpoint — last resort).
- Without #5's audit, the Posting API only publishes `SELF_ONLY` (private) to ≤5 test users.
- Token files and `.env` are gitignored; keys never enter the ledger or logs.

## Chạy app trên Mac (mở tay bằng CLI, không Docker)

1. Tải file `aicos-darwin-arm64` từ mục Releases trên GitHub về máy Mac.
2. Mở Terminal, cấp quyền chạy: `chmod +x aicos-darwin-arm64`
3. Chạy: `./aicos-darwin-arm64`
4. Mở trình duyệt: http://127.0.0.1:8080 — mọi quản lý/giám sát đều ở dashboard UI.
5. Muốn tắt: `Ctrl+C`. Dữ liệu nằm trong thư mục `data/` cạnh binary.

Hậu kỳ thủ công mỗi video (gắn giỏ hàng, CapCut, QC, upload Drive): `docs/POSTPROD_RUNBOOK.md` + `scripts/postprod.sh`.

## Daily operation (hands-off)

1. Mở app khi muốn hệ thống ON; orchestrator chạy agents theo lịch, stream theo lịch LIVE.
2. Dashboard `:8080` shows commission, spend, decisions.
3. Telegram alerts on: stream death, quota near-limit, kill events, money-path errors.
4. Human reviews `/decisions` weekly; tunes thresholds in dashboard Settings.

## Kill switch

Dashboard button, or: `kill -USR1` — orchestrator stops agents and stream
within seconds. Ledger keeps the audit trail.

## Backup

Nightly: `sqlite3 data/ledger.db ".backup data/backup/ledger-$(date +%F).db"`
Retain 30 days. Config backup alongside.
