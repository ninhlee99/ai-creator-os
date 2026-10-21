# Affiliate video production

Affiliate jobs use a staged product-to-video workflow. Product analysis and directing run first through `POST /studio/affiliate/plan`, before the operator records or generates motion clips. The server stores the resulting product brief and exact shot list for seven days; job creation consumes that plan and the worker reuses it. Each later stage writes evidence to the job log or work directory; the pipeline board reads those real markers rather than inventing independent agent activity.

## Production stages

1. **Product Analyst** inspects up to six listing images before clip production. It classifies product category, records visible details and evidence, blocks mismatched or uncertain products, and checks that requested focus is visible. Category-required physical use stays mandatory; operator notes can only add compatible details.
2. **Director** writes a category-grounded shot list before clip production. Every shot has a structured `product_use_mode` validated against product category (`wear`, `carry`, `apply`, `operate`, `prepare`, `consume`, `use`), plus physical interaction, framing focus, emotional beat, and shared campaign setting/wardrobe. The mode must remain valid in detail shots too; clothing stays worn, shoes stay on feet, and a bag stays carried/worn. The plan needs a hook, wide product-use coverage, visible detail coverage, payoff, and at least three distinct shot sizes. It receives channel Brand DNA and channel-scoped memory as low-confidence creative guidance; memory cannot override product evidence or use requirements. UI shows camera/action/focus plus a complete manual video prompt per shot. Edits to prep inputs invalidate the browser's prepared plan.
3. **Producer / Editor** receives one user-supplied motion clip per shot, rejects missing/short clips, normalizes each to its planned duration and vertical 9:16, joins them, and adds only supplied music or voiceover. The app does not call paid video-generation APIs; still-image montage is not a success path.
4. **Visual QC** first compares downscaled grayscale frames at 2 fps inside each shot and blocks frozen/nearly frozen footage. It then samples three output frames per shot against product and channel identity references. It checks product match/use, focus sharpness, professional composition, identity, emotional beat, continuity, and shot variety. Failure or uncertainty blocks completion. Pixel change detects frozen footage; it does not prove human motion or product use by itself.
5. **Growth Analyst** uses real YouTube per-video analytics for published plan items linked by platform video ID: views, shares, average view percentage, and subscribers gained. TikTok per-video analytics and video-attributed affiliate clicks/orders are not wired; those gaps stay visible and do not become growth claims.

## Operator inputs

- Choose or auto-detect product category.
- Add exact use instructions for unusual products.
- Add alternate listing angles/details and identify the visible product feature to focus.
- Create or record one motion clip per planned shot, in shot order, using the displayed prompt. Upload exactly the number of clips shown by the saved plan. The app normalizes clips but cannot prove how they were generated.
- Use only listing-supported product details. Do not add performance, material, health, or quality claims that are not verified.

## Limits

- Affiliate jobs still require operator-supplied clips. Separate Muse Chat / Google Flow generation is available from `/studio/create`; Flow access and account credits vary. Product and character references guide prompts and QC; they do not alter uploaded footage or guarantee exact reproduction. A model-generated QC report is not a substitute for human review.
- A completed render is not proof of views, engagement, affiliate clicks, orders, or revenue. YouTube format metrics require a matching `published_ref`; unlinked/manual posts and TikTok videos do not get per-video format conclusions.

Primary implementation: `internal/studio/product_analysis.go`, `internal/studio/director.go`, `internal/studio/affiliate_plan.go`, `internal/studio/affiliate.go`, `internal/studio/temporal_qc.go`, `internal/studio/visual_qc.go`, `internal/studio/prompts/affiliate_shotlist.txt`, `internal/web/studio.go`, `internal/web/team_pipeline.go`, and `internal/web/templates/studio.html`.
