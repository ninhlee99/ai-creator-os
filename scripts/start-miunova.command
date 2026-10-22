#!/bin/zsh
set -euo pipefail

repo_dir="${0:A:h}/.."
cd "$repo_dir"

if ! command -v go >/dev/null 2>&1; then
  print -u2 "Go chưa được cài hoặc chưa có trong PATH."
  read -r "?Nhấn Return để đóng cửa sổ..."
  exit 1
fi

exec go run ./cmd/aicos -addr 127.0.0.1:8080 -data ./data
