/*
Package publishers — operator notes.

Draft-first policy
------------------
Every platform defaults to leaving the content in a reviewable state:

  - TikTok: TIKTOK_DRAFT_ONLY=1 (default) uploads to the creator inbox
    (/v2/post/publish/inbox/video/init/) instead of Direct Post.
    TIKTOK_PRIVACY defaults to SELF_ONLY.
  - Facebook: FB_DRAFT_ONLY=1 (default) creates a native Page draft and
    skips the publish step, so a human reviews the draft in the Page first.
  - YouTube: YOUTUBE_DEFAULT_PRIVACY defaults to "private". Flip to
    "unlisted" (or "public" after review) per channel.

Quota notes
-----------
  - TikTok: ~15 posts/day per creator, shared across ALL API clients
    (Direct Post); 6 req/min per user token on publish endpoints.
  - YouTube: videos.insert costs ~1600 units against the 10k/day default
    quota — a handful of uploads per day per project is the free-tier
    reality.

TikTok audit gates (human steps, not code)
------------------------------------------
 1. Developer app with the Content Posting API product enabled.
 2. App audit (~1-2 wks) + Direct Post audit (~5-10 biz days) before PUBLIC
    posts are allowed. Until then: max 5 test users, SELF_ONLY visibility.
 3. Request only the scopes you use: video.publish / video.upload.

Token hygiene
-------------
OAuth token files (tiktok_token_<username>.json,
youtube_token_<username>.json) are gitignored and never logged. Publish
outcomes are recorded by the orchestration layer via ledger.Decide only.
*/
package publishers
