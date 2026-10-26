# System blueprint — TikTok core flows

## Phạm vi

Hai flow hiện hành: TikTok affiliate và TikTok viral. Truyện audio ở phase sau.

## Luồng dữ liệu

```mermaid
flowchart LR
  AT[Affiliate provider] --> PR[Product Radar]
  PR --> EV[Evidence + evaluation]
  EV -->|approved| AF[Affiliate campaign]
  EV -->|rejected| RJ[Rejected list]
  TR[TikTok Radar by topic] --> VA[Viral analysis]
  TR -->|dance source selected| AU[Audio extraction]
  AU --> AF
  AU --> VF[Viral remake]
  AF --> QC[QC + preview]
  VF --> QC
  QC --> RV[Publish review + consent]
  RV -->|Shop access| SP[Shoppable video + product anchor]
  RV -->|approved Direct Post| DP[Direct Post]
  RV -->|fallback| DR[TikTok Draft]
  SP --> PM[Post metrics]
  DP --> PM
  DR --> PM
  AT --> OM[Orders + commission]
  PM --> FB[Feedback + bounded adjustments]
  OM --> FB
  FB --> PR
  FB --> TR
```

## Thành phần

| Thành phần | Trách nhiệm |
|---|---|
| `internal/accesstrade` | Campaign, offer, affiliate link, order/commission sync khi provider hỗ trợ |
| `internal/products` | Candidate snapshots, category/rule evaluation, approved/rejected state, product usage |
| `internal/studio` | Affiliate/viral plan, media generation, Radar audio extraction, QC, job/artifact provenance |
| `internal/growth` | TikTok video snapshots, account baselines, funnel analysis, recommendations and rollback history |
| `internal/tiktok` + `internal/publishers` | TikTok OAuth, draft/direct-post adapters, Shop shoppable-video adapter, provider state/reconciliation |
| `internal/web` | Go templates, HTMX partials, operator review and controls |
| `internal/ledger` | Accounts, characters, decisions, audit records and shared settings |

## Data contracts

- Product candidate: offer provider + offer/product ID + optional TikTok Shop product ID + verified mapping state + category + price/currency + commission + source timestamp + snapshot ID.
- Evaluation: candidate snapshot + rule version + facts + unknowns + verdict + reason + confidence + actor + time.
- TikTok Radar video: provider + video ID/URL + author + topic + collected-at + metrics snapshot + audio metadata + dance classification state.
- Campaign/job: flow + account + approved offer ID + TikTok Shop product/anchor IDs and state if affiliate + selected Radar source + audio source + provider + artifact/QC + consent evidence + draft/publish state.
- Performance: account/video ID + metric name/value + provider + timestamp + window + attribution scope.
- Recommendation: evidence IDs + sample size + confidence + proposed setting diff + rule version + apply/rollback audit.

Unknown values stay null/unknown. Old snapshots remain immutable. Repeated syncs use provider IDs/idempotency keys.

## Automation boundaries

- Scheduled Radar/search and metric sync may run automatically under operator-configured limits.
- Product verdict may auto-approve only after hard gates, freshness and configured confidence pass; otherwise `needs_review`.
- Rejected products remain inspectable and re-evaluable.
- Studio generation may proceed only from approved product or selected TikTok Radar source.
- Every TikTok submission requires a review screen, correct account display, explicit per-post consent and truthful provider state. Direct Post public mode requires approved scopes/audit and permitted app use; MiuNova's internal-tool use must not assume eligibility. Shoppable affiliate publish requires TikTok Shop partner/creator authorization and a confirmed product anchor. Otherwise use Draft/manual handoff.
- Keep `upload/draft`, `publishing`, `public`, `failed`, and `reconcile_required` distinct. A successful upload response alone does not prove public post or attached product.
- Setting auto-adjustment is bounded, reversible and versioned. Offer choice, claims, publish rights, account permissions and provider budgets require operator approval.
- Preserve data in existing SQLite databases. Remove unused code and new-schema declarations without dropping existing user tables or rows.

## UI architecture

- Go `html/template` owns first render and page/partial HTML.
- HTMX sends form and action requests and swaps server-rendered partials.
- Tailwind CSS source is pinned and compiled to local static CSS during build; production does not depend on CDN.
- Stable URL per screen; HTMX history enabled only where page state is reconstructible.
- Errors render near the triggering action with source/provider and retry state; no optimistic success for provider actions.

## External capability gaps

Current repo has AccessTrade offers, links and orders; per-product sales/rating and TikTok video attribution are not proven. TikTok Radar uses TikTok Business Discovery after Access Token and Advertiser ID are configured. Discovery returns provider-ranked videos by popular hashtag but no view/like counters; topic search only matches trending hashtags. Radar does not call paid scraping providers or automate TikTok web scraping. Without valid Discovery credentials it reports required configuration and does not call TikWM. It stores at most 2,000 recent video records and 30 days of metric snapshots in SQLite; unsupported counters stay `unknown`. Discovery still needs live verification; dance classification stays operator-confirmed. It accepts only TikTok URLs with matching provider video IDs. TikTok Display API code reads follower/video totals and current public-video counters when scopes are granted; retention, watch-time, demographics and Shop product attachment are not proven end-to-end. TikTok Content Posting API supports direct post or draft upload, with audit, consent and intended-use conditions. TikTok Shop documents a separate shoppable-video path with product anchors and partner/creator authorization. Repo currently proves draft upload only. Keep each unsupported field `unknown` and gate related automation.
