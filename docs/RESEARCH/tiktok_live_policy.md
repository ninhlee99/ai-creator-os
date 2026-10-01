Research complete. Here is the structured brief for the parent agent.

---

# TikTok LIVE Programmatic Access & Policy Brief (as of Oct 2026, Vietnam focus)

## 1. Getting RTMP server + stream key / starting a LIVE

- **How it works (official, manual):** Eligible accounts (18+, 1,000+ followers, region where offered) get RTMP access via **TikTok LIVE Center desktop (livecenter.tiktok.com/producer)** — create a session (title/category/description), then the **Server URL (RTMP)** and **Stream Key** appear under Stream Settings. Alternatively in the mobile app via **"Cast to PC" / "Live Studio"** under the LIVE tab. Stream keys may **rotate per session** — fetch fresh each time. (Sources: https://www.oscal.hk/blog/app/how-do/get-tiktok-rtmp-key, https://onestream.live/blog/how-to-stream-on-tiktok-with-obs/)
- **No official API to start/stop a LIVE.** There is no TikTok-sanctioned endpoint for programmatic LIVE session control. Community tools (e.g., the Streamlabs-token OBS script at https://github.com/jeerum/TikTok-Streamlabs-RTMP-Tool-OBS-Script-) reverse-engineer private endpoints to fetch keys and end sessions — **unofficial, can break, ToS-risky**. FFmpeg can push to the RTMP URL once the key is obtained manually, which is the compliant path the repo already uses.

## 2. Reading LIVE chat / gifts / likes programmatically

- **No official API exists.** Confirmed by multiple independent sources: TikTok's developer surfaces (Display API, Content Posting API, Research API, API for Business) expose **zero** real-time LIVE webcast events. (Sources: https://github.com/isaackogan/TikTokLive FAQ, https://github.com/nglmercer/tiktok-signer/blob/HEAD/docs/10-authorized-api-feasibility.md, https://github.com/eulerstream/tiktok-live-api)
- **Unofficial options only:**
  - Open-source reverse-engineered clients: `TikTokLive` (Python, https://github.com/isaackogan/TikTokLive), `tiktok-live-connector` (Node, https://github.com/zerodytrash/tiktok-live-connector/blob/HEAD/README.md) — free, read the same Webcast websocket any viewer gets, break when TikTok changes signing.
  - Managed service: EulerStream (https://github.com/eulerstream/tiktok-live-api) — hosted signing + WebSocket, free community tier (2,500 req/day), paid tiers above. Still **unofficial, unaffiliated with TikTok**.
- **What IS official and useful:** TikTok Shop **Partner Center Open API** has an **Affiliate Creator** surface, including `POST /affiliate_creator/202405/open_collaborations/products/search` — creator-side search of open-collaboration products **sortable by `commission_rate`, `commission`, `units_sold`, price** (page_size ≤ 20). This is the legitimate backbone for the product-hunter agent. (Source: https://github.com/gaoyangz77/rivonclaw/blob/HEAD/docs/API/TIKTOK_SHOP/AFFILIATE_CREATOR.md; portal: https://partner.tiktokshop.com/docv2/page/about-partner-center-console)

## 3. Policy on AI-generated / virtual / synthetic streamers — ⚠️ THE BIG FINDING

- **TikTok Shop has banned AI-generated voices from shopping livestreams** (rules published in TikTok Shop Academy, effective May 2026, enforcement hardened July 2026 via the new **Account Health Rating** 0–1,000 system): *"Don't use non-real-time verbal interaction such as AI-generated voices, audio recordings, or radio."* Hosts must *"engage directly with viewers using real-time verbal or sign language communication."* Streams relying on TTS/AI voice are classified as **non-compliant content**. (Sources: https://ppc.land/tiktok-shops-quality-rules-ban-ai-voices-and-still-images-from-lives/, https://www.techtimes.com/articles/320624/20260715/tiktok-shop-bans-ai-voices-live-commerce-streams-violations-now-dent-account-health-score.htm, https://www.pymnts.com/news/ecommerce/2026/tiktok-shop-bans-ai-voices-from-livestreams/)
- **Related production bans in the same policy:** still-frame/looping content covering **>50% of screen** (slideshows, looping footage, screen recordings, PDP screenshots); **animated figures that are not a live person covering >50% of screen** — i.e., a full-screen AI avatar host is also non-compliant; shoppable videos need ≥3s dynamic content, real-world environment, creator's face + physical product.
- **Enforcement:** via **Creator Health Rating** (creators) and **Account Health Rating** (sellers, 180-day rolling window, no reset). Thresholds: 150 pts = can't join mega campaigns/new listings 7 days; 100 pts = 14 days + reduced livestream traffic; 0 = potential permanent deactivation. Separately, the **June 2026 Creator Enforcement Policy**: 6 violations in a rolling 90-day window → immediate loss of e-commerce permissions + frozen commissions regardless of score.
- **Platform-wide AI disclosure rule (separate):** Community Guidelines require creators to **label AI-generated or significantly edited content showing realistic people/scenes** ("Creator labelled as AI-generated"); TikTok auto-labels via C2PA credentials; undisclosed realistic AIGC *"may be removed, restricted, or labeled."* Per 2026-H2 guideline text, **generic TTS narration alone** (non-impersonating voice) reportedly does not trigger the *disclosure* label — but that does **not** override the Shop live-commerce ban on AI voices. (Sources: https://ryla.ai/en/blog/ai-content-disclosure-rules/, https://www.tiktok.com/community-guidelines/en/integrity-authenticity — page itself failed to render via text fetch, content verified via crawled snippets and secondary coverage)
- Note the tension: TikTok simultaneously pushes AI creative tools (Symphony/Dreamina) for *production*, but bans AI as the *customer-facing live salesperson*. Douyin (China) allows 24/7 virtual hosts at scale — **TikTok international does not follow Douyin's permissiveness here**.

## 4. Spam / automation / 24-7 stream risks

- **Community Guidelines — Spam and Deceptive Behavior:** bans *"use of automation to register or operate accounts in bulk, distribute high-volume commercial content, artificially increase engagement signals, and circumvent enforcement."* Violation → account ban (and linked accounts). (Source: https://www.tiktok.com/community-guidelines/en/integrity-authenticity?cgversion=2024H1update#3)
- **LIVE-specific:** sessions violating policy may be stopped mid-stream; repeated violations → temporary LIVE restrictions → account ban. **Creators are liable for third-party tools** (voice-to-text, comment overlays) used in their LIVE. (Source: https://www.tiktok.com/community-guidelines/en-GB/accounts-features)
- **TikTok Shop 2026 compliance stack:** daily posting limits (May 2026 Content Policy), misleading-claims enforcement, Promotion Performance Score (daily 0–5 for affiliate creators), Shop Performance Score (2.5 floor for affiliate access). Repetitive/looping affiliate streams hit both the AI-voice rule and the still-frame rule.
- **Vietnam-specific enforcement lore:** common VN ban triggers include phone numbers in live, links to external platforms, **replay abuse** ("lạm dụng phát lại video" — streaming pre-recorded video as live), copyrighted audio. (Source: https://limoseo.vn/nguyen-nhan-bi-cam-livestream-tren-tiktok/)

## 5. Vietnam-specific regulatory notes

- **State tightening (Sept 2026):** Under Nghị quyết 57-NQ/TW, Bộ Công Thương is requiring platforms to control goods, enforce seller authentication, combat counterfeits; **two-way product verification (seller ↔ manufacturer) before each live session is becoming a mandatory process**; livestream content under tighter monitoring, especially affiliate-linked selling. (Source: https://vietstock.vn/2026/09/nghi-quyet-57-nqtw-siet-kiem-soat-livestream-ngan-hang-gia-tren-moi-truong-so-4265-1497524.htm)
- TikTok Shop Creator VN's official account has posted about AI-generated content rules on TikTok Shop, confirming VN has a local AI-content policy surface — details not retrievable. (Source: https://www.tiktok.com/@tiktokshopcreator.vn/video/7648093603360951573 — video page, content not fetchable as text)

## 6. What I could NOT verify

- Whether the **AI-voice ban in shopping livestreams is enforced identically in Vietnam** — all primary documentation found is from TikTok Shop's US Academy and Western press. TikTok Shop VN Academy may mirror it, but I could not confirm from an official VN source.
- The exact current text of TikTok's official community-guidelines AI section (tiktok.com pages fail on text fetch — JS app error); relied on crawled snippets + reputable secondary summaries.
- Whether TikTok offers any **whitelisted/enterprise path** for automated live commerce (e.g., approved virtual-host programs) — none found publicly.

## 7. Architecture implication (for parent)

The user's core requirement — *"fully automated, TTS-voiced AI avatar livestream selling affiliate products 24/7"* — **directly collides with TikTok Shop's current live-commerce policy** (AI voices banned, non-human animated figures >50% screen banned, looping content banned). Building the TTS-avatar livestreamer as specced risks progressive AHR/CHR penalties → reduced livestream traffic → commission freeze → account deactivation, i.e., losing the valuable fully-permissioned VN account. Compliant alternatives to present to the user: (a) AI for everything *except* the live voice — human host on mic with AI driving overlays, chat triage, product moments, and scheduling; (b) full-AI pipeline for short-form shoppable videos (where generic TTS + disclosure label is the workable path); (c) use the official Affiliate Creator Open API for product hunting and commission reconciliation. No code was written per instructions.
## Bổ sung (2026-10-01, kiểm chứng thêm qua web search)

- Phạm vi lệnh cấm được mọi nguồn mô tả nhất quán là **promotional livestreams / live commerce** (TikTok Shop). Không tìm thấy nguồn nào ghi TikTok cấm giọng AI trong live giải trí thuần túy (vd. live game không bán hàng). Nguồn: metricool.com/tiktok-news, techtimes.com, ppc.land, pymnts.com.
- Digital avatar **không bị cấm hoàn toàn** — chỉ bị giới hạn không chiếm quá 50% màn hình (TechTimes FAQ).
- AI dùng trong **pre-production** (viết kịch bản, edit, dịch, tạo background) vẫn được phép; công cụ Symphony của TikTok cho advertiser vẫn được dùng.
- Hệ quả kiến trúc: live game/giải trí **không dùng tính năng Shop** nằm ngoài phạm vi điều khoản này — nhưng ranh giới bị vượt ngay khi ghim giỏ hàng / bật tính năng bán hàng trong live.
