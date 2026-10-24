#!/usr/bin/env bash
# check-reachable.sh — CI guard: không package internal mới nào được "không ai
# dùng" một cách lặng lẽ.
#
# Luật:
# Mọi package internal/... phải được binary sử dụng. Package không còn dùng
# cần bị xóa, không giữ lại danh sách tính năng đã loại bỏ.
#
# Cách dùng (R2-W7 gọi từ CI):
#   ./scripts/check-reachable.sh
set -euo pipefail
cd "$(dirname "$0")/.."

MODULE="github.com/ninhlee99/ai-creator-os"

deps=$(go list -deps ./cmd/aicos)
fail=0

# Không package internal nào được unreachable.
while IFS= read -r pkg; do
  [ -n "$pkg" ] || continue
  rel=${pkg#"$MODULE"/}
  case "$rel" in internal/*) ;; *) continue ;; esac
  if ! grep -qx "$pkg" <<< "$deps"; then
    echo "UNREACHABLE: $pkg không vào binary" >&2
    fail=1
  fi
done < <(go list -e ./internal/... 2>/dev/null)

if [ "$fail" -eq 0 ]; then
  echo "check-reachable: OK — mọi package internal đều vào binary"
fi
exit "$fail"
