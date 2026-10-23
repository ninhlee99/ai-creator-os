# Studio AI (`/studio`)

## Purpose

Studio manages video production jobs, assets, quality checks, and media. It supports affiliate jobs from operator-provided clips and provider-backed script/image/video generation through Muse Chat and video generation through Google Flow.

## Routes

- `GET /studio` — affiliate production, library, and trends.
- `GET /studio/create` — Muse/Flow creation form.
- `POST /studio/generate` — validate request, select connected account, enqueue provider job.
- `POST /studio/generate-image` — use Pollinations Free API Classic for prompts without references; otherwise render a neutral text card from supplied product name/category. Gemini image calls are blocked under U-04, and reference-based generation is unavailable. Fallback adds no rating, sales, price, discount, benefit, or unrelated local photo.
- `GET /studio/jobs`, `GET /studio/jobs/{id}` — job list and detail.
- `POST /studio/jobs/{id}/delete` — remove failed jobs.
- `GET /studio/assets/{job}/{file}` — job assets.
- `GET /studio/trends` and `POST /studio/trends/refresh` — trending sounds.

## Provider-backed creation

`internal/web/studio_ai.go` calls `studio.Studio.CreateExternalJob` with `kind=ai_content`.

- Muse accepts script, image, and video prompts. It uses the selected account's saved web session, prefers its direct Hatch gateway, and falls back to hidden Chrome/Edge only when the direct gateway fails before sending the prompt. Script output is saved as Markdown. Media output is downloaded to Studio and checked against its file signature.
- Flow accepts video prompts only. The Flow web API creates a project, submits one generation operation, polls project history, resolves the media URL, and downloads an MP4 into Studio.
- `internal/studio/external_job.go` shares Studio's concurrency slots and persists queued/running/done/failed progress in the Studio database.
- Generation uses the selected provider account's quota/credits. The UI must not describe account credit or access as guaranteed free.
- A “credits acknowledged” checkbox confirms possible usage; it does not prove the provider will charge $0. Under MiuNova's free-only target, do not enable a Flow path until zero-cost access and quota are verified.

The provider APIs are private web-client interfaces and are not live-tested by the test suite. Provider changes can fail jobs until the adapter is updated.

## Affiliate flow

`POST /studio/affiliate/plan` creates a product-specific shot list. The operator supplies one moving clip per shot through `POST /studio/affiliate`; Studio normalizes, assembles, adds selected audio, and runs visual/motion checks. QC samples clips and does not prove product use or guarantee outcomes.

## Job progress

`internal/web/studio_poll_test.go` specifies the frontend polling contract: only cards marked `.studio-live-job[data-job]` receive progress requests. Completed and failed tabs do not poll. A failed job is static until the operator deletes it or opens its details.
