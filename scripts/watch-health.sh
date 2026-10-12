#!/bin/bash
# Watchdog ngoài cho AI Creator OS (Đợt S) — chạy định kỳ bằng cron hoặc
# launchd (StartInterval), vd: kiểm tra mỗi 5 phút.
#
# Vì sao cần: KeepAlive của launchd chỉ restart khi process *exit*. Process
# bị deadlock/treo (tick kẹt) thì launchd tưởng vẫn khỏe. Script này gọi
# GET /healthz: không kết nối được hoặc db_ok=false → ghi log + exit 1
# (cron/launchd ghi nhận), để người vận hành hoặc bước tiếp theo xử lý.
#
# Dùng với cron (crontab -e):
#   */5 * * * * /path/to/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
#
# Biến môi trường:
#   AICOS_HEALTH_URL  URL health (mặc định http://127.0.0.1:8080/healthz)
set -u

URL="${AICOS_HEALTH_URL:-http://127.0.0.1:8080/healthz}"

resp="$(curl -sS --max-time 10 "$URL" 2>&1)"
rc=$?
stamp="$(date '+%Y-%m-%d %H:%M:%S')"

if [ $rc -ne 0 ]; then
  echo "$stamp WATCHDOG: không kết nối được $URL ($resp)"
  exit 1
fi

# db_ok=false → app sống nhưng DB có vấn đề.
if ! printf '%s' "$resp" | grep -q '"db_ok":true'; then
  echo "$stamp WATCHDOG: app trả lời nhưng DB không khỏe: $resp"
  exit 1
fi

# Ghi 1 dòng gọn mỗi lần kiểm tra OK? Không — chỉ ghi khi có vấn đề,
# tránh spam log trên máy 24/7. Muốn verbose: WATCHDOG_VERBOSE=1.
if [ "${WATCHDOG_VERBOSE:-0}" = "1" ]; then
  uptime="$(printf '%s' "$resp" | grep -o '"uptime_sec":[0-9]*')"
  echo "$stamp WATCHDOG: OK ($uptime)"
fi
exit 0
