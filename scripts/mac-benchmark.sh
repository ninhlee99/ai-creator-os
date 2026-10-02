#!/usr/bin/env bash
# Benchmark các model local trên Mac (M1 Pro) cho AI Creator OS.
# Chạy trên máy Mac của Ninh:  bash scripts/mac-benchmark.sh
# Kết quả dùng để hiệu chỉnh các ước tính fps/tok-s trong app.
set -u
MODEL="${MODEL:-$HOME/aicos-data/models/qwen2.5-7b-instruct-q4_k_m.gguf}"
LLAMA_PORT="${LLAMA_PORT:-8081}"
VIENEU_URL="${VIENEU_URL:-http://127.0.0.1:8000}"

say() { printf '\n==> %s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }

say "1/4 — Kiểm tra công cụ"
for t in brew ffmpeg uv llama-server python3; do
  if have "$t"; then echo "  [OK] $t"; else echo "  [THIẾU] $t"; fi
done
if ! have brew; then echo "Cài Homebrew trước: https://brew.sh"; exit 1; fi
if ! have llama-server; then echo "Chạy: brew install llama.cpp"; fi
if ! have uv; then echo "Chạy: brew install uv"; fi
if ! have ffmpeg; then echo "Chạy: brew install ffmpeg"; fi

say "2/4 — Benchmark LLM (llama-server, tok/s)"
if [ ! -f "$MODEL" ]; then
  echo "  [BỎ QUA] chưa có model: $MODEL"
  echo "  Tải qwen2.5-7b-instruct-q4_k_m.gguf vào thư mục models rồi chạy lại."
else
  if ! curl -sf "http://127.0.0.1:$LLAMA_PORT/health" >/dev/null 2>&1; then
    echo "  Khởi động llama-server (nền)..."
    nohup llama-server -m "$MODEL" --port "$LLAMA_PORT" -c 4096 -ngl 99 \
      >/tmp/llama-bench.log 2>&1 &
    for _ in $(seq 1 60); do
      curl -sf "http://127.0.0.1:$LLAMA_PORT/health" >/dev/null 2>&1 && break
      sleep 2
    done
  fi
  echo "  Sinh 200 token, đo tốc độ..."
  curl -s "http://127.0.0.1:$LLAMA_PORT/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"Viết một đoạn mở đầu phim ngắn khoảng 150 từ bằng tiếng Việt."}],"max_tokens":200,"temperature":0.7}' \
    -o /tmp/llm-bench.json -w '  HTTP %{http_code}, tổng %{time_total}s\n'
  python3 - <<'EOF'
import json,sys
try:
    d=json.load(open('/tmp/llm-bench.json'))
    u=d.get('usage',{})
    pt,ct=u.get('prompt_tokens',0),u.get('completion_tokens',0)
    tt=d.get('timings',{})
    pred=tt.get('predicted_n',ct) or ct
    ms=(tt.get('predicted_ms',0) or 1)
    print(f"  prompt={pt} token, sinh={pred} token, tốc độ={pred/(ms/1000):.1f} tok/s")
except Exception as e:
    print("  Không đọc được kết quả:",e); sys.exit(0)
EOF
fi

say "3/4 — Benchmark TTS VieNeu (độ trễ / RTF)"
if curl -sf "$VIENEU_URL/health" >/dev/null 2>&1; then
  echo "  Sidecar đang chạy. Tổng hợp câu mẫu..."
  START=$(date +%s.%N)
  curl -s -X POST "$VIENEU_URL/v1/audio/speech" \
    -H 'Content-Type: application/json' \
    -d '{"input":"Xin chào, đây là bài kiểm tra giọng đọc tiếng Việt của AI Creator OS.","response_format":"wav"}' \
    -o /tmp/tts-bench.wav -w '  HTTP %{http_code}, %{time_total}s\n'
  END=$(date +%s.%N)
  DUR=$(python3 -c "print(f'{$END-$START:.2f}')")
  if command -v ffprobe >/dev/null 2>&1; then
    AUDIO_DUR=$(ffprobe -v error -show_entries format=duration -of csv=p=0 /tmp/tts-bench.wav 2>/dev/null || echo 0)
    RTF=$(python3 -c "print(f'{$DUR/max(float($AUDIO_DUR),0.01):.2f}')")
    echo "  Tổng hợp mất ${DUR}s cho ${AUDIO_DUR}s audio → RTF=${RTF} (càng nhỏ càng nhanh; <1 là realtime)"
  else
    echo "  Tổng hợp mất ${DUR}s (thiếu ffprobe nên không tính được RTF)"
  fi
else
  echo "  [BỎ QUA] sidecar VieNeu chưa chạy ở $VIENEU_URL"
  echo "  Mở app aicos → Cài đặt → Model local → bật VieNeu rồi chạy lại bước này."
fi

say "4/4 — HyperFrames"
echo "  HyperFrames là hướng tăng fps avatar đã nghiên cứu (docs/RESEARCH.md)."
echo "  Nếu đã cài theo hướng dẫn nghiên cứu, chạy benchmark của nó và ghi lại fps."
echo "  Chưa cài → bỏ qua, không chặn các bước trên."

say "Xong. Gửi 3 con số (tok/s, RTF, HyperFrames fps nếu có) cho Milo để cập nhật vào app."
