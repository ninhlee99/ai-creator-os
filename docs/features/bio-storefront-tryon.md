# Bio storefront and virtual try-on

## Current implementation

- The storefront has a separate detail page for each product, showing available
  source images, current/original price, discount, eligible rating/sold count,
  shop, freshness, and active voucher details. Rating and sold count appear
  only when both have a recent HTTPS evidence source; missing or stale evidence
  keeps the numbers hidden. Manual evidence links provide traceability, not
  independent verification. Unknown values stay unknown.
- Channels publish products independently. `/bio` opens only the explicitly
  selected default published channel; it never chooses the first account.
- `/bio-admin` manages profile publication, default channel, product order,
  per-channel visibility, and vouchers. `/bio-admin/analytics` shows channel
  and product metrics with a 30-day chart.
- Anonymous access is restricted by an exact Bio route/method allowlist.
  Dashboard, admin, analytics, APIs, and other routes fail closed remotely.
- Product views, profile views, outbound intent, and voucher copies are
  recorded with a 400-day retention limit.
- Profile and product views are deduplicated per client IP and page for
  30 minutes. Repeated click and voucher events for a product are deduplicated
  for 30 seconds. A short-lived, page/action-scoped HMAC digest is stored for
  cross-restart dedupe. Its signing key is separate and mode 0600 in the data
  directory; raw IP and cross-page visitor IDs are not stored. Expired digests
  are swept every 15 minutes. Shared networks can count several people as one
  view.
- Bio stays a product storefront: product photos, current price, discount,
  product facts, shop, freshness, and seller voucher remain the source listing.
  AI try-on is a separate, optional preview.
- Try-on uses Gemini image generation with the shopper photo and product photo
  as references. It creates a 3D-style image, not a 3D garment mesh or a fit
  simulation. The result is labeled as an estimate and shown beside both source
  photos.
- Creating a try-on image requires Google sign-in. Google subject identifies
  the shopper for quotas; the app stores a keyed hash of that subject plus the
  verified email and display name used in the admin account list.

## Public/private boundary

Anonymous visitors get published bio routes: profile, product detail,
voucher-copy, affiliate redirect, and legacy `/bio-go/{id}` or
`/bio-go?id={id}`. Google login and callback are allowlisted. Try-on upload,
status, and result routes require the same active Google account session;
opaque job IDs alone do not grant access. Every other same-origin route stays
private, including dashboard pages, APIs, `/bio-admin`, analytics, and
`POST /bio/pin`. When no dashboard password exists, remote requests outside
the public allowlist fail closed; loopback administration keeps working. A
valid dashboard session permits administration.

The public template has no same-origin link outside the bio namespace. Purchase
and social links may leave the app for their configured external destination.
`/bio` redirects only to an explicitly configured default public profile; it
never guesses an account. Unknown, private, or unpublished channels return 404.

## Product and channel data

- Keep provider facts on `products`: current price, original price, currency,
  source update time, images, shop, rating, sales, and source URL. Preserve
  existing rows with additive SQLite migrations.
- Store channel publication separately: account ID, product ID, published
  state, display order, optional voucher code/value/expiry/terms, and timestamps.
  A product appears only on channels where it was explicitly published.
- Bio publication requires at least one valid HTTP(S) product image URL and a
  valid HTTP(S) purchase URL. Admin shows the missing requirement; the server
  rejects publishing, and public profile/detail/voucher/buy routes hide items
  without both. Public pages render only validated HTTP(S) image URLs.
- Each Bio listing can override catalog images with up to eight channel-specific
  HTTP(S) image URLs or owner-uploaded JPEG/PNG product photos (one file per
  save, maximum 8 MB, 12 million pixels). Uploaded shop photos are stored in a
  private directory and served only when referenced by that listing; removed
  files are deleted after the listing update succeeds. Upload strips EXIF/GPS,
  XMP, IPTC, comments, text and timestamps while preserving image pixels, color
  profiles, and orientation. Blank override falls back to provider/catalog
  images. These are seller product photos, not shopper try-on uploads.
- Each product can carry a channel-specific, owner-entered description for
  verified material, color, size, and care details. Empty fields stay absent;
  Bio never invents product specifications.
- Admin shows source shop and price-update freshness before publication so
  the owner can spot manual or stale catalog data.
- Preserve unknown values as unknown. Never invent a price, discount, voucher,
  rating, sales count, or price timestamp. Label unknown price freshness for
  buyers; show stale product data as stale. Show an offer only while its
  source/owner-provided expiry is valid.
- `/bio-admin` lets the owner publish profiles/products and manage vouchers.
  It remains behind dashboard authentication.

## Try-on flow and quotas

1. The buyer signs in with Google before the upload form appears. OAuth uses
   `openid email profile`, validates Google’s verified email claim, and
   identifies the account by Google’s stable subject (`sub`), not email alone.
   A random, `HttpOnly`, `SameSite=Lax` session cookie scoped to `/bio` maps to
   a server-side hashed session with a 30-day expiry. The app does not retain
   Google access tokens. Logout revokes that session; blocking an account from
   `/bio-admin/try-on-accounts` revokes all its active sessions.
2. Accept one JPEG, PNG, or WebP upload, maximum 8 MB, 12 million pixels, and
   6000 pixels per side. Decode and normalize to a maximum 1536-pixel edge;
   re-encoding removes EXIF, GPS, and other metadata. Reject malformed or
   oversized files before queueing.
3. Verify that the channel profile and product are public, the Google account
   is active, and the buyer has consented to sending the photo to Google AI.
   Hash the normalized image with a server-held HMAC key; never persist the raw
   upload as analytics or expose it outside the private result directory.
4. In one SQLite transaction, reuse an unexpired job for the same Google
   account, product, and image hash. Otherwise enforce at most three accepted
   generations per account/product and fifteen per account/day, using
   `Asia/Ho_Chi_Minh` calendar days. Duplicate reuse does not consume quota.
   An accepted reservation consumes one attempt even if rendering later fails;
   rejected requests before reservation do not. The admin page reports usage
   by Google email and product and lets the owner block or re-enable accounts.
5. Persist the normalized source photo in a private `bio-tryon-results`
   directory and queue the job. A single background worker claims jobs and
   calls Gemini with shopper and product references. External product images
   must use HTTPS, resolve only to public IPs, and cannot redirect. Uploaded
   source photos are deleted after processing. Status and result routes
   require the matching signed-in Google account; local file paths are never
   sent to the browser.
6. Queued jobs expire after 15 minutes; running jobs after two hours; failed
   jobs after 15 minutes; successful results after 24 hours. The retention
   worker runs every 15 minutes, removes source/result files from the private
   directory, and sweeps stale crash leftovers after a grace longer than the
   maximum job lifetime. It removes expired job rows and image hashes, plus
   daily account quota buckets older than 400 days. Per-product account
   counters remain to enforce the three-use cap. Paths outside that directory,
   symlinks, and unexpected filenames fail closed.

Quota failure must be atomic across per-product and daily account counters.
Parallel requests must not pass the same final slot. Store idempotency and
quota counters in the same product database transaction; use a unique
constraint for duplicate requests.

## Google OAuth setup

- Configure `BIO_GOOGLE_CLIENT_ID` and `BIO_GOOGLE_CLIENT_SECRET`. If both are
  empty, Bio uses the existing `YOUTUBE_CLIENT_ID` and `YOUTUBE_CLIENT_SECRET`.
- For a remote deployment, set `BIO_GOOGLE_REDIRECT_URI` to the exact HTTPS
  callback registered in Google Cloud OAuth, ending in
  `/bio/google/callback`. Local loopback can derive its callback automatically.
- Add the callback URL to the OAuth client's authorized redirect URIs. Request
  only `openid email profile`; no Google account data or tokens are used for
  other purposes.

## Fidelity boundary

Gemini creates a 2D generated image with a 3D-rendered style. It does not build
a garment mesh, calculate measurements, or prove fit. A catalog photo may hide
garment details, and generated output can alter fabric, color, pattern, face,
or body. Show the untouched product and shopper photos beside the result. Do
not label generated output as an exact product photo or use it to promise fit,
size, or color accuracy.

## Analytics

Recorded events include profile/product views, affiliate exits, voucher copies,
try-on starts, successes, failures, and quota denials. Do not store raw IP,
Google subject, session tokens, or uploaded images in analytics. Google email
and display name are stored in the restricted account usage table so the owner
can audit and manage quotas.

The authenticated dashboard shows 30-day trends, per-product interest, raw
outbound click counts, and voucher engagement. Do not call clicks/profile
views a click-through rate: visitors can click from product-detail pages, and
page views use a 30-minute IP dedupe. Label outbound clicks as intent, not
purchases; show order/sales conversion only when an affiliate postback supplies
verified conversion data. Aggregate by
`Asia/Ho_Chi_Minh`; event retention is bounded to 400 days.

Analytics and product controls are private; bio visitors receive no analytics
route or control-plane link.

## Delivery gates

- Public users can reach only published bio/product/try-on/purchase routes.
- Each product appears only on its explicitly selected channel, with verified
  prices and non-expired vouchers.
- Try-on generation requires an active Google account session and explicit
  consent; another account cannot inspect a job or its result.
- Repeated identical requests by the same Google account return the retained
  result without using quota; concurrent requests cannot exceed either quota.
- Account daily caps stop at fifteen; per-product account caps stop at three
  accepted unique generations.
- Uploads are size/pixel limited, metadata-stripped, temporary, and deleted on
  schedule. Failed/expired results fail closed and do not leak local paths.
- Analytics distinguish page interest, outbound clicks, and verified sales.
- Try-on is not marketed as exact unless a validated 3D/rendering pipeline can
  prove the claimed fidelity for the supported product and shopper inputs.
