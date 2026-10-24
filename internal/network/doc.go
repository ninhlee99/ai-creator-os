// Package network manages TikTok accounts, channel identity, onboarding,
// topic research, and account lifecycle.
//
// Persistence goes through internal/ledger; account-specific settings are
// resolved by the application at runtime.
//
// Engines (LLM and friends) are intentionally not imported: callers inject
// them through the local LLMClient interface, which engines.LLMChain is
// expected to satisfy.
package network
