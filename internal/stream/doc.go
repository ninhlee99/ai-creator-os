// Package stream is the in-process FFmpeg RTMP supervisor for AI Creator OS.
//
// It replaces the standalone stream-engine binary: instead of a separate
// process driven over HTTP (127.0.0.1:8090), the streamer agent calls
// Engine directly in the same binary.
//
// Resource-capped by design:
//   - one FFmpeg process at a time,
//   - watchdog restarts with backoff fails*5s, gives up after 5 failures,
//   - PushSegment queue is bounded (oldest segments are dropped).
//
// v1: test pattern (testsrc 1280x720 @30fps) + 440Hz tone via lavfi,
// libx264 veryfast zerolatency, aac, flv. PushSegment validates and RECORDS
// segments only — there is NO real overlay in v1. The bounded queue is the
// hook the v2 frame pipe will consume; do not mistake the counter for an
// overlay.
//
// Disclosure text: accepted by Start, currently bookkeeping/log only.
// Burning it into frames (drawtext) needs a fontfile and is deferred to v2.
//
// v2 roadmap: avatar scene via frame pipe on an fd — replace the lavfi
// inputs with a rawvideo pipe and feed rendered frames (avatar, captions,
// disclosure) straight from Go.
package stream
