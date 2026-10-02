// Package studio is the "AI studio" of AICOS: it creates affiliate videos
// and photo-list spots the way a human producer would — director (LLM)
// writes the shot plan, media generation shoots each shot/photo, FFmpeg
// assembles the final, trending music is attached.
//
// PIVOT 2026-10-02: the short-film pipeline (film.go, film_director.go,
// film_shots.go, cinematic.go, prompts/parked/ — build tag "parked") is
// parked. This package's live surface is affiliate video creation.
//
// Everything is driven from the dashboard (page "Studio"); there is no CLI
// workflow. Long renders run as background jobs with progress in SQLite,
// so Ninh only turns the app ON and watches.
//
// Media generation goes through the MediaGen interface. The production
// implementation is GeminiMediaGen: image generation over the Gemini API,
// reusing the multi-key rotation the app already uses for LLM/TTS
// (GEMINI_API_KEYS).
package studio
