package avatar

import (
	"strings"
	"unicode/utf8"
)

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

// tagCues maps TTS emotion tags (lowercased, without brackets) to
// expression parameters. Tags not listed here are ignored by the avatar
// layer (TTS still interprets them for the voice).
var tagCues = map[string]EmotionCue{
	"cười":        {Name: "smile", Intensity: 0.8, HoldMs: 1500},
	"cười khẽ":    {Name: "smile", Intensity: 0.5, HoldMs: 1200},
	"cười lớn":    {Name: "laugh", Intensity: 1.0, HoldMs: 2000},
	"cười mỉm":    {Name: "smile", Intensity: 0.4, HoldMs: 1500},
	"thở dài":     {Name: "sigh", Intensity: 0.7, HoldMs: 1200},
	"hắng giọng":  {Name: "throat_clear", Intensity: 0.6, HoldMs: 800},
	"khóc":        {Name: "cry", Intensity: 0.9, HoldMs: 2500},
	"khóc nức nở": {Name: "cry", Intensity: 1.0, HoldMs: 3000},
	"ngạc nhiên":  {Name: "surprised", Intensity: 0.9, HoldMs: 1200},
	"buồn":        {Name: "sad", Intensity: 0.7, HoldMs: 2000},
	"tức giận":    {Name: "angry", Intensity: 0.8, HoldMs: 1500},
	"giận":        {Name: "angry", Intensity: 0.6, HoldMs: 1200},
	"thì thầm":    {Name: "whisper", Intensity: 0.6, HoldMs: 2000},
	"ngáp":        {Name: "yawn", Intensity: 0.7, HoldMs: 1500},
	"nháy mắt":    {Name: "wink", Intensity: 0.7, HoldMs: 600},
	"gật đầu":     {Name: "nod", Intensity: 0.6, HoldMs: 800},
	"lắc đầu":     {Name: "shake_head", Intensity: 0.6, HoldMs: 800},
	"nhún vai":    {Name: "shrug", Intensity: 0.6, HoldMs: 800},
}

// EmotionCueFromTag maps one tag (with or without brackets, any case) to
// its cue. It reports false for unknown tags.
func EmotionCueFromTag(tag string) (EmotionCue, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.TrimPrefix(tag, "[")
	tag = strings.TrimSuffix(tag, "]")
	tag = strings.TrimSpace(tag)
	cue, ok := tagCues[tag]
	return cue, ok
}

// ParseEmotionTags scans text for [tag] markers and returns the cues in
// order. CharOffset is the rune offset in the tag-stripped text; AtMs is
// -1 (unknown) — call Timed with the real audio duration to estimate.
// Unknown tags are skipped.
func ParseEmotionTags(text string) []TimedEmotionCue {
	var cues []TimedEmotionCue
	var clean strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '[' {
			if end := strings.IndexByte(text[i:], ']'); end > 0 {
				tag := text[i+1 : i+end]
				if cue, ok := EmotionCueFromTag(tag); ok {
					cues = append(cues, TimedEmotionCue{
						EmotionCue: cue,
						CharOffset: utf8.RuneCountInString(clean.String()),
						AtMs:       -1,
					})
				}
				i += end + 1
				continue
			}
		}
		clean.WriteByte(text[i])
		i++
	}
	return cues
}

// StripEmotionTags removes [tag] markers from text (the tags are for the
// voice/expression layers, not for display).
func StripEmotionTags(text string) string {
	var clean strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '[' {
			if end := strings.IndexByte(text[i:], ']'); end > 0 {
				i += end + 1
				continue
			}
		}
		clean.WriteByte(text[i])
		i++
	}
	return strings.TrimSpace(clean.String())
}

// Timed estimates each cue's AtMs by linear interpolation: a tag at rune
// offset o in a text of totalChars runes lands at o/totalChars of
// durationMs. This is a heuristic (speaking rate is not uniform), so the
// contract marks it as such — the renderer treats HoldMs as a window, not
// a sample-exact trigger.
func Timed(cues []TimedEmotionCue, totalChars int, durationMs int) []TimedEmotionCue {
	out := make([]TimedEmotionCue, len(cues))
	for i, c := range cues {
		out[i] = c
		if totalChars > 0 && durationMs > 0 {
			out[i].AtMs = c.CharOffset * durationMs / totalChars
		}
	}
	return out
}
