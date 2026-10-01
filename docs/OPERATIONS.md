# Operations

## Install (Mac M1 Pro)

```bash
# 1. deps
brew install ffmpeg go python@3.11 ollama
ollama pull qwen3:4b

# 2. repo
git clone <this-repo> /opt/tiktok-affiliate-os
cd /opt/tiktok-affiliate-os
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
