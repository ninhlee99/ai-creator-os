# Viral source discovery and character replacement (`/viral`)

`/viral` keeps discovery separate from production. YouTube Viral Radar can discover candidate sources. Character replacement production is paused under the project's free-only rule: the only implemented editor, Gemini Omni video edit, requires billing. No video-edit API request is sent, and enqueue requests fail before creating a job.

## Current workflow

- Radar stores source candidates only; it does not download or reuse their footage.
- An operator can use a separately created video through Studio's manual import path. Imported media is not evidence of character replacement and does not pass viral character QC automatically.
- Existing rendered jobs without current QC evidence remain marked unverified and cannot be published from the done gallery.

## Limits and status

- Gemini Omni video editing is paid-only and disabled by the free-only policy.
- No source video, reference image, or prompt can guarantee perfect choreography, identity, or channel growth. A visual QC pass is screening evidence, not evidence of future views or sales.
- InsightFace face swap is not accepted as full-character replacement because it changes only the face.

Primary implementation: `internal/automation/viral_radar.go`, `internal/studio/gemini_video_edit.go`, `internal/studio/viral.go`, `internal/web/viral.go`, and `internal/web/templates/viral.html`.
