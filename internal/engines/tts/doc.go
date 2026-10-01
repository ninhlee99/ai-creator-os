// Package tts is the TTS capability of AI Creator OS: a 3-tier failover
// chain with a stable output contract — WAV bytes, PCM s16le, 24 kHz, mono —
// so the stream engine and avatar lip-sync always get one format.
//
// Default tier order (user-reorderable at runtime via ChainConfig/SetConfig):
//
//  1. Gemini TTS free tier ("gemini") — DEFAULT. The most expressive option
//     (style prompting); the user chose quality first.
//  2. VieNeu-TTS v3 Turbo local ("vieneu") — offline, no quota, native
//     Vietnamese with emotion cues [cười]/[thở dài]/[hắng giọng]. The
//     fallback when the free API runs out of quota mid-live.
//  3. Edge TTS ("edge") — no key, unofficial Microsoft endpoint (ToS-gray),
//     last resort only.
//
// The chain types (ProviderEntry, ChainConfig) are defined here and aliased
// by the parent engines package so both chains share one type.
//
// Honest notes about the VieNeu sidecar:
//
//   - The VieNeu server is THIRD-PARTY PYTHON code
//     (github.com/pnnbao97/VieNeu-TTS, Apache-2.0). Our codebase is 100% Go:
//     Go never imports Python — it manages the sidecar as an independent
//     subprocess, exactly like FFmpeg.
//   - The machine needs `uv` (recommended: it manages its own Python, never
//     touches the system Python) OR Docker (TTS_SIDECAR_MODE=docker). Start()
//     fails with a clear error when neither is present.
//   - The sidecar repo lives at <data>/third_party/vieneu-tts (fetched by the
//     local Registry: git clone, or zip download when git is missing). If it
//     is absent, Healthy() is false and Synthesize fails with a clear
//     "VieNeu chưa được tải" error so the chain fails over.
//   - The v3 Turbo model (~334 MB, pnnbao-ump/VieNeu-TTS-v3-Turbo) downloads
//     itself from HuggingFace on first start; EnsureModel(ctx, onProgress)
//     drives that explicitly for the dashboard "tải model" button, with
//     best-effort progress parsed from the sidecar's stdout.
//   - Resource use: VieNeu ONNX on CPU ≈ 300–500 MB RAM — far lighter than a
//     local LLM, comfortable next to a live stream.
//   - Weakness: VieNeu's emotional range does not match Gemini TTS on
//     difficult passages — that is exactly why Gemini is the default and
//     VieNeu the fallback, not the other way around.
package tts
