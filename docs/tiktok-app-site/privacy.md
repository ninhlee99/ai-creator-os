# Privacy Policy — MiuNova TikTok Connector

Last updated: 2026-10-24

## What we access
- **Basic profile** (`user.info.basic`): your TikTok open ID and display name, used only to confirm which account is connected.
- **Video upload** (`video.upload`): permission to send videos to your TikTok inbox as drafts.
- **Direct Post** (`video.publish`): not requested or supported by this build. Direct Post remains disabled until eligibility is verified and per-post consent controls exist.

The TikTok connector does not request follower lists, messages, TikTok analytics, other users' data, or payment information.

## How data is stored
OAuth access and refresh tokens are stored as JSON files under `<dataDir>/tokens` on the host running MiuNova, including a VPS when used. New token files are written with operating-system mode `0600`. MiuNova does not encrypt these token files itself; the instance owner and anyone with sufficient access to the host or its backups may be able to read them. Tokens are sent to TikTok's official API for code exchange, refresh and authorized posting operations. MiuNova does not display token values in the browser UI.

Videos and posting metadata are stored on the MiuNova host according to the instance's storage and backup configuration. They are sent to TikTok when the operator starts a draft upload.

## Sharing
MiuNova does not sell or rent TikTok account data. TikTok receives the data needed for authentication and the draft upload. Hosting providers may process infrastructure data according to the instance's hosting arrangement. Other AI providers may receive prompts or media when selected for content generation; their own terms apply.

## Retention and deletion
Tokens remain on the MiuNova host until the instance owner deletes the token file or the host's data is removed. Revoke access in TikTok under Settings and privacy → Security → Manage app permissions; remove the token file and any retained backup copies. Generated media and metadata may also remain in the instance's output directory or backups until deleted by the operator.

## Children
The tool is not intended for anyone under 18.

## Contact
CONTACT_EMAIL
