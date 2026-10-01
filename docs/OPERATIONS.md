# Operations

## Install (Mac M1 Pro)

```bash
# 1. deps
brew install ffmpeg go python@3.11 ollama
ollama pull qwen3:4b

# 2. repo
git clone <this-repo> /opt/ai-creator-os
cd /opt/ai-creator-os
cp .env.example .env   # fill keys

# 3. stream engine
cd apps/stream-engine && go build -o ../../bin/stream-engine . && cd ../..

# 4. rehearsal (touches nothing external)
python3 -m apps.orchestrator.main --rehearse
```

## Go-live checklist (all must be true)

- [ ] `python3 -m apps.orchestrator.main --rehearse` passes
- [ ] `.env` filled: TikTok Shop token, RTMP url/key, TTS provider
- [ ] TikTok account: Shop Creator + LIVE permission verified
- [ ] `DRY_RUN=false` set deliberately
- [ ] Telegram alerting tested (`TELEGRAM_BOT_TOKEN`/`CHAT_ID`)
- [ ] AI disclosure text reviewed (`AI_DISCLOSURE_TEXT`)
- [ ] First live session supervised via dashboard at :8080

## Credential setup (human steps — code is ready, keys are not)

Code status: TTS chain wired (Gemini→VieNeu→Edge), Posting API client
complete, Shop API signing complete. These need your accounts/keys:

| # | What | Where to get it | Env vars | Lead time |
|---|---|---|---|---|
| 1 | **Gemini API key** (AI Studio) — expressive Vietnamese TTS | aistudio.google.com | `TTS_API_KEY` | minutes |
| 2 | **TikTok Shop Partner Center app** (Affiliate type) | partner.tiktokshop.com | `TIKTOK_SHOP_APP_KEY`, `TIKTOK_SHOP_APP_SECRET` | app review: days–weeks |
| 3 | Creator OAuth → access token + shop cipher | Seller Center / TikTok app authorization | `TIKTOK_SHOP_ACCESS_TOKEN`, `TIKTOK_SHOP_CIPHER` | with #2 |
| 4 | Verify affiliate endpoint paths in sandbox | `python3 scripts/probe_shop_api.py` with #2+#3 | fill `ENDPOINTS` in `tiktok/shop/client.py` | 1 session |
| 5 | **TikTok Developers app** (Login Kit + Content Posting API) | developers.tiktok.com | `TIKTOK_CLIENT_KEY`, `TIKTOK_CLIENT_SECRET` | audit ~3–4 weeks for public Direct Post |
| 6 | OAuth per creator account → refresh token | one-time browser flow per account | `tiktok_token_<account>.json` (gitignored) | minutes/account |
| 7 | **RTMP server + stream key per account** | TikTok LIVE Center → Go LIVE | `RTMP_KEY_<USERNAME>` (env only, never in code) | minutes/account |

Notes:
- Without #1, TTS falls back to Edge (no key, unofficial endpoint — last resort).
- Without #5's audit, the Posting API only publishes `SELF_ONLY` (private) to ≤5 test users.
- Token files and `.env` are gitignored; keys never enter the ledger or logs.

## Daily operation (hands-off)

1. Orchestrator runs agents on schedule (launchd keeps it alive).
2. Dashboard `:8080/stats` shows commission, spend, decisions.
3. Telegram alerts on: stream death, quota near-limit, kill events, money-path errors.
4. Human reviews `/decisions` weekly; tunes thresholds in `.env`.

## Kill switch

Dashboard button, or: `kill -USR1` — orchestrator stops agents and stream
within seconds. Ledger keeps the audit trail.

## Backup

Nightly: `sqlite3 data/ledger.db ".backup data/backup/ledger-$(date +%F).db"`
Retain 30 days. Config backup alongside.
