# Web AI accounts

## Purpose

Optional account pool for ChatGPT, Claude, Gemini, Grok, Muse AI, and Google Flow. Accounts and session cookies live in `data/webai.db`; the UI masks cookie values and job logs do not contain them.

Settings → System → Web AI restores accounts from a JSON backup. MiuNova does not open a login browser or read cookies from a user's browser profile. The account pool remains optional; provider jobs can use imported accounts.

Web AI account support is optional. If no account exists, jobs that require that account stop with a provider-specific error; other provider API chains keep working.

## Accounts and session checks

- JSON export includes Web AI accounts and scoped cookie jars; importing a backup restores those accounts. Legacy free-tier confirmation is not exported or required.
- Cookie jars preserve provider domain, path, expiry, secure/HTTP-only, SameSite, priority, and partition metadata through direct requests and headless fallbacks. Older backups with only a flat cookie header are migrated to the provider's default domain and `/` path because their original scope is unavailable.
- Import accepts supported provider IDs only; legacy `muse` / `muse_ai` and `google_flow` aliases normalize to canonical IDs. An account without usable credentials is disconnected and cannot enter the provider pool; Muse and Flow require a cookie, while other platforms accept a session key or cookie.
- Account probes run from `POST /web-ai/accounts/test`. Muse checks first-party auth and VM session endpoints without depending on Hatch WebSocket availability; Flow checks authenticated Flow history. HTTP 401 marks a session expired; HTTP 403 and network failures remain unknown because provider policy or Cloudflare can block server requests. Unknown probes do not delete cookies or disable an already connected account.
- The topbar count checks active local credentials plus a saved `connected` / `ok` status. Per-platform and Studio counts use active accounts with saved `connected` / `ok` status; they do not recheck credentials. None performs a live provider probe or proves generation permission or available credits.
- Changed cookies are persisted by account ID after provider responses, including Flow/Muse media downloads. Flow CSRF header refreshes with its cookie. Providers can still revoke or expire sessions.

## Muse and Flow video jobs

`POST /studio/generate` selects one active provider account and creates an `ai_content` job through `studio.Studio.CreateExternalJob`.

- **Muse AI** supports script, image, and video prompts. `internal/webai/muse_gateway.go` obtains Muse access/session/Hatch credentials, opens its Noise WebSocket, sends `chat.stream`, then reads `chat.history`. Replies with Muse file links are downloaded and signature-checked before saving to the Studio media directory. If the direct gateway fails before `chat.stream` is sent, `internal/webai/muse_headless.go` starts installed Chromium with `--headless=new` and submits the saved Muse session. After a prompt might have been accepted, the app does not retry through another transport, preventing duplicate jobs.
- **Google Flow** supports video. `internal/webai/flow_api.go` creates a Flow project, submits the web client's asynchronous generation request, correlates a new media item in authenticated project history, resolves its download URI, and saves a validated MP4. No AI Studio API key is used.
- Google Flow's current web entry point is [`flow.google.com`](https://flow.google.com/). The private API hosts used by the current adapter are implementation details and may change independently.
- Hidden generation fallback uses an isolated headless Chromium profile; it does not read the user's existing browser profile.
- `GET /studio/jobs?tab=running` shows active jobs. Only queued/running cards poll progress; done/failed jobs do not poll. Failed jobs can be deleted.

These are private web-client protocols, not stable public APIs. Flow access and account credits vary. Endpoints, model keys, and response shapes can change; no live provider generation is part of tests.

## Implementation and storage

- `internal/webai/web_session_validation.go`: checks imported account sessions and persists refreshed cookies.
- Web AI account import/export is handled by Settings JSON backup; there is no interactive browser-login endpoint.
- `internal/webai/store_accounts.go`: account storage, active account selection, status and cooldown.
- `internal/webai/muse_gateway.go`, `muse_headless.go`: Muse direct gateway and hidden generation fallback.
- `internal/webai/flow_api.go`: Flow web-client requests and MP4 download.
- `internal/web/studio_ai.go`: validates provider/kind/prompt, creates jobs, downloads script/media outputs.
- `internal/studio/external_job.go`: queues work, reports progress, persists status and output.

## Fail-closed behavior

- Expired, inactive, invalid, or disconnected accounts are skipped.
- Media downloads require trusted Muse/Google hosts and supported file signatures.
- Muse fallback runs only before prompt submission; ambiguous post-submit failures are not retried.
- Provider jobs fail with a reason when no account/session or downloadable media exists.

## Protocol references

- [Muse-Chat-MCP](https://github.com/duclm1x1/Muse-Chat-MCP) documents its browser-based Muse chat interaction.
- [muse-cli protocol notes](https://github.com/nikships/muse-cli/blob/main/docs/PROTOCOL.md) and [gateway implementation](https://github.com/nikships/muse-cli/blob/main/src/muse_cli/gateway.py) describe the internal Hatch/Noise transport used as a protocol reference.
- [Google Flow API sample](https://github.com/396001000/flow-Gemini-api/blob/main/flow_api.py) documents observed Flow web-client project, generation, history, and media calls.
