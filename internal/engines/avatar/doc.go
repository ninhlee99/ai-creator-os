// Package avatar renders audio-driven talking-head video: every frame's
// motion (lips, eyes, head, expression, gesture) is synthesized from the
// audio at that frame. Identity is locked per character so the same face
// appears in every frame of every clip.
//
// QUALITY CONTRACT (non-negotiable): the pipeline MUST NOT return static
// images with pan/zoom (Ken Burns) or crossfade transitions. Motion comes
// from the audio, per frame. Anything else is a bug.
//
// Architecture: provider chain (local free sidecar -> HeyGen -> D-ID),
// swappable like the TTS/LLM chains. Heavy models run as third-party
// sidecar subprocesses managed by the Go binary (same pattern as the
// VieNeu-TTS sidecar) — no Docker, no Python in this repo.
//
// Sidecar HTTP contract (v1) — the sidecar MUST implement:
//
//	GET  /health
//	  → 200 {"status":"ok","model":"<id>","device":"mps|cpu|cuda","version":"1"}
//
//	POST /render  (application/json)
//	  {
//	    "reference_image_b64": "<png|jpg base64>",
//
//	    "audio_wav_b64":       "<wav PCM s16le 24kHz mono base64>",
//	    "emotion_cues":        [{"name":"smile","intensity":0.8,"at_ms":1200,"hold_ms":800}],
//	    "seed": 12345,
//	    "identity_lock":       "<sha256 hex of image bytes + seed>",
//	    "width": 1080, "height": 1920, "fps": 25
//	  }
//	  → 200 {"video_mp4_b64":"<h264 mp4 base64>","duration_sec":12.5,
//	         "frames":312,"identity_lock":"<echo>"}
//	  HARD CONTRACT: every output frame is synthesized from (identity,
//	  audio at that timestamp). No slideshows, no Ken Burns.
//
//	POST /stream/open
//	  {"reference_image_b64","seed","identity_lock","width","height","fps"}
//	  → 200 {"session_id":"..."}
//	POST /stream/push  {"session_id":"...","pcm_b64":"<s16le mono 24kHz>"}
//	  → 200 {"accepted":true}
//	GET  /stream/frame?session_id=...
//	  → 200 image/jpeg (latest frame) | 204 no frame yet
//	POST /stream/close {"session_id":"..."} → 200
//
// Honest status: the local provider is fully specified here (client +
// lifecycle + contract) but the sidecar itself is third-party software
// that must be installed on the owner's Mac and integration-tested there.
// Until then LocalAvatarProvider.Healthy reports false and RenderClip
// fails with a clear "sidecar not running" error — never fake frames.
package avatar
