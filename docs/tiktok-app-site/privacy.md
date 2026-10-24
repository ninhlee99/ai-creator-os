# Privacy Policy — MiuNova TikTok Connector

Last updated: 2026-10-24

## What we access
- **Basic profile** (`user.info.basic`): your TikTok open ID and display name, used only to confirm which account is connected.
- **Profile statistics** (`user.info.stats`): follower count, total likes and public video count for the connected account.
- **Public videos** (`video.list`): recent public video IDs, descriptions/titles, post time and current view, like, comment and share counters. Used to show per-video performance for the connected account.
- **Video upload** (`video.upload`): permission to send videos to your TikTok inbox as drafts.
- **Direct Post** (`video.publish`): not requested or supported by this build. Direct Post remains disabled until eligibility is verified and per-post consent controls exist.

The connector does not request follower lists, messages, other users' private data, audience demographics, watch-time/retention, or payment information. It does not treat public video counters as retention, purchase or demographic data.

## How data is stored
OAuth access and refresh tokens are stored as JSON files under `<dataDir>/tokens` on the host running MiuNova, including a VPS when used. New token files are written with operating-system mode `0600`. MiuNova does not encrypt these token files itself; the instance owner and anyone with sufficient access to the host or its backups may be able to read them. Tokens are sent to TikTok's official API for code exchange, refresh and authorized posting operations. MiuNova does not display token values in the browser UI.

Videos and posting metadata are stored on the MiuNova host according to the instance's storage and backup configuration. They are sent to TikTok when the operator starts a draft upload.

Profile and public-video metric snapshots are stored in the instance's SQLite database so the operator can compare connected-channel and video performance over time. Missing TikTok fields remain unknown; MiuNova does not fill them with zero or derive audience age or watch time.

## Sharing
MiuNova does not sell or rent TikTok account data. TikTok receives authentication requests, authorized Display API requests and draft uploads. Hosting providers may process infrastructure data according to the instance's hosting arrangement. Other AI providers may receive prompts or media when selected for content generation; metric snapshots are not sent to them by the growth sync.

## Retention and deletion
Tokens and metric snapshots remain on the MiuNova host until the instance owner deletes them or the host's data is removed. Revoke access in TikTok under Settings and privacy → Security → Manage app permissions; remove the token file and any retained backup copies. Generated media, metrics and metadata may also remain in the instance database, output directory or backups until deleted by the operator.

## Children
The tool is not intended for anyone under 18.

## Contact
CONTACT_EMAIL
