# Policy & Safety

Non-negotiable rules encoded in the system. Violating any of these is a bug.

## 1. Account protection (the scarcest asset)

- Max 120 min per live session (tunable), mandatory breaks between sessions.
- No duplicate/spam content: content agent deduplicates scripts per product.
- AI disclosure overlay/text on every stream, per TikTok synthetic-media policy.
- No unofficial scraping or bypass tools for chat/gifts — ever. If TikTok
  offers no authorized event source, the show runs without live chat input
  rather than risking the account.
- One account. No multi-account schemes.

## 2. Money integrity

- Revenue/orders/commissions recorded ONLY from TikTok Shop API responses.
- Money tables append-only. No UPDATE/DELETE on orders/commissions.
- Idempotency keys on every external action: retries never double-post.
- Analyst proposes; governance disposes. No agent spends beyond caps.

## 3. Content integrity

- Blocked categories (drugs, unverified supplements, etc.) can never enter
  the shelf — enforced in `product_eligible`, not in prompts.
- Affiliate disclosure on content per platform rules.

## 4. Failsafes

- Global kill switch: stops agents + stream in seconds.
- Free-tier API caps are hard stops, not warnings.
- Dry-run is the default; live requires explicit checklist sign-off.
- Every governance decision logged with inputs (audit trail in `decisions`).

## 5. Privacy

- Secrets in env/Keychain only. Never in repo, logs, or chat transcripts.
- No viewer personal data stored beyond what TikTok's API returns for
  reconciliation; nothing is resold or shared.
