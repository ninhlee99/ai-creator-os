#!/bin/bash
# Legacy launchd cleanup. MiuNova no longer installs startup services.
set -euo pipefail

LABEL="com.ninhlee.aicos"
DEST="$HOME/Library/LaunchAgents/$LABEL.plist"
WD_LABEL="$LABEL.watchdog"
WD_DEST="$HOME/Library/LaunchAgents/$WD_LABEL.plist"
WD_SCRIPT_DEST="$HOME/Library/LaunchAgents/$WD_LABEL.sh"

if [[ "${1:-}" == "--uninstall" ]]; then
  launchctl bootout "gui/$(id -u)" "$WD_DEST" 2>/dev/null || true
  launchctl bootout "gui/$(id -u)" "$DEST" 2>/dev/null || true
  rm -f "$DEST" "$WD_DEST" "$WD_SCRIPT_DEST"
  echo "Đã gỡ dịch vụ tự khởi động $LABEL và watchdog."
  exit 0
fi

echo "Tự khởi động đã bị tắt. Mở app thủ công; nếu cần, tự cấu hình trong macOS." >&2
echo "Gỡ cấu hình cũ bằng: $0 --uninstall" >&2
exit 2
