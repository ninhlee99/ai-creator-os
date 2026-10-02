package avatar

import ()

// EmotionCue is one expression parameter sent to the avatar renderer.
// It is derived from the TTS emotion tags (e.g. [cười], [thở dài]) so the
// face matches the voice: the tags already flow through TTS untouched, and
// this layer parses them separately for expression control.
type EmotionCue struct {
	Name      string  `json:"name"`      // "smile", "laugh", "sigh", ...
	Intensity float64 `json:"intensity"` // 0..1
	HoldMs    int     `json:"hold_ms"`   // how long to hold the expression
}

// TimedEmotionCue places a cue at a point in the audio.
type TimedEmotionCue struct {
	EmotionCue
	// CharOffset is the rune offset of the tag in the tag-stripped text.
	CharOffset int `json:"-"`
	// AtMs is milliseconds into the audio. -1 = unknown (call Timed to
	// estimate from audio duration).
	AtMs int `json:"at_ms"`
}
