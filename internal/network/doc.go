// Package network manages the AI creator network: the TikTok account
// registry and lifecycle (account.go), persona assignment (persona.go),
// the onboarding pipeline (onboarding.go), the topic engine (topics.go),
// live slot scheduling (scheduler.go), and the network daemon (daemon.go).
//
// This is the Go port of apps/orchestrator/network/*.py. The Python sources
// are read-only reference; all behavior here mirrors them. Persistence goes
// through internal/ledger; the RTMP key itself is never stored, only the
// environment variable name that holds it, resolved at stream time.
//
// Engines (LLM and friends) are intentionally not imported: callers inject
// them through the local LLMClient interface, which engines.LLMChain is
// expected to satisfy.
package network
