// Package local manages on-machine model assets and the sidecar processes
// that serve them.
//
//   - Registry: model files live under <data>/models (e.g. the llama.cpp GGUF)
//     and third-party repos under <data>/third_party (e.g. vieneu-tts). It
//     downloads from HuggingFace/GitHub with resume support and progress
//     callbacks. Models are NEVER bundled into the Go binary.
//   - Process: a supervised long-running child process (llama-server, the
//     VieNeu TTS sidecar, whisper.cpp later): Start/Stop/Restart, automatic
//     restart on crash with increasing backoff (up to MaxRestarts, default 5,
//     then it reports error state), and a health check every 30s.
package local
