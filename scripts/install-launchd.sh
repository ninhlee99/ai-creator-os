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
# nhập và tự chạy lại khi crash (KeepAlive). Đợt AK: cài kèm watchdog
# sức khỏe (scripts/watch-health.sh mỗi 5 phút) — app treo mà chưa crash
# cũng tự được khởi động lại.
#
# Gỡ: ./scripts/install-launchd.sh --uninstall
set -euo pipefail

LABEL="com.ninhlee.aicos"
DEST="$HOME/Library/LaunchAgents/$LABEL.plist"
WD_LABEL="$LABEL.watchdog"
WD_DEST="$HOME/Library/LaunchAgents/$WD_LABEL.plist"
WD_SCRIPT="$(cd "$(dirname "$0")" && pwd)/watch-health.sh"
WD_SCRIPT_DEST="$HOME/Library/LaunchAgents/$WD_LABEL.sh"
TEMPLATE="$(cd "$(dirname "$0")/../deploy" && pwd)/com.ninhlee.aicos.plist"

if [[ "${1:-}" == "--uninstall" ]]; then
  launchctl bootout "gui/$(id -u)" "$WD_DEST" 2>/dev/null || true
  launchctl bootout "gui/$(id -u)" "$DEST" 2>/dev/null || true
  rm -f "$DEST" "$WD_DEST" "$WD_SCRIPT_DEST"
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
# được shell profile của Ninh). Không bắt buộc: bỏ trống cũng được, nhập
# key sau ở Cài đặt · Nhà cung cấp trong dashboard.
KEYS="${GEMINI_API_KEYS:-}"

sed -e "s#__AICOS_BIN__#$BIN#g" \
    -e "s#__AICOS_DATA__#$DATA#g" \
    -e "s#__GEMINI_KEYS__#$KEYS#g" \
    "$TEMPLATE" > "$DEST"
# Đợt T: plist 600 (thay vì 644) — file chứa key plaintext, chỉ chủ máy đọc được.
chmod 600 "$DEST"

# Nạp lại (idempotent): gỡ bản cũ nếu có rồi nạp bản mới.
launchctl bootout "gui/$(id -u)" "$DEST" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$DEST"

# Đợt AK: watchdog sức khỏe — app treo (không crash) vẫn được tự chạy lại.
# launchd may lack Files & Folders permission for scripts kept under Documents.
# Stage the helper beside its LaunchAgent plist so /bin/bash can read it.
cp "$WD_SCRIPT" "$WD_SCRIPT_DEST"
chmod 700 "$WD_SCRIPT_DEST"
cat > "$WD_DEST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$WD_LABEL</string>
  <key>ProgramArguments</key>
  <array><string>/bin/bash</string><string>$WD_SCRIPT_DEST</string></array>
  <key>StartInterval</key><integer>300</integer>
  <key>StandardOutPath</key><string>$DATA/logs/watchdog.log</string>
  <key>StandardErrorPath</key><string>$DATA/logs/watchdog.log</string>
</dict>
</plist>
PLIST
chmod 644 "$WD_DEST"
launchctl bootout "gui/$(id -u)" "$WD_DEST" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$WD_DEST"

echo "Đã cài watchdog: $DEST"
echo "Đã cài kiểm tra sức khỏe mỗi 5 phút: $WD_DEST (log: $DATA/logs/watchdog.log)"
echo "Kiểm tra: launchctl print gui/$(id -u)/$LABEL | grep -i state"
echo "Log: $DATA/logs/aicos.log"
if [[ -z "$KEYS" ]]; then
  echo "CHÚ Ý: GEMINI_API_KEYS đang trống — không sao, nhập key ở Cài đặt ·"
  echo "Nhà cung cấp trong dashboard là app tự dùng ngay (không cần cài lại)."
fi
