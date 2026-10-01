// Package web is the Go port of the Python FastAPI dashboard
// (apps/api/main.py + apps/api/templates/*).
//
// The whole system is administered through the UI: accounts, onboarding,
// topic planning, live schedule, video production, multi-platform
// publishers, affiliate shop, analytics, settings and the kill switch.
//
// Only the standard library plus modernc.org/sqlite (for a small read
// handle used for decision/analytics queries the ledger API does not
// expose) are used. The engines (LLM/TTS chains), the VieNeu sidecar and
// per-provider health checkers are behind small local interfaces; the
// wiring worker injects the real implementations later. Until then the
// dashboard degrades gracefully (nil interfaces render "not connected").
package web
