// Package studio is the "AI studio" of AICOS: it creates affiliate videos,
// short films and photo-list spots the way a human producer would —
// director (LLM) writes the shot plan, media generation shoots each
// shot/photo, FFmpeg assembles the final, trending music is attached.
//
// Everything is driven from the dashboard (page "Studio"); there is no CLI
// workflow. Long renders run as background jobs with progress in SQLite,
// so Ninh only turns the app ON and watches.
//
// Media generation goes through the MediaGen interface. The production
// implementation is GeminiMediaGen: image generation and Veo video
// generation over the Gemini API, reusing the multi-key rotation the app
// already uses for LLM/TTS (GEMINI_API_KEYS). Veo requires a
// billing-enabled Google Cloud project on the key; when it is unavailable
// the studio falls back to the photo-list format (still photos + trending
// music), which needs only image generation.
package studio
