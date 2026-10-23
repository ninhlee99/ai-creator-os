# Viral source discovery and character replacement (`/viral`)

`/viral` separates YouTube source discovery, storyboard analysis, and full-character replacement. The Radar stores candidate metadata; it does not download or reuse footage. Production uses Google Flow `video_edit` when Flow and visual QC are ready, and requires per-job credit acknowledgement. That acknowledgement does not guarantee $0 usage.

## Current workflow

- Radar stores source candidates only; it does not download or reuse their footage.
- Storyboard analysis samples a supplied source video and returns a reference timeline; it does not create a video or replace the character.
- Before enqueue, production validates source file, minimum 3-second duration, 9:16 dimensions, character photos from the selected account, a ready Google Flow video-edit adapter, a ready visual QC analyzer, and per-job credit acknowledgement. The worker checks source dimensions again. Flow edits source segments up to 10 seconds, then assembles and checks output.
- Existing rendered jobs without current QC evidence remain marked unverified and cannot be published from the done gallery.

## Limits and status

- Flow availability and visual QC depend on local provider configuration. Private web APIs are not live-verified; jobs can fail after enqueue if provider behavior changes.
- No source video, reference image, or prompt can guarantee perfect choreography, identity, or channel growth. A visual QC pass is screening evidence, not evidence of future views or sales.
- InsightFace face swap is not accepted as full-character replacement because it changes only the face.

Primary implementation: `internal/automation/viral_radar.go`, `internal/studio/viral_character_edit.go`, `internal/studio/viral_jobs.go`, `internal/web/viral.go`, and `internal/web/templates/viral.html`.
