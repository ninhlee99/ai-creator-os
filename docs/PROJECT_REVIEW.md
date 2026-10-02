# PROJECT REVIEW — Senior Expert Audit (2026-10-02)

> **Trạng thái:** tài liệu này là **spec đang điều phối các đợt sửa đang
> chạy** (Đợt 1–3). Khi các đợt hoàn tất, nội dung sẽ được gộp vào
> `docs/ARCHITECTURE.md` và file này sẽ bị xoá — giữ nguyên nguyên văn
> trong lúc điều phối để không lệch spec giữa chừng.

> Người rà soát: Milo (vai Senior Expert Engineer). Phạm vi: trạng thái đã commit tại
> `bc1a247` (growth giai đoạn 2 vừa lên main trong lúc rà soát — các nhận định về growth
> dựa trên đúng commit này, kiểm chứng bằng `git log`/`go vet`; xem mục 0.3).
> Nguyên tắc: mọi khẳng định đều có dẫn chứng `file:line` hoặc lệnh đã chạy.
> Chuẩn đánh giá (Ninh đã chốt): chỉ API chính thức cấu hình trong Settings; zero-touch
> (hệ thống tự quyết, con người chỉ bootstrap tài khoản/key/OAuth + giữ kill switch);
> mọi tính năng có UI/UX; không bao giờ bịa số liệu/tương tác; UI tiếng Việt;
> chỉ biến CSS theme; trạng thái fail-closed trung thực. Đây là **sản phẩm để bán**,
> không phải MVP — chấm theo chuẩn đó.

---

## 0. Kết luận tổng

### 0.1 Điểm số tổng thể: **6,5/10 theo chuẩn "bán được giá cao"** (8/10 theo chuẩn MVP tốt)

Phần lõi kỹ thuật tốt hơn nhiều so với vẻ ngoài của nó: kiến trúc module dưới tầng web
khá sạch, kỷ luật trung thực (fail-closed, không bịa số) được giữ nghiêm túc, test xanh
toàn bộ. Nhưng sản phẩm chưa bán được ở trạng thái hiện tại vì bốn lý do lớn:

1. **Có dữ liệu giả ngay trong app thật** — trang Agent Team tự bịa 3 "tác vụ demo" khi
   chưa có job nào (`internal/web/team.go:127-136`). Với một sản phẩm bán bằng niềm tin
   vào automation, đây là lỗi nghiêm trọng nhất: người mua mở app lần đầu sẽ thấy việc
   không có thật đang "chạy".
2. **Quá nhiều trang trùng nhau** — 3 bề mặt báo cáo (Trang chủ / Phân tích / Phát triển
   kênh), 2 trang sản phẩm affiliate (`/products` + `/shop`), 2 trang làm video
   (`/content` + `/studio`). 79 route cho một app một người dùng. Người không rành kỹ
   thuật (đúng persona Ninh hướng tới) sẽ lạc.
3. **Zero-touch chưa phải mặc định** — tự động hoá growth đã nối xong (bc1a247) nhưng
   công tắc mặc định TẮT; lịch live vẫn có nút "xây lại" thủ công; số followers vẫn gõ
   tay. Hệ thống *có khả năng* tự vận hành nhưng chưa *tự vận hành ngay khi mở lên*.
4. **Chưa có vỏ sản phẩm** — không LICENSE, không CI/release pipeline, version còn ghi
   `v0.5-go` (`cmd/aicos/main.go:49`), không backup/restore trong UI, không chế độ demo
   cho người mua dùng thử, dashboard không có lớp đăng nhập nào khi phơi ra LAN.

### 0.2 Bằng chứng nền đã kiểm chứng

| Kiểm chứng | Kết quả |
|---|---|
| `go vet ./...` | Sạch, không cảnh báo |
| `go test ./...` | **21/21 package OK**, ~298 test (`grep -rc "func Test"`) |
| `go.mod` | 1 dependency trực tiếp duy nhất (`modernc.org/sqlite`) — cực kỳ gọn, dễ bán/đóng gói |
| TODO/FIXME trong code | 1 chỗ duy nhất (`internal/agents/hunter/hunter.go:40`) |
| Route đã đăng ký | 79 (`grep -c HandleFunc internal/web/handlers.go`) |
| File lớn nhất | `internal/web/handlers.go` 1.638 dòng; `internal/web/templates/settings.html` 739 dòng; `cmd/aicos/main.go` 777 dòng |

### 0.3 Lưu ý về growth

Growth giai đoạn 2 (bc1a247) đã đóng các khoảng trống lớn nhất của MVP: daemon tick 5 phút
nối plan → Studio (idempotent, tối đa 3 job/tài khoản/tick, item quá hạn 14 ngày tự dropped),
biến thể đa nền tảng + `concept_hash` (Jaccard ≥ 0,8 ép đổi góc kể), đăng YouTube fail-closed
kèm canh quota 1600/10.000 units, đồng bộ số liệu nền mỗi giờ. Các nhận định về growth dưới
đây đã tính cả commit này. Giới hạn còn lại của growth (theo chính ghi chú commit + code):
renderer Studio chỉ có khung dọc 9:16 (chưa có bản dài 16:9 thật), 1 project OAuth chung
chưa tách theo kênh, TikTok draft-only, YouTube Analytics API (views-30d/giờ-xem) còn nợ.

---

## 1. Rà soát từng URL

Tiêu chí chấm (0–10): dữ liệu thật (không giả/không mồ côi) · mức tự động (ít click thừa) ·
trạng thái trống/lỗi trung thực · nhất quán UI/tiếng Việt · giá trị với người mua.

| Route | Điểm | Nhận định chính |
|---|---|---|
| `GET /` Trang chủ | 7 | Dữ liệu thật từ ledger; checklist 3 bước tốt cho người mới. Nhật ký quyết định phơi nguyên tên agent kỹ thuật (`growth_director · generate_plan`) — người không rành sẽ không hiểu. |
| `GET /accounts` | 6,5 | Danh sách mỏng; việc thật nằm ở trang chi tiết. Ngưỡng "tối thiểu N followers để live" là đúng trọng tâm. |
| `GET /accounts/new` | 6 | Tạo xong tự chạy onboarding nhiều bước là điểm cộng lớn (tự gán persona/research). Nhưng theme/persona vẫn phải chọn tay từng tài khoản — nên có gợi ý tự động. |
| `GET /accounts/{id}` | 5,5 | "Kitchen-sink": transition, followers gõ tay, YouTube, theme, autopilot, upload ảnh mẫu, live-topic, replan, khối growth — quá nhiều form POST trên một trang. Ô nhập followers thủ công mâu thuẫn zero-touch và là cửa ngõ bịa số. |
| `GET /schedule` + `POST /schedule/build` | 6 | Daemon đã tự xếp lịch mỗi ngày (`internal/network/daemon.go`), vậy mà UI vẫn bắt bấm "xây lịch". Nút thủ công nên hạ xuống "xây lại ngay" phụ trợ. |
| `GET /content` + `POST /content` | **4** | Engine kinetic-typography đời cũ (`internal/web/handlers.go:530-607`, runJob riêng) trùng vai trò với Studio AI. Hai nơi làm video = người dùng không biết vào đâu. **Ứng viên gộp/xoá số 1.** |
| `GET /studio` (+ jobs/assets/trends/mediagen) | 7,5 | Trang chủ lực, polling cập nhật tại chỗ tốt, trạng thái có màu. Hạn chế cứng: chỉ render dọc 9:16 → mục tiêu "video dài YouTube" chưa có đường render thật. Hai form tạo (affiliate/film) nên giữ nhưng cần giải thích khác nhau rõ hơn. |
| `GET /team` | **5** | Vẽ từ Studio jobs thật — tốt. Nhưng khi chưa có job, app hiển thị **3 tác vụ demo bịa** (`internal/web/team.go:127-136`: "Video affiliate — túi kem quilted" đang running 55%). Vi phạm luật trung thực. Phải thay bằng empty-state có CTA. |
| `GET /products` | 6 | Tìm sản phẩm theo theme + kho cho autopilot. Chồng lấn với `/shop`. Tìm kiếm vẫn thủ công trong khi autopilot đã có chu kỳ tự chạy. |
| `GET /shop` | 5 | "Kệ hàng" là một view khác của cùng kho sản phẩm ở `/products`. Hai trang một dữ liệu. **Ứng viên gộp số 2** (tab trong /products). |
| `GET /analytics` | 5,5 | Doanh thu/quà/chi phí API — chồng lấn Trang chủ (doanh thu) và Phát triển kênh (views/followers). Bảng chi phí API quá kỹ thuật cho người dùng cuối. **Ứng viên gộp số 3.** |
| `GET /growth` (+ sync/plan) | 7,5 | Trang đúng triết lý nhất: giai đoạn, KPI, đếm ngược hạn YPP, luật tự quyết hiển thị rõ. Sau bc1a247 đã có tick tự động 5 phút + sync nền mỗi giờ, nhưng **toggle tự động mặc định TẮT** và nút "Đồng bộ + chạy vòng quyết định" vẫn là thao tác chính trên UI — thông điệp zero-touch bị loãng. Views-30d/giờ-xem vẫn trống chờ Analytics API. |
| `GET /publishers` (+ TikTok OAuth) | 6 | Bootstrap OAuth là việc con người làm một lần — chấp nhận được và đã fail-closed đúng. Thiếu chỉ dẫn từng bước cho người không rành (hiện phải đọc USER_GUIDE). |
| `GET /settings` (+ ~20 route con) | **4,5** | 739 dòng template, 8 mục: trạng thái hệ thống, biến môi trường, RTMP, nhân vật AI, 3 chuỗi provider, VieNeu, avatar sidecar, vùng an toàn. Đây là trang cho kỹ sư, không phải cho chủ kênh. Thuật ngữ env phơi thẳng; 3 khối chuỗi provider gần như copy-paste. Người mua phổ thông sẽ sợ trang này. |
| `GET /api/stats`, `/api/products`, `/api/decisions` | — | **Route mồ côi**: đã đăng ký (`handlers.go:129-131`) nhưng không có bất kỳ UI nào gọi tới (grep `fetch('/api` trong templates/static: rỗng). Hoặc tài liệu hoá thành API bán kèm (có token), hoặc xoá. Để lửng là bề mặt tấn công + dead weight. |

**Hành động người dùng đang phải làm mà máy nên tự làm** (chi tiết ở mục 4): bấm xây lịch,
bấm đồng bộ growth, bấm sinh plan, gõ followers, bấm tìm sản phẩm, bấm refresh trending
(chart đã có cache 6h — nên tự refresh khi quá hạn lúc mở Studio), bấm test/tải model ở
Settings (VieNeu/avatar sidecar đã có ensure/progress — nên tự ensure lúc khởi động khi
provider local được bật).


---

## 2. Kiến trúc

### 2.1 Bản đồ package & hướng phụ thuộc

```
cmd/aicos/main.go (777 dòng — composition root, ~10 adapter types)
        │  lắp ráp tất cả, khởi động daemon + tick growth 5' + HTTP
        ▼
internal/web (hub — import network×6, ledger×5, studio×4, publishers×4, growth×4, products×2, tiktok×1)
        │
        ├── internal/growth ──► (chỉ time ở lõi; KHÔNG import studio — đã kiểm chứng grep)
        ├── internal/studio ──► engines (mediagen/tts/llm), products
        ├── internal/network ──► ledger, agents/*
        ├── internal/publishers ──► tiktok, env Settings
        ├── internal/products ──► ledger (store riêng qua settings/db)
        └── internal/ledger (976 dòng — tầng DB dùng chung, nút thắt trung tâm)

internal/engines (tts/llm/avatar/local) ──► sidecar ngoài qua HTTP (llama-server, VieNeu, MuseTalk)
internal/agents/* (hunter/director/... qua orchestrator + team view)
internal/stream, internal/affiliatehunter (ADB), internal/tiktok (posting/shop)
```

**Đánh giá hướng phụ thuộc: tốt.** Tầng nghiệp vụ dưới web không phụ thuộc lẫn nhau bừa bãi:
growth không import studio (việc nối plan → Studio sống ở `internal/web/growth_produce.go:199`
`GrowthAutomationTick` — coupling nằm đúng ở tầng điều phối, xoá được). Xoá thử trong đầu:
xoá growth → web mất 2 file + khối account-detail, studio không hề hấn; xoá publishers →
chỉ mất auto-publish; xoá `/content` (engines/film qua `runJob` ở web) → sạch, không ai khác
dùng. Điểm trừ duy nhất: **web là hub quá to** và **ledger là DB-layer dùng chung cho mọi
thứ** (kèm settings table bị dùng làm cả kho env `env:` — xem 2.5).

### 2.2 Tính độc lập module (tiêu chí "thêm/bớt không vỡ")

- **Tốt:** growth, publishers, products, affiliatehunter, stream — biên rõ, test riêng.
- **Trung bình:** studio — bị web (autopilot wiring ở `cmd/aicos/main.go:602-703`), team
  (`Studio.ListJobs` ở `team.go:115`) và growth-produce cùng chạm; vẫn gỡ được nhưng phải
  sửa 3 nơi.
- **Yếu:** `internal/web` tự nó là một "module" 9.444 dòng (handlers 1.638 + autopilot 634 +
  growth_produce 573 + server 540...). Mọi tính năng đều sửa chung một package — đây là nơi
  dễ vỡ nhất khi thêm/bớt tính năng.

### 2.3 Điểm nóng coupling & file ngoại lệ

| File | Dòng | Vấn đề |
|---|---|---|
| `internal/web/handlers.go` | 1.638 | Chứa routes + dashboard + accounts + schedule + content + publishers + shop + analytics + settings-env + chains API. Phải là 5–6 file theo miền (đã có sẵn autopilot.go/studio.go/growth.go làm mẫu). |
| `internal/web/templates/settings.html` | 739 | 8 mối quan tâm trong 1 trang cuộn; JS fetch/poll viết tay lặp lại (vieneu, sidecar, chains dùng cùng mẫu ensure→progress mà không share helper). |
| `internal/growth/store.go` | 985 | 8 bảng + toàn bộ truy vấn trong một file; nên tách theo thực thể khi growth lớn thêm. |
| `internal/ledger/ledger.go` | 976 | DB dùng chung + decisions + slots + settings — chấp nhận được ở quy mô này, nhưng là nơi mọi thay đổi schema đều chạm. |
| `cmd/aicos/main.go` | 777 | ~10 adapter types (ttsChainAdapter, llmChainAdapter, avatarChainAdapter...) — đây là seam DI đúng đắn, nhưng file đã phình; nên tách `adapters.go`. |

### 2.4 DRY

- **Tốt:** `statusLabel`/`statusClass`/`formatVND` dùng chung thật (19/14/6 lượt dùng trong
  templates); template func nào cũng có người dùng (đã grep từng func — không thừa).
- **Vi phạm lớn nhất:** 3 khối UI chuỗi provider (TTS/LLM/Avatar) trong `settings.html`
  (~50 dòng × 3) + logic ở `internal/web/chains.go` (290 dòng) — cùng một mẫu giao diện lặp
  3 lần. Một renderer tham số hoá sẽ cắt ~100 dòng template và mọi sửa UI chỉ sửa một chỗ.
- JS polling/fetch: Studio có một bản, Settings có vài bản tự viết — nên có `static/app.js`
  dùng chung (toast, fetchJSON, poll).
- 56 thuộc tính `style="..."` inline trong templates — không thảm hoạ nhưng làm theme khó
  nhất quán; nên dồn vào class.

### 2.5 Cấu hình/settings — 4 lớp chồng nhau (rủi ro bán hàng)

1. Env lúc khởi động: `MASTER_SWITCH`, `DRY_RUN`, `KILL_SWITCH` (đọc ở `cmd/aicos/main.go`).
2. Settings UI lưu vào DB (`env:` keys, áp lại lúc start — `handlers.go:806-855`) + công tắc
   dry-run/kill qua `s.Cfg.SetDryRun/SetKillSwitch` (`handlers.go:1043-1066`).
3. Chuỗi provider lưu JSON riêng (chains) kèm API key trong đó.
4. Env theo tài khoản ở publishers (`envFor(username, ...)` trong `publishers/base.go:55`).

⚠️ **Phải kiểm chứng ngay (chưa kết luận là bug):** daemon network giữ `NetConfig` là bản sao
giá trị lúc khởi động (`network/daemon.go` — `d.cfg` đọc mỗi Tick), còn kill switch ở UI ghi
vào `s.Cfg` của web Server. Nếu hai nơi không cùng một nguồn sự thật, nút Kill trên UI có thể
không dừng được daemon live. Đợt 1 (mục 6) bắt buộc kiểm chứng + hợp nhất về một nguồn.

### 2.6 Xử lý lỗi & log

Nhất quán ở mức khá: handlers dùng chung `s.fail` + `log.Printf` có prefix (`web:`, `growth:`,
`autopilot:`), có recoverer chống panic sập server (`handlers.go:136`). Các endpoint JSON
(chain/keys/vieneu) trả lỗi theo kiểu riêng từng chỗ — nên chuẩn hoá một định dạng lỗi JSON.

### 2.7 Test

298 test, 21 package — phủ tốt ở tầng engine (growth 23+ test gồm dedup/quota/production sau
bc1a247; studio có test render ffmpeg thật). Điểm mỏng: `cmd/aicos` **0 test** (composition
root chứa wiring quan trọng nhất lại không có smoke test khởi động); tầng web có test nhưng
route 79 mà `web_test.go` 510 dòng — phủ một phần; template/CSS không có kiểm tra tự động
ngoài render smoke thủ công. Chưa có CI nên "test xanh" vẫn là kỷ luật tay.

---

## 3. Dead code & bề mặt thừa (chỉ liệt kê mục đã kiểm chứng)

| # | Mục | Bằng chứng | Xử lý đề xuất |
|---|---|---|---|
| D1 | 3 tác vụ demo bịa trên `/team` khi chưa có job | `internal/web/team.go:127-136` (demo-1/2/3, hardcode title/progress) | **Xoá ngay** — thay bằng empty-state thật có CTA sang Studio |
| D2 | 3 route `/api/stats`, `/api/products`, `/api/decisions` không có UI nào gọi | `handlers.go:129-131`; grep `fetch('/api` trong templates/static → rỗng | Quyết định: tài liệu hoá thành API có token (tính năng bán kèm) hoặc xoá |
| D3 | `LocalAvatarProvider.SupportsRealtime()` trả `true` trong khi local chỉ render offline | `internal/engines/avatar/local.go:85-89`; UI đã phải đi vòng tránh nó (`handlers.go:1026-1031` `avatarRealtime()` chỉ tính cloud) | Sửa về `false` (hoặc đổi contract thành `SupportsStreamingContract`), để không tầng nào bị lừa nữa — nợ đã ghi từ audit M1 |
| D4 | Trang `/content` + `runJob` kinetic đời cũ trùng Studio | `handlers.go:530-607`; sidebar có cả "Sản xuất video" và "Studio AI" | Gộp vào Studio (route redirect), xoá engine path riêng nếu không còn job loại này |
| D5 | Trang `/shop` trùng dữ liệu `/products` | `handlers.go:667-713` cùng kho products | Gộp thành tab "Kệ hàng" trong `/products` |
| D6 | Lớp CSS `badge-queued`, `badge-running`, `st-*` không thấy trong templates | grep trong templates/static: chỉ xuất hiện ở `style.css` | **Chưa đủ bằng chứng chết** (JS nối chuỗi `badge-'+jobClass(...)` ở studio.html) — phải đối chiếu lúc chạy trước khi xoá; ghi vào đợt dọn kho kèm kiểm chứng render |
| D7 | Tài liệu trùng: `docs/UI_UX_BLUEPRINT.md` vs `docs/UI_UX_REVIEW.md`; `docs/affiliate-pipeline-v2-design.md` vs `docs/CHANNEL_GROWTH.md`; thư mục `docs/RESEARCH/` (5 file) nằm ngoài mục lục chính | `ls docs/` — 12+ tài liệu | Hợp nhất còn ~5 tài liệu chuẩn ở Đợt dọn kho; RESEARCH chuyển vào `docs/research/` có index |
| D8 | Ô nhập followers thủ công | `handlers.go` `handleAccountFollowers` (~:405) ghi thẳng `SetFollowers` | Không phải dead code mà là **tính năng nên chết**: thay bằng chỉ-đọc từ metric snapshot; chưa có nguồn thì hiện "chưa kết nối" |

Không tìm thấy: template func thừa (đã grep đủ 9 func), TODO/FIXME rải rác (chỉ 1 chỗ ở
hunter), dependency thừa trong `go.mod`. Về dead code thuần Go, repo sạch hơn mặt bằng chung
rất nhiều — vấn đề của dự án không nằm ở code chết mà ở **trang thừa và mặc định chưa tự động**.


---

## 4. Khoảng trống tự động hoá (xếp theo giá trị tiết kiệm cho người dùng)

Nguyên tắc đối chiếu: Ninh chỉ (a) mở máy + mạng + pin, (b) start app, (c) bootstrap một lần
(tài khoản/key/OAuth/audit). Mọi thứ khác hệ thống phải tự quyết.

| # | Việc con người còn phải làm tay | Hiện trạng (dẫn chứng) | Mức tự động hoá đề xuất | Tiết kiệm |
|---|---|---|---|---|
| A1 | Bật công tắc tự động growth | Tick 5 phút đã nối (bc1a247) nhưng **mặc định TẮT** | Mặc định BẬT; an toàn dựa vào dry-run + kill switch (đúng triết lý đã chốt: dry-run là cổng, không phải toggle từng tính năng) | Cao nhất — đây là khác biệt giữa "có automation" và "zero-touch" |
| A2 | Gõ số followers cho tài khoản | `handleAccountFollowers` (`handlers.go` ~:405) | Số chỉ đến từ metric snapshot (YouTube Data API đã có; TikTok chờ OAuth). Xoá form gõ tay; chưa có nguồn → "chưa kết nối" | Cao (đúng luật không bịa số + bớt 1 việc/tài khoản) |
| A3 | Bấm "xây lịch" live | `POST /schedule/build`; daemon vốn đã tự plan mỗi ngày (`daemon.go` bước 2) | Tự xây khi mở ngày mới/thiếu lịch; nút hạ thành "xây lại ngay" | Cao |
| A4 | Bấm đồng bộ + sinh plan ở /growth | Sync nền mỗi giờ + plan engine đã có | Plan tự sinh khi tài khoản được tạo/đổi theme; sync tự chạy; nút chỉ còn "làm ngay" phụ trợ | Cao |
| A5 | Bấm tìm sản phẩm ở /products | Autopilot đã có chu kỳ tự chạy qua Products UI settings (`main.go:602-703`) | Trang sản phẩm thành màn hình trạng thái autopilot (lần chạy tới, kho còn bao nhiêu); tìm kiếm tự theo cadence | Trung bình |
| A6 | Bấm refresh nhạc trending | Chart cache 6h (`internal/studio/trends.go`) | Mở Studio thấy cache quá hạn → tự refresh nền, không bắt bấm | Thấp (1 click) nhưng đúng nguyên tắc |
| A7 | Vào Settings bấm ensure/tải model (VieNeu, avatar sidecar, llama) | Đã có ensure/progress API (`handlers.go:111-126`) | Khởi động app: provider local nào đang bật mà thiếu model → tự ensure nền, báo tiến độ ở Trang chủ | Trung bình (lần đầu dùng local rất dễ kẹt ở đây) |
| A8 | Tự nhớ backup dữ liệu | Chưa có backup/restore trong app (convention Drive nằm ngoài app) | Nút + lịch tự backup DB (zip theo ngày) trong Settings, restore 1 click, giữ N bản | Trung bình — sống còn khi bán (mất DB = mất kênh) |
| A9 | Kiểm tra key chết/quota bằng tay | Có keyring test từng key (`/settings/keys/test`) | Health nền định kỳ: key 429/chết tự vào cooldown (đã có một phần ở tts keyring) + cảnh báo ở Trang chủ khi pool sắp cạn | Trung bình |
| A10 | Quyết định "kênh nào đáng đẩy tiếp" | Growth đã có kill/double-down/penalty/stalled + alert ghi lại | Đã đạt — chỉ cần đảm bảo alert hiện ở Trang chủ (hiện nằm ở /growth) để Ninh không phải đi tìm | Thấp (đã gần xong) |

Việc **không** nên tự động hoá thêm (giữ cho người): kill switch, dry-run lần đầu, bootstrap
OAuth/audit TikTok Direct Post, AdSense/thuế khi đạt monetization, chọn theme khởi điểm.
Đây là ranh giới đúng — đừng "tự động hoá" luôn cả chúng bằng cách giả lập.

---

## 5. Khoảng trống để bán được giá cao

Những gì người mua gặp trong **giờ đầu tiên**:

| # | Thiếu sót | Dẫn chứng | Ảnh hưởng bán hàng |
|---|---|---|---|
| S1 | Không có LICENSE | `ls` repo root: không file LICENSE | Không bán/chuyển nhượng sạch về pháp lý được; người mua doanh nghiệp sẽ dừng ở đây |
| S2 | Không CI, không release pipeline | Không có `.github/`; binary darwin phát hành tay (theo quy ước dự án) | Mỗi bản bán = một lần build tay dễ sai; không có bằng chứng "test xanh" tự động |
| S3 | Version đóng cứng `v0.5-go` | `cmd/aicos/main.go:49` | Sản phẩm đã qua nhiều đợt lớn mà version như đồ chơi; hỗ trợ khách sẽ rối ("bản nào?") |
| S4 | Dữ liệu demo giả trong bản thật | `team.go:127-136` (mục D1) | Người mua dùng thử thấy việc bịa → mất niềm tin ngay lập tức |
| S5 | Không chế độ demo/seed an toàn | Không có cơ chế dữ liệu mẫu có nhãn trong app thật | Người mua không có key API không đánh giá được sản phẩm; phải có "Xem thử với dữ liệu mẫu" ghi nhãn rõ, tách hẳn dữ liệu thật |
| S6 | First-run chưa thành wizard | Checklist 3 bước ở Trang chủ là khởi đầu tốt nhưng dừng ở mức nhắc | Cần luồng: tạo tài khoản → chọn theme (có gợi ý) → dán key (test ngay tại chỗ) → chạy dry-run 1 video → bật tự động. Mỗi bước tự kiểm tra và báo xanh/đỏ |
| S7 | Không đăng nhập, nhưng có cờ phơi LAN | `main.go:363` (`-addr`, mặc định 127.0.0.1 — tốt) | Bán cho khách chạy LAN/nhiều người sẽ bị hỏi bảo mật ngay; tối thiểu thêm token/mật khẩu đơn giản khi bind ngoài localhost |
| S8 | Không backup/restore trong UI | Grep `backup|restore` ở handlers: không có | Khách mất máy/mất DB là mất toàn bộ kênh; không có câu trả lời = không bán được cho người nghiêm túc |
| S9 | Tài liệu người mua còn thiếu lệnh cài trên Mac | USER_GUIDE thiếu `brew install llama.cpp` và `uv` (nợ đã ghi từ MAC_M1_CORE_AUDIT) | Local tier chết lặng lẽ; khách tưởng app lỗi |
| S10 | README kể câu chuyện lớn hơn sản phẩm hiện tại | README hứa live/persona đầy đủ; live avatar đã hạ ưu tiên theo nghiên cứu M1 Pro | Người mua đối chiếu sẽ thấy lệch; cần trang "trạng thái thật" (đã có kỷ luật này trong docs — đưa lên README) |
| S11 | Bản dài YouTube chưa render 16:9 thật | Ghi chú bc1a247 + renderer Studio chỉ 9:16 | Một trong 4 trụ doanh thu (video dài) chưa có sản phẩm thật; phải làm ở Đợt 6 hoặc bỏ khỏi lời hứa bán hàng |
| S12 | 1 project OAuth YouTube cho mọi kênh | Ghi chú bc1a247 (quota 10.000 units/project/ngày ≈ 6 upload) | Scale lên nhiều kênh sẽ nghẽn quota; người mua có 5+ kênh sẽ chạm trần ngay tuần đầu |

Điểm cộng giữ nguyên khi bán: 1 binary duy nhất không Docker, SQLite WAL, 1 dependency trực
tiếp, test 21 package xanh, kỷ luật không-bịa-số nhất quán (fail-closed khắp nơi), UI tiếng
Việt sáng kiểu macOS đã có bản preview soi được.

---

## 6. SPEC — các đợt sửa (theo thứ tự giá trị/rủi ro)

Mỗi đợt: mục tiêu · phạm vi file · thay đổi cụ thể · tiêu chí nghiệm thu · rủi ro · cỡ (S/M/L).
Thực thi theo đúng kỷ luật repo: không `git add -A`, chỉ stage path cụ thể; `docs/assets/`
untracked giữ nguyên; không chạm `vercel-preview/**` trừ đợt tái xuất riêng.

### Đợt 1 — "Sự thật & an toàn" (S–M) — làm trước mọi thứ

- **Mục tiêu:** app không bao giờ hiển thị thứ không có thật; nút khẩn cấp chắc chắn ăn.
- **Phạm vi:** `internal/web/team.go`, `internal/engines/avatar/local.go` (+ test liên quan),
  `internal/web/handlers.go` (followers), `cmd/aicos/main.go` + `internal/web/config.go`
  (hợp nhất kill/dry-run), `internal/network/daemon.go` (đọc nguồn chung).
- **Thay đổi:**
  1. Xoá fallback demo ở `/team` (D1) → empty-state (icon + 1 dòng + CTA "Mở Studio AI"),
     dùng đúng component `.empty-state` đã có.
  2. Kiểm chứng đường kill/dry-run: UI → Cfg → daemon phải là **một** nguồn sự thật
     (shared state có khoá, daemon đọc mỗi tick). Viết test: bật kill qua handler → tick
     daemon thấy dừng.
  3. `SupportsRealtime()` của local → `false` kèm comment lý do (D3); UI giữ nguyên hành vi
     (đã đúng), cập nhật test chain nếu cần.
  4. Xoá form gõ followers (D8): trang chi tiết hiển thị followers từ snapshot mới nhất,
     không có thì "chưa kết nối". Số liệu cũ trong DB giữ nguyên (không xoá dữ liệu).
- **Nghiệm thu:** `go vet` + `go test ./...` xanh (có test mới cho kill-switch xuyên tầng);
  smoke app mới tinh: /team trống hiện empty-state (không chữ "quilted"); grep không còn
  `demo-1`; chụp /team + account detail trước/sau.
- **Rủi ro:** thấp; mục 2 có thể lộ ra bug thật (đó là lý do làm sớm).
- **Cỡ:** S (mục 2 có thể phình thành M nếu phải refactor Cfg).

### Đợt 2 — "Gộp trang trùng" (M)

- **Mục tiêu:** từ 11 mục sidebar xuống 8; mỗi việc một nơi duy nhất.
- **Phạm vi:** `internal/web/handlers.go` (routes), `templates/{content,shop,analytics}.html`
  (xoá sau gộp), `templates/{studio,products,dashboard,growth}.html`, `templates/base.html`.
- **Thay đổi:**
  1. `/content` → tab "Video chữ động" trong Studio (hoặc xoá hẳn nếu xác nhận không còn
     dùng — xem Câu hỏi mở Q3); route cũ redirect 301/303 sang Studio.
  2. `/shop` → tab "Kệ hàng" trong `/products`.
  3. `/analytics`: doanh thu gộp về Trang chủ (đã có thẻ), chi phí API chuyển thành một khối
     trong Settings › Trạng thái hệ thống, số liệu theo video nằm ở `/growth`. Xoá trang.
  4. Sidebar cập nhật; mọi link nội bộ trỏ chỗ mới.
- **Nghiệm thu:** số route giảm đo được (`grep -c HandleFunc`); grep không còn href chết
  tới 3 route cũ; test xanh; chụp lại 4 trang sau gộp (light+dark); export preview lại nếu
  cần soi.
- **Rủi ro:** trung bình — job content cũ trong DB phải vẫn xem được (giữ handler đọc
  `/media/{name}` và chi tiết job; chỉ bỏ đường *tạo mới* trùng lặp nếu gộp kiểu tab).
- **Cỡ:** M.

### Đợt 3 — "Zero-touch là mặc định" (M)

- **Mục tiêu:** mở app lên (sau bootstrap) là hệ thống tự chạy; click chỉ để can thiệp.
- **Phạm vi:** `internal/web/growth_produce.go`, `internal/web/growth.go`,
  `internal/web/handlers.go` (schedule/followers đã làm ở Đợt 1), `internal/studio` (hook
  refresh trends), `cmd/aicos/main.go` (ensure model nền), templates growth/schedule/products.
- **Thay đổi:**
  1. Toggle tự động growth: mặc định BẬT khi dry-run tắt (A1); UI diễn đạt lại: dry-run là
     cổng an toàn toàn cục, không phải từng tính năng một công tắc.
  2. Lịch live: tự xây khi sang ngày mới/chưa có lịch (daemon đã làm — bỏ yêu cầu bấm;
     nút thành "Xây lại ngay" nhỏ, phụ).
  3. Plan growth tự sinh khi tạo tài khoản/đổi theme; cảnh báo growth đưa lên Trang chủ.
  4. Studio mở ra thấy trending quá 6h → tự refresh nền (A6).
  5. Khởi động: provider local đang bật mà thiếu model → tự ensure, tiến độ ở Trang chủ (A7).
  6. Trang `/products`: thêm khối trạng thái autopilot (lần chạy gần nhất/kế tiếp, số sản
     phẩm trong kho) — tìm kiếm thủ công hạ xuống phụ (A5).
- **Nghiệm thu:** kịch bản khói "không chạm tay": DB trống → tạo 1 tài khoản → (không key)
  mọi vòng tự động chạy và dừng ở trạng thái chờ trung thực, không lỗi, không bịa; có key
  test → plan sinh, job vào Studio, lịch có slot — tất cả không cần click thêm. Test cho
  các trigger tự động; test xanh toàn bộ.
- **Rủi ro:** trung bình — tự động mặc định dễ sinh job ngoài ý muốn nếu cổng dry-run hở;
  Đợt 1 (kill/dry-run một nguồn) là tiền đề bắt buộc.
- **Cỡ:** M. Phụ thuộc: Đợt 1.

### Đợt 4 — "Tách module tầng web" (L)

- **Mục tiêu:** `internal/web` từ monolith thành các file/miền độc lập; sửa tính năng A
  không chạm tính năng B.
- **Phạm vi:** `internal/web/handlers.go` (tách), `internal/web/chains.go`,
  `templates/settings.html`, thêm `internal/web/static/app.js`.
- **Thay đổi:**
  1. Tách handlers.go (1.638 dòng) thành `dashboard.go`, `accounts.go`, `schedule.go`,
     `publishers.go`, `analytics.go` (nếu còn sau Đợt 2), `settings_env.go` — không đổi
     hành vi, chỉ di chuyển + bỏ trùng lặp helper.
  2. Một renderer tham số hoá cho 3 chuỗi provider trong Settings (cắt ~100 dòng template).
  3. `app.js` dùng chung: toast, fetchJSON, poll-until-done (Studio/Settings dùng chung).
  4. Dọn 56 `style="..."` inline về class; đối chiếu CSS chết bằng render thật (D6) rồi xoá.
- **Nghiệm thu:** không file web nào > 700 dòng; test xanh không sửa test hành vi; bộ ảnh
  soi trước/sau giống hệt (light/dark, 5 trang chính).
- **Rủi ro:** trung bình (refactor thuần cơ học nhưng diện rộng) — làm sau khi hành vi đã
  khoá bởi Đợt 1–3.
- **Cỡ:** L. Phụ thuộc: Đợt 2 (để không refactor trang sắp xoá).

### Đợt 5 — "Đóng gói bán được" (M)

- **Mục tiêu:** người mua cài và dùng được trong giờ đầu, có pháp lý + bản phát hành tử tế.
- **Phạm vi:** repo root (LICENSE, `.github/workflows/`), `cmd/aicos/main.go` (version),
  `internal/web` (wizard + backup + demo mode), `docs/USER_GUIDE.md`, `README.md`.
- **Thay đổi:**
  1. LICENSE theo quyết định của Ninh (Q1) + workflow CI: build + vet + test + build bản
     darwin-arm64 gắn version từ git tag (bỏ `v0.5-go` đóng cứng).
  2. First-run wizard 5 bước (S6) thay checklist tĩnh: tài khoản → theme (có gợi ý tự động)
     → key (test tại chỗ) → dry-run thử 1 video → bật tự động.
  3. Backup/restore trong Settings: xuất zip (DB + cấu hình, không kèm secret thô — hoặc
     có cảnh báo rõ), nhập lại 1 click, tự backup theo ngày giữ 7 bản (A8).
  4. Chế độ xem thử dữ liệu mẫu có nhãn (S5), tách hẳn khỏi dữ liệu thật, tắt mặc định.
  5. Bảo vệ khi bind ngoài localhost: yêu cầu đặt mật khẩu/token ở lần đầu (S7).
  6. USER_GUIDE bổ sung `brew install llama.cpp`, `uv`; README thêm mục "trạng thái thật"
     (S9, S10).
- **Nghiệm thu:** máy sạch (hoặc data dir mới): từ tải binary → wizard → ra video dry-run
  đầu tiên < 10 phút không cần đọc docs; CI xanh trên GitHub; backup→xoá→restore khôi phục
  đúng số tài khoản/plan.
- **Rủi ro:** thấp–trung bình; mục 5 cần cẩn thận không khoá nhầm người dùng hiện tại
  (chỉ áp khi bind ngoài 127.0.0.1).
- **Cỡ:** M.

### Đợt 6 — "YouTube dài hạn thật" (L)

- **Mục tiêu:** trụ doanh thu "video dài YouTube" thành sản phẩm thật, hết nghẽn quota.
- **Phạm vi:** `internal/studio/assemble.go` (+ render 16:9), `internal/publishers/youtube.go`,
  `internal/growth/metrics.go` (Analytics API), Settings (OAuth theo kênh), `docs/CHANNEL_GROWTH.md`.
- **Thay đổi:**
  1. Đường render ngang 16:9 trong Studio (tái dùng đạo diễn/phân cảnh; khác ở assemble +
     upscale đích).
  2. OAuth project theo từng kênh YouTube (thoát trần 10.000 units/project — S12), UI gắn
     kênh↔project trong Publishers.
  3. YouTube Analytics API: lấp views-30d/giờ-xem vào KPI growth (bỏ hai ô trống hiện tại),
     vẫn fail-closed khi chưa cấp quyền.
- **Nghiệm thu:** 1 video dài 16:9 render thật từ plan; upload thử lên kênh test (unlisted)
  qua project riêng; KPI growth hiện số thật từ Analytics; quota đếm đúng theo project.
- **Rủi ro:** cao nhất trong các đợt (API/quota/OAuth) — vì vậy để sau, khi Đợt 3 đã ổn.
  Phụ thuộc: Đợt 3, và bootstrap OAuth của Ninh (việc một lần).
- **Cỡ:** L.

### Đợt 7 — "Dọn kho cuối" (S)

- **Mục tiêu:** repo gọn đúng luật "không dùng thì xoá" trước khi đóng gói bán.
- **Phạm vi:** routes `/api/*` (D2), `docs/` (D7), CSS (phần còn lại của D6), tên miền/nhãn.
- **Thay đổi:** quyết định API công khai (tài liệu + token) hoặc xoá 3 route; docs 12 → 5
  (README, USER_GUIDE, DEVELOPER, ARCHITECTURE, CHANNEL_GROWTH; RESEARCH vào thư mục con
  có index; UI_UX_* gộp một); rà nhãn tiếng Việt còn sót tiếng Anh kỹ thuật trên UI
  (vd. nhật ký quyết định ở Trang chủ dịch sang tiếng người).
- **Nghiệm thu:** grep route/link/tài liệu chết = 0; người ngoài đọc docs hiểu được hệ
  thống trong 15 phút (tự kiểm bằng cách đối chiếu mục lục).
- **Cỡ:** S. Làm cuối (sau Đợt 2/4 để không dọn thứ sắp xoá hai lần).

### Thứ tự & phụ thuộc tổng

Đợt 1 → Đợt 2 → Đợt 3 → Đợt 4 → Đợt 5 → Đợt 6 → Đợt 7 (Đợt 7 có thể kéo từng phần lên
sớm nếu cần gọn repo; Đợt 5 nên xong trước khi bán bất kỳ bản nào).

---

## 7. Câu hỏi mở cho Ninh (chỉ những quyết định thuộc về chủ sản phẩm)

- **Q1 — Mô hình bán:** bán source, bán binary có license key, hay SaaS? Quyết định này
  chi phối LICENSE (S1), cơ chế kích hoạt và mức đầu tư vào chống sao chép. (Kỹ thuật không
  tự quyết được.)
- **Q2 — Khách hàng đích:** solo creator (một người, localhost là đủ) hay team/agency
  (cần đăng nhập, nhiều người, LAN)? Chi phối S7 và độ sâu của wizard.
- **Q3 — Trang `/content` (video chữ động kinetic):** còn dùng thật không? Nếu không, Đợt 2
  xoá hẳn thay vì gộp tab — repo gọn hơn một engine.
- **Q4 — Facebook publisher:** còn trong kế hoạch không (hiện chỉ cần `FB_PAGE_ID`, đã có
  publisher fail-closed)? Giữ thì đưa vào Publishers tử tế; bỏ thì xoá khỏi lời hứa bán hàng.
- **Q5 — Video dài YouTube:** giữ trong cam kết bán (→ Đợt 6 bắt buộc) hay tạm rút khỏi
  pitch tới khi 16:9 xong? Ảnh hưởng README/pitch ngay lập tức.
- **Q6 — Tên/miền hiển thị:** giữ "AI Creator OS" cho bản bán, hay đổi tên thương mại?
  Đổi tên là việc rẻ nhất khi làm sớm (docs, title, binary).

---

## 8. Lời kết của người rà soát

Nền kỹ thuật của dự án này tốt thật — không phải lời khen xã giao: 1 dependency, 21 package
test xanh, kỷ luật không-bịa-số hiếm thấy ở các dự án AI tự chế. Thứ đang kéo giá trị bán
xuống không nằm ở engine mà nằm ở **bề mặt**: trang trùng, dữ liệu demo lẫn trong bản thật,
tự động hoá chưa bật mặc định, và thiếu vỏ đóng gói (license/bản phát hành/backup/wizard).
Sáu đợt đầu trong SPEC xử lý đúng thứ tự đó: sự thật → gọn → tự động → mô-đun hoá → đóng gói
→ mở rộng doanh thu. Làm xong Đợt 1–3 là đã có thể demo bán; xong Đợt 5 là bán được.

*— Hết báo cáo. File duy nhất được tạo trong lần rà soát này: `docs/PROJECT_REVIEW.md`
(không sửa code, không commit, theo đúng brief).*
