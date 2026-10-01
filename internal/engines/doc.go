// Package engines wires every AI capability of AI Creator OS as a
// 3-tier failover chain. Every future caller (network/agents/web) MUST go
// through a Chain — never call a single provider directly.
//
// Tier layout per capability (default order; user-reorderable at runtime via
// ChainConfig/SetConfig, no restart needed):
//
//	LLM  (internal/engines/llm.go)
//	  1. Gemini free tier (GeminiProvider "gemini") — realtime, no local
//	     compute. Default because quality and latency beat local.
//	  2. llama-server local (LlamaServerProvider "llama-server") — llama.cpp,
//	     OpenAI-compatible HTTP on localhost:8081, GGUF Qwen2.5-7B-Instruct
//	     Q4_K_M. Automatic failover when the API errors, times out,
//	     rate-limits or runs out of quota.
//	  3. Paid API ("paid") — stub only (Enabled=false); returns
//	     "paid tier disabled". Placeholder, no real calls implemented.
//
//	TTS  (internal/engines/tts)
//	  1. Gemini TTS free tier ("gemini") — DEFAULT. Most expressive; user chose
//	     quality first, local as the offline fallback when quota runs out.
//	  2. VieNeu-TTS v3 Turbo local ("vieneu") — offline, no quota, native
//	     Vietnamese, emotion cues [cười]/[thở dài]/[hắng giọng]. Runs as a
//	     supervised sidecar subprocess (third-party Python, like FFmpeg — our
//	     codebase stays 100% Go, no Python imports).
//	  3. Edge TTS ("edge") — no key, unofficial endpoint (ToS-gray), last
//	     resort only.
//
// Runtime configuration: each chain holds a ChainConfig{Order []ProviderEntry}
// protected by RWMutex. SetConfig applies immediately — the web settings page
// calls it after every save; DefaultChains optionally loads the saved config
// at startup via ConfigSource. Disabled providers are skipped; each attempt
// gets its own context timeout; Retries controls per-provider retry before
// failover. A provider without its API key fails that attempt fast with a
// clear "missing API key" error and the chain still fails over.
//
// Tier switches are reported through onSwitch(from, to, reason), which
// DefaultChains wires to the ledger via DecisionLogger — e.g. when Gemini
// runs out of quota mid-live and the chain jumps to VieNeu, the decision is
// logged with from=gemini, to=vieneu, reason=quota/rate-limit, so the user
// always knows which tier is serving.
//
// Local model management lives in internal/engines/local: a Registry for model
// files (<data>/models for the llama GGUF, <data>/third_party/vieneu-tts for the
// sidecar repo) and a Process supervisor (start/stop, crash restart with
// backoff, 30s health checks) shared by llama-server and the VieNeu sidecar.
//
// Resource estimates (Mac M1 Pro 32GB, live stream running alongside):
//
//	llama-server  Qwen2.5-7B Q4_K_M  ~6 GB RAM  (4.7 GB download)
//	VieNeu ONNX CPU           ~300-500 MB RAM  (334 MB model, auto-downloaded)
//	whisper.cpp base (future) ~500 MB RAM
//
// The total fits comfortably next to a live stream on 32 GB.
//
// Honest weaknesses (read before relying on this in production):
//
//   - Gemini free tier has unpublished per-minute quotas — the exact reason the
//     local tiers exist. Expect mid-live failovers; watch the ledger.
//   - VieNeu v3 Turbo's emotional range does not match Gemini TTS on difficult
//     passages — that is why Gemini is the TTS default and VieNeu the fallback.
//   - The VieNeu sidecar is third-party Python code. Go only supervises it as
//     an independent process (like FFmpeg); the machine needs `uv` (which
//     manages its own Python, untouched system Python) OR Docker.
//   - Edge TTS is an unofficial endpoint: unpublished rate limits, can be cut
//     off without notice. Last resort only.
//   - Local LLM quality/latency is a fallback, not the default: first-token
//     latency on CPU is far worse than the free API.
//   - llama.cpp needs a Metal-enabled build for GPU acceleration on Apple
//     Silicon; a CPU-only build still works, just slower.
package engines
