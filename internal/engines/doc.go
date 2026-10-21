// Package engines wires AI capabilities through provider chains. Callers
// should use these chains instead of calling providers directly.
//
// Default providers:
//   - LLM: Gemini API.
//   - TTS: Gemini API followed by Edge.
//
// ChainConfig supports runtime ordering and enablement. Settings changes
// apply immediately. Failover decisions are recorded through DecisionLogger.
package engines
