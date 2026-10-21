// Package tts provides speech synthesis through local macOS, Gemini, and Edge providers.
// Both produce WAV bytes in PCM s16le, 24 kHz, mono format for video and
// narration pipelines.
//
// Default provider order is local macOS speech, Edge, then Gemini. The chain types
// (ProviderEntry, ChainConfig) are defined here and aliased by the parent
// engines package so both chains share one type.
package tts
