# Web AI accounts

## Purpose

Connect web sessions for ChatGPT, Claude, Gemini, Grok, Muse Chat, and Google Flow. The account store is `data/webai.db`; session cookies stay masked in the UI and are not written to job logs.

## Connect an account

- Open `GET /settings/system#tab-webai` and start browser login for the provider.
- `POST /web-ai/browser-login/start` opens an isolated Chrome/Edge profile and captures the provider session through CDP.
- Flow can also copy the saved Google session with `POST /web-ai/accounts/sync-google`.
- Account probes run from `POST /web-ai/accounts/test`. Muse checks its first-party session and Hatch history; Flow checks authenticated Flow history. Failed network checks stay unknown unless provider returns an explicit expired-session response.

## Muse and Flow video jobs

`POST /studio/generate` selects one active provider account and creates an `ai_content` job through `studio.Studio.CreateExternalJob`.

- **Muse Chat** supports script, image, and video prompts. `internal/webai/muse_gateway.go` obtains Muse access/session/Hatch credentials, opens its Noise WebSocket, sends `chat.stream`, then reads `chat.history`. Replies with Muse file links are downloaded and signature-checked before saving to the Studio media directory. If the direct gateway fails before `chat.stream` is sent, `internal/webai/muse_headless.go` starts installed Chrome/Edge with `--headless=new` and submits the prompt through the signed-in Muse page. It never opens a visible generation window. After a prompt might have been accepted, the app does not retry through another transport, preventing duplicate jobs.
- **Google Flow** supports video. `internal/webai/flow_api.go` creates a Flow project, submits the web client's asynchronous generation request, correlates a new media item in authenticated project history, resolves its download URI, and saves a validated MP4. No AI Studio API key is used.
- `GET /studio/jobs?tab=running` shows active jobs. Only queued/running cards poll progress; done/failed jobs do not poll. Failed jobs can be deleted.

These are private web-client protocols, not stable public APIs. Flow access and account credits vary. Endpoints, model keys, and response shapes can change; no live provider generation is part of tests.

## Implementation and storage

- `internal/webai/browser_login.go`: provider login, cookie capture, CDP session isolation.
- `internal/webai/store.go`: `accounts` table, active account selection, status and cooldown.
- `internal/webai/muse_gateway.go`, `muse_headless.go`: Muse direct gateway and hidden browser fallback.
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
