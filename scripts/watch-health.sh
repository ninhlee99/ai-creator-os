#!/bin/bash
# Watchdog ngoài cho MiuNova (Đợt S, tự hồi phục từ Đợt AK).
#
# Vì sao cần: KeepAlive của launchd chỉ restart khi process *exit*. Process
# bị deadlock/treo (tick kẹt) thì launchd tưởng vẫn khỏe. Script này gọi
# GET /healthz và coi là LỖI khi: không kết nối được, db_ok=false, hoặc
# needs_restart=true (có tác vụ nền kẹt).
#
# Script này không được cài hoặc chạy tự động. Nếu tự cấu hình watchdog,
# người dùng tự chọn scheduler và chính sách restart. Ví dụ cron:
#   */5 * * * * /path/to/ai-creator-os/scripts/watch-health.sh >> ~/aicos-data/logs/watchdog.log 2>&1
#
# Biến môi trường:
#   AICOS_HEALTH_URL         URL health (mặc định http://127.0.0.1:8080/healthz)
#   AICOS_WATCHDOG_RESTART   =0 → chỉ ghi log, không tự khởi động lại
#   AICOS_LAUNCHD_LABEL      nhãn launchd của app (mặc định com.ninhlee.aicos)
#   WATCHDOG_VERBOSE         =1 → ghi 1 dòng cả khi khỏe
set -u

URL="${AICOS_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
LABEL="${AICOS_LAUNCHD_LABEL:-com.ninhlee.aicos}"
STATE="${TMPDIR:-/tmp}/aicos-watchdog.fails"
stamp="$(date '+%Y-%m-%d %H:%M:%S')"

problem=""
resp="$(curl -sS --max-time 10 "$URL" 2>&1)"
if [ $? -ne 0 ]; then
  problem="không kết nối được $URL ($resp)"
elif ! printf '%s' "$resp" | grep -q '"db_ok":true'; then
  problem="app trả lời nhưng DB không khỏe: $resp"
elif printf '%s' "$resp" | grep -q '"needs_restart":true'; then
  problem="có tác vụ nền kẹt: $(printf '%s' "$resp" | grep -o '"stale_ticks":\[[^]]*\]')"
fi

if [ -z "$problem" ]; then
  rm -f "$STATE"
  if [ "${WATCHDOG_VERBOSE:-0}" = "1" ]; then
    echo "$stamp WATCHDOG: OK ($(printf '%s' "$resp" | grep -o '"uptime_sec":[0-9]*'))"
  fi
  exit 0
fi

fails=$(( $(cat "$STATE" 2>/dev/null || echo 0) + 1 ))
echo "$fails" > "$STATE"
echo "$stamp WATCHDOG: $problem (lần $fails liên tiếp)"

if [ "$fails" -ge 2 ] && [ "${AICOS_WATCHDOG_RESTART:-1}" != "0" ]; then
  if command -v launchctl >/dev/null 2>&1 &&
     launchctl print "gui/$(id -u)/$LABEL" >/dev/null 2>&1; then
    echo "$stamp WATCHDOG: tự khởi động lại app ($LABEL)"
    launchctl kickstart -k "gui/$(id -u)/$LABEL" && rm -f "$STATE"
  elif command -v systemctl >/dev/null 2>&1; then
    # Linux: unit user "aicos" (deploy/README.md). cron không có biến
    # môi trường của phiên đăng nhập → tự đặt XDG_RUNTIME_DIR.
    export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
    if systemctl --user is-enabled aicos >/dev/null 2>&1; then
      echo "$stamp WATCHDOG: tự khởi động lại app (systemd --user aicos)"
      systemctl --user restart aicos && rm -f "$STATE"
    fi
  fi
fi
exit 1
