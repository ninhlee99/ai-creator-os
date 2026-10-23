// Package studio is the "AI studio" of AICOS: it creates affiliate videos
// and photo-list spots the way a human producer would — director (LLM)
// writes the shot plan, media generation shoots each shot/photo, FFmpeg
// assembles the final, trending music is attached.
//
// PIVOT 2026-10-02: the short-film pipeline (film.go, film_director.go,
// film_shots.go, cinematic.go, prompts/parked/ — build tag "parked") is
// parked. The live surface includes affiliate creation and the YouTube Story
// pipeline; fresh Story jobs are paused while free reference-image generation
// is unavailable.
//
// Everything is driven from the dashboard (page "Studio"); there is no CLI
// workflow. Long renders run as background jobs with progress in SQLite,
// so Ninh only turns the app ON and watches.
//
// Media generation goes through the MediaGen interface. GeminiMediaGen sends
// prompt-only image generation to Pollinations Free API Classic, uses gated
// Gemini text calls for image analysis, and blocks reference-image generation
// and Veo under the free-only policy. Fresh multiscene story jobs remain
// paused until a free provider can accept reference images.
package studio
