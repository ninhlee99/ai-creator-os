/*
Package publishers — operator notes.

Publication behavior by platform
--------------------------------
The adapters do not share one draft model:

  - TikTok: uploads to the creator inbox
    (/v2/post/publish/inbox/video/init/). Direct Post stays blocked until
    eligibility is verified and per-post consent controls exist. Setting
    TIKTOK_DRAFT_ONLY=0 does not bypass this gate. Draft uploads do not send
    TIKTOK_PRIVACY; TikTok's app handles final visibility.
  - Facebook: FB_DRAFT_ONLY=1 (default) leaves the video unpublished. The
    Graph API uses published=false; the CLI path creates a native Page draft.
    Both skip live publishing for operator review.
  - Instagram: there is no draft endpoint. By default the adapter uploads and
    waits for processing, then stops before media_publish. IG_AUTO_PUBLISH=1
    publishes the Reel directly.
  - Threads: publishes a text post with a link immediately; it does not upload
    the rendered video.
  - YouTube: YOUTUBE_DEFAULT_PRIVACY defaults to "private". Flip to
    "unlisted" (or "public" after review) per channel. A per-video
    VideoMeta.Privacy override takes precedence when a pipeline requires a
    stricter state; Story forces "private" regardless of this default.

Quota notes
-----------
  - TikTok: ~15 posts/day per creator, shared across ALL API clients
    (Direct Post); 6 req/min per user token on publish endpoints.
  - YouTube: videos.insert has a separate default bucket of 100 calls/day,
    at one quota unit per call. The bucket is project-wide and resets at
    midnight Pacific Time; other methods share a separate 10k-unit/day bucket.

TikTok audit gates (human steps, not code)
------------------------------------------
 1. Developer app with the Content Posting API product enabled.
 2. App audit (~1-2 wks) + Direct Post audit (~5-10 biz days) before PUBLIC
    posts are allowed. Until then: max 5 test users, SELF_ONLY visibility.
 3. Current build requests video.upload only. Do not request video.publish
    until eligibility and per-post consent gates are implemented.

Token hygiene
-------------
OAuth token files (tiktok_token_<username>.json,
youtube_token_<username>.json) are gitignored and never logged. Publish
outcomes are recorded by the orchestration layer via ledger.Decide only.
*/
package publishers
