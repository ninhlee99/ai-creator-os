#!/usr/bin/env bash
# check-reachable.sh — CI guard: không package internal mới nào được "không ai
# dùng" một cách lặng lẽ.
#
# Luật:
#  1. Mọi package internal/... phải vào được binary (qua `go list -deps
#     ./cmd/aicos`), TRỪ các package trong PARKED_ALLOWLIST (park có chủ đích
#     sau tag `//go:build parked`, test bằng `go test -tags parked ./...`).
#  2. Ngược lại, package trong PARKED_ALLOWLIST KHÔNG được lọt vào binary
#     ở chế độ build thường (tag đang hoạt động đúng).
#
# Cách dùng (R2-W7 gọi từ CI):
#   ./scripts/check-reachable.sh
set -euo pipefail
cd "$(dirname "$0")/.."

MODULE="github.com/ninhlee99/ai-creator-os"

# Parked có chủ đích — xem docs/ARCHITECTURE.md §12 (mục 3), docs/REVIEW_ROUND2.md R2-W6.
PARKED_ALLOWLIST="internal/affiliatehunter internal/agents/analyst internal/agents/content internal/agents/governance internal/agents/hunter internal/agents/streamer internal/stream"

deps=$(go list -deps ./cmd/aicos)
fail=0

in_parked() { case " $PARKED_ALLOWLIST " in *" $1 "*) return 0;; esac; return 1; }

# 1. Không package mới nào unreachable mà không khai báo parked.
while IFS= read -r pkg; do
  [ -n "$pkg" ] || continue
  rel=${pkg#"$MODULE"/}
  case "$rel" in internal/*) ;; *) continue ;; esac
  if in_parked "$rel"; then continue; fi
  if ! grep -qx "$pkg" <<< "$deps"; then
    echo "UNREACHABLE: $pkg không vào binary và không khai báo parked" >&2
    fail=1
  fi
done < <(go list -e ./internal/... 2>/dev/null)

# 2. Package parked không được lọt vào binary ở build thường.
for rel in $PARKED_ALLOWLIST; do
  if grep -qx "$MODULE/$rel" <<< "$deps"; then
    echo "LEAKED: $rel lọt vào binary ở build thường (tag parked hỏng?)" >&2
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "check-reachable: OK — mọi package internal đều vào binary hoặc khai báo parked"
fi
exit "$fail"
