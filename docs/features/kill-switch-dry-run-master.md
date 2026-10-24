# Automation safety controls

## Scope

Safety gates apply to TikTok affiliate/viral radar, media generation, metrics sync and publishing actions. They do not claim that a provider call was canceled unless the worker reports cancellation.

## Controls

- Kill switch stops new automation actions and requests cancellation of cancellable in-flight jobs.
- Dry-run permits reads/planning but blocks production-side effects such as generating paid media, creating external links, uploading drafts and public publishing.
- Master switch controls scheduled automation ticks. Manual review and read-only inspection remain available.
- Every state change records actor, time, previous state and new state.

## Correct status

Show scheduler heartbeat, job state and provider result separately. A tick is not proof of a completed Radar sync, draft upload or order reconciliation. Provider timeout after submit stays `unknown/reconcile_required` until checked; do not retry blindly.

## Default behavior

External mutations stay disabled until operator config and provider permissions are valid. A missing gate fails closed for production actions. Draft upload and public publish remain distinct states.
