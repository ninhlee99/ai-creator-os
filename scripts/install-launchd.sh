#!/bin/bash
# Cài watchdog cho AI Creator OS trên Mac (Đợt O1) — chạy 1 lần duy nhất.
#
#   ./scripts/install-launchd.sh /đường/dẫn/aicos /đường/dẫn/data
#
# Ví dụ:
#   ./scripts/install-launchd.sh ~/ai-creator-os/aicos-darwin-arm64 ~/aicos-data
#
# Việc của script: sinh plist đúng đường dẫn máy Ninh → chép vào
# ~/Library/LaunchAgents → nạp vào launchd. Từ đó app tự chạy khi đăng
# nhập và tự chạy lại khi crash (KeepAlive).
#
# Gỡ: ./scripts/install-launchd.sh --uninstall
set -euo pipefail

LABEL="com.ninhlee.aicos"
DEST="$HOME/Library/LaunchAgents/$LABEL.plist"
TEMPLATE="$(cd "$(dirname "$0")/../deploy" && pwd)/com.ninhlee.aicos.plist"

if [[ "${1:-}" == "--uninstall" ]]; then
  launchctl bootout "gui/$(id -u)" "$DEST" 2>/dev/null || true
  rm -f "$DEST"
  echo "Đã gỡ watchdog $LABEL."
  exit 0
fi

if [[ $# -lt 2 ]]; then
  echo "Dùng: $0 /đường/dẫn/aicos /đường/dẫn/data [--uninstall]" >&2
  exit 1
fi

BIN="$1"; DATA="$2"
[[ -x "$BIN" ]] || { echo "Không thấy binary thực thi: $BIN" >&2; exit 1; }
mkdir -p "$DATA/logs" "$HOME/Library/LaunchAgents"

# GEMINI_API_KEYS: lấy từ môi trường hiện tại nếu có (launchd không đọc
# được shell profile của Ninh, nên phải nhúng thẳng vào plist).
KEYS="${GEMINI_API_KEYS:-}"

sed -e "s#__AICOS_BIN__#$BIN#g" \
    -e "s#__AICOS_DATA__#$DATA#g" \
    -e "s#__GEMINI_KEYS__#$KEYS#g" \
    "$TEMPLATE" > "$DEST"
chmod 644 "$DEST"

# Nạp lại (idempotent): gỡ bản cũ nếu có rồi nạp bản mới.
launchctl bootout "gui/$(id -u)" "$DEST" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$DEST"

echo "Đã cài watchdog: $DEST"
echo "Kiểm tra: launchctl print gui/$(id -u)/$LABEL | grep -i state"
echo "Log: $DATA/logs/aicos.log"
if [[ -z "$KEYS" ]]; then
  echo "CHÚ Ý: GEMINI_API_KEYS đang trống — app vẫn chạy nhưng tier Gemini sẽ fail-over."
  echo "Đặt biến môi trường rồi chạy lại script để nhúng key vào plist."
fi
