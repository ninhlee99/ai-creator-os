// Package studio creates TikTok affiliate and viral videos — director (LLM)
// writes the shot plan, media generation shoots each shot/photo, and FFmpeg
// assembles the final with optional operator-provided audio.
//
// Everything is driven from the dashboard (page "Studio"); there is no CLI
// workflow. Long renders run as background jobs with progress in SQLite,
// so Ninh only turns the app ON and watches.
//
// Media generation goes through the MediaGen interface.
package studio
