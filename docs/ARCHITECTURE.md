# Kiến trúc code hiện hành

MiuNova phục vụ hai flow TikTok: affiliate và viral. Xem [System blueprint](SYSTEM_BLUEPRINT.md) cho data contracts và automation boundaries.

## Gói cốt lõi

| Gói | Vai trò |
|---|---|
| `cmd/aicos` | App setup, cấu hình và khởi chạy on-demand (không chạy daemon ngầm) |
| `internal/web` | Go templates, HTMX routes/partials và operator controls |
| `internal/network`, `internal/tiktok` | Kênh, account và TikTok OAuth |
| `internal/studio` | Radar video, job, media, audio, QC và artifact |
| `internal/products`, `internal/accesstrade` | Product Radar, evidence, offer, link, order |
| `internal/publishers` | TikTok Draft và trạng thái provider |
| `internal/growth` | TikTok snapshots, attribution, insights và feedback |
| `internal/webai`, `internal/engines` | Media providers cần cho hai flow |
| `internal/ledger`, `internal/backup` | State, audit trail và backup |

## Hợp đồng nguồn, publish và metrics

- Video/audio source thuộc kết quả TikTok Radar; lưu ID, URL, author, collected-at và audio metadata với job.
- Video cuối phải giữ audio từ nguồn Radar đã chọn. Source lỗi/thiếu audio thì job dừng.
- Draft khác public post; chỉ response/reconciliation có bằng chứng mới cập nhật trạng thái.
- Metrics lưu account/video ID, source, timestamp và window. Unknown không được ghi thành số 0.
- Recommendation phải trỏ tới evidence, sample size, cấu hình trước/sau và thao tác rollback.
