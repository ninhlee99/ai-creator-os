# REVIEW_ROUND2.md — Review vòng 2: AI Creator OS (sau Đợt 1–3)

> Ngày: 2026-10-02. Baseline: `main` @ `06c91ef` (+ sửa `style.css` của agent
> bảng màu đang chạy song song — file đó KHÔNG thuộc phạm vi vòng này).
> Phương pháp: đọc code thật toàn repo; `go build ./...` ✅, `go vet ./...` ✅
> sạch, `go test ./...` ✅ 22/22 gói `ok`, 312 hàm test; `deadcode` (golang.org/x/tools)
> trên `./...`; `go list -deps ./cmd/aicos` để xác định code nào thật sự vào binary;
> cross-build 5 đích trên chính HEAD này (xem §1). Mọi kết luận đều có `file:line`.
> Tài liệu điều phối: `docs/PROJECT_REVIEW.md` (vòng 1), `docs/MODEL.md`,
> `docs/UI_UX_BLUEPRINT.md`, `docs/CHANNEL_GROWTH.md`, GOAL của dự án.

---

## 1. Chấm điểm lại: **7.0 / 10** (vòng 1: 6.5)

**Tăng +0.5 vì bằng chứng, không vì cảm tính:**

- Kill switch đã là một nguồn sự thật duy nhất và được kiểm chứng bằng test hồi quy
  (chi tiết §2) — lỗi Blocker duy nhất của vòng 1 đã đóng thật, không vá bề mặt.
- Zero-touch đã thành mặc định có cổng dry-run/kill (Đợt 3), có test ở mức tick thật:
  `TestGrowthTickUnsetDefaultProduces`, `TestGrowthTickKillSwitchWins`,
  `TestGrowthTickDryRunBlocksProduction`, `TestGrowthTickRestartDoesNotDuplicate`
  (`internal/web/*_test.go`, 39 hàm test riêng gói web).
- Bằng chứng 1-binary vừa đo lại trên HEAD hiện tại (không dùng số cũ):
  `windows/amd64` 22,316,032 B; `darwin/arm64` 21,349,170 B; `darwin/amd64`
  22,275,584 B; `linux/amd64` 22,086,932 B; `linux/arm64` 21,014,671 B — cùng một
  nguồn, thuần Go, một dependency trực tiếp (`modernc.org/sqlite v1.60.1`), không `import "C"`.

**Bị chặn ở 7.0 vì:**

- 8/21 package (~2.976 dòng gồm test, 49 hàm test) không hề vào binary (§4, R2-01) —
  một "vũ trụ song song" gồm agents/stream/affiliate-hunter đang mục dần trong repo.
- Luồng ưu tiên số 1 (affiliate) có nứt dữ liệu: hai kho sản phẩm không nói chuyện
  với nhau (R2-02), và hai thẻ tiền trên Trang chủ không có writer nào chạm tới
  được trong app đã ship (R2-03).
- Trang Settings vẫn là bản 9-trong-1 trái chính blueprint đã chốt (R2-04);
  cấu hình vận hành vỡ 3 nơi, có knob không UI nào chạm được (R2-05).
- "YouTube dài" vẫn render khung dọc 9:16 (R2-06). Vỏ phát hành (LICENSE/CI/version/
  sao lưu) chưa tồn tại (R2-07).

**Không có Blocker.** App chạy được, an toàn (dry-run/kill), dữ liệu không giả.
Toàn bộ vấn đề còn lại là Major/Minor về kiến trúc, tính đầy đủ và vỏ sản phẩm.

---

## 2. Xác minh Đợt 1–3: đạt chuẩn dài hạn thật, không vá bề mặt

| Hạng mục vòng 1 | Kết luận | Bằng chứng |
|---|---|---|
| Kill switch hai nguồn (Blocker) | **ĐẠT** | `cmd/aicos/main.go:740` truyền chính `webCfg` làm `network.Gate`; daemon đọc lại `Gate` ở đầu mỗi tick (`internal/network/daemon.go:145-149`) thay vì chụp ảnh lúc khởi động; test `TestKillSwitchStopsDaemon`, `TestDryRunGateBlocksDaemonStart` (gói web). |
| `/team` dữ liệu mẫu giả | **ĐẠT** | Không còn seed demo: trạng thái rỗng thật (`internal/web/team.go:125-129`), test `TestTeamPageEmptyState`. (Phần còn lại của `/team` xem R2-12.) |
| Avatar local khai realtime giả | **ĐẠT** | `SupportsRealtime() = false` có chủ đích (`internal/engines/avatar/local.go:85-92`); chain chỉ báo true khi provider trả phí được bật (`chain.go:126-134`). |
| Xoá form gõ tay followers | **ĐẠT ở chi tiết tài khoản** | Followers đọc từ snapshot đã đồng bộ (`web/growth.go`, `accountGrowthView`; test `TestAccountDetailFollowersComeFromSnapshot`). Lưu ý nhỏ: danh sách `/accounts` vẫn hiện cột `accounts.followers` đã lưu — đó là số đồng bộ gần nhất, không phải ô nhập tay; chấp nhận được. |
| Gộp trang (sidebar 12→9) | **ĐẠT** | Route thật: 78 `HandleFunc` (`internal/web/handlers.go`), 11 GET trang, 4 redirect legacy `/content` `/shop` `/analytics`; sidebar đúng 9 mục (`templates/base.html`). `/analytics` đã tan (404), nội dung cũ sống ở tab/tab Trang chủ. |
| Zero-touch mặc định (Đợt 3) | **ĐẠT** | Mặc định BẬT khi chưa từng đặt, giá trị `"0"` đã lưu thắng (`web/growth_produce.go:57-60`); dry-run là cổng chặn trước `Enqueue`; kill kiểm ở đầu tick; lịch live tự xây ≤1 lần/ngày/process (`handlers.go:206,492,541-575`, `server.go:91-92`); plan tự sinh khi tạo tài khoản/đổi chủ đề; trends tự làm mới khi quá 6 giờ; model local tự ensure lúc khởi động (`cmd/aicos/main.go`). |

Nhận định chung: Đợt 1–3 sửa đúng gốc (đổi nguồn sự thật, thêm test hồi quy ở mức
hành vi), không phải sửa giao diện cho qua. Phần chưa đạt không nằm ở 3 đợt này —
nằm ở những tầng chưa từng được review kỹ, liệt kê ở §4.

---

## 3. Bảng từng URL (11 GET trang + nhóm POST/API)

Sidebar hiện tại: Trang chủ · Tài khoản · Lịch live · Phát triển kênh | Studio AI ·
Agent Team · Sản phẩm | Đa nền tảng · Cài đặt.

| URL | Verdict | Lý do + việc cụ thể |
|---|---|---|
| `GET /` Trang chủ | **Giữ, nâng cấp nhẹ** | Đúng vai trò tổng quan. Cần: (1) thêm chip trạng thái daemon (Master/dry-run/kill) để "vì sao máy không chạy" nhìn thấy ngay (R2-08); (2) hai thẻ "Doanh thu affiliate"/"Hoa hồng" phải ghi rõ nguồn số trong tooltip (R2-03); (3) feed quyết định đang hiện tên agent thô (`{{.Agent}}` kiểu `growth_director`) → nhãn tiếng Việt + icon (R2-11). |
| `GET /accounts` | **Giữ** | Danh sách sạch. Cột followers là số đồng bộ gần nhất — thêm mốc thời gian đồng bộ thay vì để người dùng đoán độ tươi. |
| `GET /accounts/new` | **Giữ** | Đây là bootstrap một lần, thao tác tay được chấp nhận. Persona đã được gợi ý tự động. |
| `GET /accounts/{id}` | **Làm lại (tab hoá)** | Một trang gánh ~9 mối quan tâm (tổng quan, tăng trưởng, autopilot, kết nối, điều khiển live, chủ đề, lịch sử) với ~10 form POST; blueprint đã chốt tách tab Tổng quan/Autopilot/Kết nối. Lỗi ở các POST (transition/youtube/replan/live-topic) chỉ `log.Printf` rồi redirect như thành công (`handlers.go:427,447,470,482`) → phải trả `?err=` + toast (R2-13). Nút chuyển trạng thái đang hiện slug tiếng Anh thô `→ {{.}}` (R2-11). |
| `GET /schedule` Lịch live | **Giữ, nâng cấp nhẹ** | Sau Đợt 3 đã tự xây lịch. Còn hiện `{{.AccountID}}` thô thay vì username như các trang khác đã làm — sửa cho nhất quán (R2-11). |
| `GET /studio` | **Nâng cấp** | Form affiliate 13 trường một cột → 4 nhóm fieldset như blueprint §3.4; tách `/studio/jobs` và `/studio/trends` (blueprint đã vạch, chưa làm) để `/team` và nơi khác liên kết sâu được. JS nội tuyến đang tự định nghĩa lại nhãn trạng thái job trùng logic server (`JOB_LABELS/JOB_CLASSES` trong `studio.html`) → chuyển về một nguồn (R2-04). |
| `GET /team` | **Giữ, sửa nhãn trung thực** | Cây trạng thái suy ra từ tiến độ job Studio (`web/team.go:38-60` ánh xạ % → giai đoạn), KHÔNG phải hệ Agent Team v2 trong `docs/AGENT_TEAM.md` (TeamInstance/blackboard/QC 3 lớp — grep toàn repo: 0 dòng code). Trang không bịa dữ liệu nữa, nhưng cần một dòng nhãn "theo tiến độ job Studio" để không hàm ý agent thật đang chạy (R2-12). |
| `GET /growth` Phát triển kênh | **Giữ, nâng cấp nhẹ** | Trang tốt nhất về trung thực số (fail-closed, ghi rõ thiếu Analytics API). Nhưng zero-touch đã là mặc định mà nút "Sinh plan 30 ngày"/"Đồng bộ + chạy vòng quyết định" vẫn là nút chính từng hàng → hạ xuống hộp "Tác vụ thủ công" (R2-11). Các ngưỡng kill/double-down đang hardcode, không xem/sửa được ở đây (R2-05). |
| `GET /products` Sản phẩm | **Làm lại phần dữ liệu** | Hai tab đang đọc/ghi HAI kho khác nhau: tab tìm kiếm + thêm tay + autopilot dùng `products.Store` (products.db); tab "Kệ hàng" dùng bảng `products` trong ledger.db (`Ledger.ShelfProducts/UpsertProduct`). Thêm ở Kệ hàng thì autopilot không bao giờ thấy (R2-02). Gộp về một kho, UI giữ nguyên hai tab. |
| `GET /publishers` Đa nền tảng | **Giữ** | Đúng vai trò cổng kết nối. Runbook trong trang còn trỏ tới `docs/POSTPROD_RUNBOOK.md` đã bị xoá ở đợt dọn tài liệu (R2-10). Redirect URI TikTok đang cố định `127.0.0.1:8080` trong khi `-addr` đổi được cổng → đổi cổng là OAuth hỏng khó hiểu (R2-14). |
| `GET /settings` | **Làm lại (tách 5 trang con)** | 760 dòng một trang, 9 mục neo cuộn, ~370 dòng `<script>` nội tuyến, markup thẻ provider và keyring lặp nguyên văn 2 lần, toast chỉ tồn tại ở trang này. Blueprint §2/§4 đã chốt 5 trang con (`/settings/he-thong`, `nha-cung-cap`, `model-local`, `nhan-vat`, trang phục thuộc) — chưa thực hiện (R2-04). |
| Redirect `/content` `/shop` `/analytics` | **Giữ** | Tương thích ngược đúng mực; giữ tới khi preview ngoài không còn link cũ. |
| Nhóm POST `/accounts/{id}/*`, `/growth/*`, `/products/*`, `/settings/*` | **Giữ route, sửa kỷ luật lỗi** | Route không thừa; vấn đề là nuốt lỗi lặng (R2-13) và một POST ghi nhầm kho (`/products/shelf/add`, R2-02). |
| `GET /api/stats` `/api/products` `/api/decisions` | **Hạ mặc định TẮT** | Đăng ký ở `handlers.go:128-130`, không trang UI nào gọi, không khoá: ai mở được địa chỉ bind là đọc được chi tiêu/sản phẩm/lý do quyết định. Mặc định: sau cờ cài đặt `api.enabled=false` (404 khi tắt); `/api/products` phải đọc kho đã gộp ở R2-02 hoặc xoá. |

---

## 4. Findings mới (R2-xx)

### R2-01 — Major — 8 package không vào binary: một vũ trụ code song song đang mục

`go list -deps ./cmd/aicos` KHÔNG chứa: `internal/affiliatehunter`,
`internal/agents/{analyst,config,content,governance,hunter,streamer}`,
`internal/stream` — cộng ~2.976 dòng (kèm test) và 49 hàm test xanh mà sản phẩm
không bao giờ chạy. Hệ quả cụ thể:

- `network.Daemon` có sẵn hook `OnStartLive/OnStopLive/OnRunAgent`
  (`internal/network/daemon.go:101-108`) nhưng `cmd/aicos/main.go` không gán bao
  giờ → daemon không thể bắt đầu live hay chạy agent kể cả khi bật Master.
  `ARCHITECTURE §12.1` mục 3 tự ghi nhận "cố ý chưa nối mù" — tức là biết mà để.
- Vũ trụ này mang theo bộ cấu hình THỨ HAI: `internal/agents/config` định nghĩa lại
  `Config` gồm cả `KillSwitch/DryRun/Telegram` riêng (`config.go:53-54,128-129`),
  và ngưỡng governance (`KillViewsNoOrder`, `KillSessionsNoOrder`) không cai quản
  thứ gì đang chạy. Ai lỡ nối nhầm vào đây sẽ tạo nguồn sự thật thứ hai cho kill
  switch — đúng loại lỗi Đợt 1 vừa sửa.
- Chi phí bảo trì thật: 49 hàm test giữ cho code chết luôn xanh, che mắt review.

Hướng sửa: quyết định park/xoá/nối theo từng package (SPEC R2-W6, câu hỏi Q-B).
Dù chọn gì, phải xoá `internal/agents/config` (gộp về một nguồn cấu hình) và thêm
kiểm tra reachability vào CI để package mới không lặng lẽ thành "không ai dùng".

### R2-02 — Major — Sản phẩm vỡ hai kho: thêm ở "Kệ hàng" thì autopilot mù

- Kho A: `internal/products` (`products.Store`, file products.db) — tab tìm kiếm,
  form thêm tay, và `studio.Autopilot` (`TopByTheme`) đọc ở đây.
- Kho B: bảng `products` trong ledger.db (schema ở ledger) — tab "Kệ hàng" đọc qua
  `Ledger.ShelfProducts`, ghi qua `Ledger.UpsertProduct`; `GET /api/products` cũng
  đọc kho B (`ledger.go:383`, `GetProducts`).

Người dùng thêm sản phẩm ở tab Kệ hàng (đúng cái tên mời gọi nhất) thì luồng ưu
tiên số 1 của dự án — affiliate autopilot — không thấy sản phẩm đó, và không có
thông báo nào cho biết. Đây là nứt dữ liệu ở luồng chính, phải gộp về một kho
duy nhất (SPEC R2-W1); giữ nguyên UI hai tab.

### R2-03 — Major — Hai thẻ tiền trên Trang chủ không có writer trong app đã ship

- "Doanh thu affiliate" = `SUM(orders.commission)` (`internal/web/handlers.go:203`).
- "Hoa hồng" = `SUM(commissions.amount)` (`internal/ledger/ledger.go:892-897`).

Writer duy nhất ghi `commissions` là `analyst.ReconcileOrders`
(`internal/agents/analyst/analyst.go:47`) — thuộc vũ trụ không vào binary (R2-01).
Trong app thật, cả hai thẻ đứng ở 0 vĩnh viễn. Số 0 này trung thực (không bịa),
nhưng tính năng đối soát tiền coi như chưa tồn tại. Hướng sửa: đưa reconcile vào
luồng đồng bộ đang chạy thật (growth sync tick) với payload từ API nhà cung cấp,
giữ fail-closed (SPEC R2-W1); quà tặng (gifts) tương tự — chỉ ghi khi API có số.

### R2-04 — Major — Tầng web vẫn là monolith; Settings trái blueprint đã chốt

- `internal/web` tổng 7.565 dòng Go; `handlers.go` 1.588 dòng; `autopilot.go` 729
  dòng nhưng chứa cả autopilot sản phẩm lẫn autopilot tài khoản (tên file nói dối
  phạm vi); `growth_produce.go` 575; `server.go` 543; `growth.go` 501.
- `settings.html` 760 dòng, một trang 9 mục; ~370 dòng JS nội tuyến; markup thẻ
  provider + keyring bị lặp; toast chỉ có ở trang này — mọi trang khác không có
  cơ chế thông báo thống nhất. `static/` chỉ có `style.css` + ảnh agent: chưa có
  `app.js` dùng chung. 58 thuộc tính `style="..."` nội tuyến rải khắp template.
- Việc tách Settings thành 5 trang con và tách `/studio/jobs`, `/trends` đã được
  chính `UI_UX_BLUEPRINT.md` chốt từ vòng 1 — Đợt 2 mới gộp trang, chưa đụng cấu
  trúc bên trong trang.

Hướng sửa: SPEC R2-W2 (tách file theo miền, không file >700 dòng, route giữ nguyên
hệt) + R2-W3 (tách trang con, partials dùng chung, `app.js` + toast toàn app).

### R2-05 — Major — Cấu hình vỡ 3 nơi; knob vận hành không UI; `Cfg` đọc cũ

Ba "vũ trụ" cài đặt đang sống song song: `settings` trong ledger.db (chains, khoá
`env:`, trạng thái growth), `settings` trong products.db (lịch/nhạc/tự đăng của
autopilot), và `web.Config` đọc từ env một lần lúc khởi động. Hệ quả đo được:

- Lưu biến env qua UI cập nhật tiến trình + DB nhưng KHÔNG cập nhật các trường đã
  đọc vào `web.Config` → các đường kiểm tra như `ValidateForLive` đọc giá trị cũ
  cho tới khi restart (chính code tự ghi nhận giới hạn này).
- Các knob vận hành sau KHÔNG có UI nào (trái luật dự án "mọi năng lực vận hành
  phải có UI"): ngân sách API/ngày 5 USD, giới hạn phút live/phiên 120, ngưỡng
  kill theo view/phiên (`KillViewsNoOrder=10000`, `KillSessionsNoOrder=3`), rating
  sàn người bán 4.0, giá trần, và toàn bộ ngưỡng của growth engine
  (`internal/growth/growth.go:76`: `KillMinVideos=8`, `KillMinDays=14`,
  `KillCompletionFloor=0.35`, hệ số double-down/breakout, `PenaltyViewsDrop`,
  `StalledDays`) — vòng đồng bộ đang gọi thẳng `growth.DefaultConfig()` hardcode.
- Hằng số vận hành khác cũng nằm chết trong code: tối đa 3 job/tick, item quá hạn
  14 ngày, cửa sổ chống trùng 45 ngày (`internal/web/growth_produce.go`).

Hướng sửa: SPEC R2-W4 — một mặt tiền Settings (interface) ghi ledger.db, `Cfg`
đọc qua mặt tiền hoặc refresh khi lưu; đưa ngân sách + ngưỡng growth lên UI
(trang `/growth` hiện giá trị đang dùng, sửa được, có nút về mặc định).

### R2-06 — Major — "YouTube dài" vẫn là video dọc 9:16

`growth` sinh biến thể dài 150 giây (`internal/growth/production.go:32-41`) và gọi
nó là "YouTube dài", nhưng tầng dựng chỉ có một khổ: `internal/studio/assemble.go:15-16`
cố định 1080×1920; prompt đạo diễn hardcode "vertical 9:16" ở nhiều nơi
(`studio.go:388,557,566,715,739,751`; `director.go:133,180`). Tài liệu đã ghi
trung thực giới hạn này, nhưng sản phẩm vẫn lên lịch và đăng bản "dài" dọc lên
YouTube. Với mục tiêu kênh-dài-là-tài-sản, đây là thiếu sót chức năng thật, không
phải lỗi nhãn. Hướng sửa: SPEC R2-W5 — tham số khổ hình xuyên suốt job (đạo diễn →
dựng), acceptance bằng ffprobe 1920×1080.

### R2-07 — Major — Vỏ phát hành 1-binary chưa tồn tại

Đã chứng minh biên dịch được 5 đích (§1), nhưng "mở là chạy trên máy trắng" còn
thiếu, toàn bộ đo được:

- Không `LICENSE`, không `.github` (không CI), version vẫn hardcode `v0.5-go`
  (`cmd/aicos/main.go:49`) — không có khái niệm đường nâng cấp.
- Không có sao lưu/khôi phục trong UI, trong khi trạng thái nằm rải: 3 file DB
  (ledger/studio/products) + `content_jobs.json` + file token OAuth nằm ở **thư
  mục làm việc hiện tại (CWD)** chứ không nằm trong thư mục dữ liệu
  (`internal/publishers/tiktok.go:51`, `youtube.go:106`) — chạy binary từ thư mục
  khác là "mất kết nối" bí ẩn, sao lưu thư mục dữ liệu cũng không đủ.
- Schema tạo bằng `CREATE TABLE IF NOT EXISTS` rải + 3 câu `ALTER` nội tuyến
  (`internal/ledger/ledger.go:35-37`), không `user_version` — chưa có kỷ luật
  nâng cấp schema cho các bản sau.
- `ffmpeg` lấy từ PATH, thiếu là job chết với lỗi thô (`internal/web/jobs.go:282`),
  không có trạng thái "chưa thấy ffmpeg" trong UI Settings.

Hướng sửa: SPEC R2-W7.

### R2-08 — Major — MASTER_SWITCH chỉ bật được bằng env, và tắt một cách vô hình

Daemon network chỉ chạy khi `MASTER_SWITCH` (env) bật (`cmd/aicos/main.go:727`,
mặc định false) và tự thoát lặng ở đầu tick (`daemon.go:149`). Không toggle UI,
không chỉ báo trên Trang chủ. Người dùng mở app, thấy mọi trang sống, nhưng tầng
live/agent không bao giờ chạy và không chỗ nào nói vì sao. Kể cả khi mảng live
đang hạ ưu tiên, trạng thái này phải NHÌN THẤY được: chip trên Trang chủ + toggle
ở Settings (lưu bền, mặc định TẮT kèm nhãn trung thực "chưa nối dây chuyền live"
cho tới khi R2-W6 quyết). Gộp trong SPEC R2-W4.

### R2-09 — Minor — API `/api/*` mở, không khoá, không người gọi

Xem bảng §3. Rủi ro thấp khi bind 127.0.0.1 mặc định, nhưng app cho phép bind LAN
(`-addr`) — lúc đó chi tiêu + lý do quyết định bị lộ cho cả mạng nội bộ. Hạ mặc
định tắt sau cờ cài đặt (SPEC R2-W7), không cần thêm hệ auth.

### R2-10 — Minor — Dead code trong cây đang chạy thật (19+ biểu tượng)

`deadcode` trên cây reachable (ngoài phần thuộc R2-01) cho, tiêu biểu: toàn bộ
factory phim `internal/engines/film.go` (`PlanFilm:60`, `SynthNarration:185`,
`srtTime:232`, `BuildSRT:244`, `AssembleFilm:259`, `MakeFilm:287`),
`engines/llm.go:319 NewGeminiProvider`, `engines/tts/gemini.go:55`,
4 hàm `engines/avatar/emotion.go:54,67,94,115`, `avatar/paid.go:125 maskedKeys`,
`avatar/wiring.go:102 MarshalAvatarChain`, `growth/engine.go:233 CadencePerDay`,
`network/daemon.go:123 Daemon.State`, `network/onboarding.go:81`,
`network/scheduler.go:162 SlotsForDashboard`, `studio/assemble.go:245 ExtractFrame`
+ `:270 ProbeDuration`, `web/growth.go:404 growthNum`. Xoá hoặc nối dây, và đưa
`deadcode` vào CI (SPEC R2-W2/W7) để không tái phát.

### R2-11 — Minor — Vi phạm nguyên tắc ít-chữ ở chi tiết

- Feed quyết định hiện tên agent thô (`{{.Agent}}`), nút chuyển trạng thái tài
  khoản hiện slug `→ {{.}}`, `/schedule` hiện `AccountID` thô.
- `/growth` vẫn để thao tác tay làm nút chính dù zero-touch là mặc định.
- `publishers.html` trỏ runbook đã xoá; `handlers.go` còn comment trỏ
  `docs/MAC_M1_CORE_AUDIT.md` đã gộp; `docs/USER_GUIDE.md:138` hướng dẫn
  `kill -USR1` trong khi `main.go:772` chỉ bắt SIGINT/SIGTERM — làm theo hướng
  dẫn là không có tác dụng như mô tả. (R2-10b: dọn tham chiếu tài liệu chết.)

Sửa trong SPEC R2-W3 (nhãn tiếng Việt + icon, hộp "Tác vụ thủ công") và R2-W7
(SIGUSR1 hoặc sửa hướng dẫn — sửa handler rẻ hơn).

### R2-12 — Minor — `/team` và `AGENT_TEAM.md` đang kể hai câu chuyện khác nhau

Thiết kế v2 (TeamInstance, blackboard, QC chấm mù 3 lớp, ngân sách cứng) có 0 dòng
code; trang `/team` là phép chiếu tiến độ job Studio lên cây tĩnh. Doc §8 tự ghi
"ánh xạ trung thực" nên đây không phải nói dối tài liệu — nhưng người xem trang
sẽ hiểu agent thật đang chạy. Quyết định mặc định: giữ trang, thêm nhãn nguồn
("suy ra từ tiến độ job Studio"), KHÔNG đầu tư hiện thực hoá v2 cho tới khi mảng
live được mở lại (gắn với Q-B).

### R2-13 — Minor — Nuốt lỗi lặng ở tầng handler

141 điểm `_ =` toàn repo (90 điểm trong `web` + `growth`). Nhiều chỗ chấp nhận
được (ghi log phụ), nhưng các POST tài khoản chỉ `log.Printf` rồi redirect như
thành công (`handlers.go:427,447,470,482`) khiến người dùng không phân biệt được
"đã lưu" với "lưu hỏng". Chuẩn hoá: lỗi thao tác người dùng → redirect kèm `?err=`
+ toast (trang chi tiết đã có sẵn cơ chế `Error/Notice`); lỗi nền → log (SPEC R2-W4).

### R2-14 — Minor — Điểm nghẽn scale và chi tiết vận hành

- `growthView()` truy vấn theo từng tài khoản trên mỗi lần tải trang /growth —
  N+1 sẽ chậm tuyến tính khi lên hàng chục kênh; chưa phải lỗi hôm nay, ghi sổ.
- Một project OAuth YouTube dùng chung toàn mạng (quota 1600/10.000 units đã canh
  trong code) — quyết định mặc định: giữ cho tới khi quota thật sự chạm trần mới
  tách theo kênh (SPEC không mở việc này).
- Nguồn số TikTok cho growth vẫn là stub trung thực `ErrNotConnected`
  (`internal/growth/metrics.go:160-166`) — đúng kỷ luật fail-closed, chờ OAuth/audit.
- TikTok redirect URI cố định cổng 8080 (xem §3, `/publishers`).

### R2-15 — Minor — Số liệu trong README đã cũ

`README.md:89` ghi "250 test function / 20 package (2026-10-02)"; đo thật hôm nay:
312 hàm test, 22 gói `ok`. Cập nhật kèm một dòng lệnh kiểm chứng trong DEVELOPER
để lần sau không lệch (SPEC R2-W7). Thư mục `vercel-preview/` (bản tĩnh theo dõi
trong git) sẽ phải xuất lại sau mỗi đợt UI — ghi thành bước nghiệm thu của R2-W3
thay vì để lệch âm thầm.

---

## 5. SPEC các đợt tiếp theo (thay thế Đợt 4–7 cũ)

Thứ tự: **R2-W1 → R2-W2 → R2-W3 → R2-W4 → R2-W5 → R2-W6 → R2-W7**.
(R2-W5 độc lập, có thể đổi chỗ với R2-W4. Đợt đổi bảng màu CSS đang chạy song song
phải commit trước R2-W3.) Mỗi đợt: xong → `go build ./... && go test ./...` xanh →
commit riêng bằng đường dẫn tường minh (không `git add -A`) → push → đối chiếu
`HEAD == origin/main`. Không đợt nào được làm yếu cổng dry-run/kill-switch; các
test hồi quy nêu tên trong §2 phải xanh sau mọi đợt.

### R2-W1 — Một kho sản phẩm + writer tiền thật (đóng R2-02, R2-03)

- Phạm vi: `internal/products`, `internal/ledger` (bỏ đường ghi shelf riêng),
  `internal/web` (tab Kệ hàng, autopilot), vòng đồng bộ growth.
- Việc: chọn `products.Store` làm kho duy nhất; trường shelf (trạng thái/điểm) chuyển
  sang kho này; `/products/shelf/add` ghi kho chung; `/api/products` đọc kho chung.
  Đưa reconcile hoa hồng vào tick đồng bộ đang chạy (payload API thật, fail-closed,
  không số → giữ 0 và ghi lý do vào quyết định).
- Nghiệm thu đo được: test tích hợp "thêm qua Kệ hàng → `TopByTheme` của autopilot
  thấy sản phẩm"; test reconcile với payload giả lập → `commissions` có dòng và thẻ
  Trang chủ đổi số; toàn bộ suite xanh.

### R2-W2 — Tách monolith web + dọn dead code (đóng R2-04 phần code, R2-10)

- Phạm vi: `internal/web/*.go` (không đụng template, không đổi route/hành vi).
- Việc: `handlers.go` tách theo miền (dashboard/accounts/settings/trang còn lại),
  không file nào >700 dòng; đổi tên/tách `autopilot.go` theo đúng phạm vi; xoá các
  biểu tượng chết ở R2-10; thêm `PRAGMA user_version` đặt nền (chưa migrate gì).
- Nghiệm thu: bảng route trước/sau giống hệt (xuất danh sách route ra file, `diff`
  rỗng); `deadcode` trên cây reachable sạch; build/test xanh; test render mọi trang
  (`TestAllPagesRender`) xanh.

### R2-W3 — Cấu trúc UI theo blueprint (đóng R2-04 phần UI, R2-11, R2-12)

- Phạm vi: `internal/web/templates`, `internal/web/static` (thêm `app.js`),
  handler settings/account/studio.
- Việc: Settings 5 trang con; chi tiết tài khoản 3 tab, thao tác nguy hiểm vào
  modal xác nhận; `/studio/jobs` + `/studio/trends`; partials thẻ-provider/keyring
  dùng chung; toast toàn app qua `app.js`; nhãn tiếng Việt cho agent/trạng thái;
  `/schedule` hiện username; nút tay ở `/growth` vào hộp "Tác vụ thủ công"; nhãn
  nguồn dữ liệu ở `/team`.
- Nghiệm thu: `TestAllPagesRender` mở rộng cho route mới; xuất lại `vercel-preview/`
  và commit riêng; kiểm mắt 9 trang sidebar không đổi, không trang nào thêm tường chữ.

### R2-W4 — Automation ra khỏi web + một nguồn cấu hình (đóng R2-05, R2-08, R2-13)

- Phạm vi: mới `internal/automation` (tick growth, scheduler autopilot, auto-publish),
  `cmd/aicos/main.go` (gầy đi), `internal/web` (handler gọi service), settings.
- Việc: dời logic tick khỏi `web.Server`; mặt tiền Settings duy nhất ghi ledger.db;
  lưu env qua UI thì refresh `Cfg`; MASTER_SWITCH thành cài đặt bền + toggle
  Settings + chip Trang chủ (mặc định TẮT, nhãn trung thực); ngưỡng growth + ngân
  sách API lên UI; chuẩn hoá lỗi POST → `?err=` + toast.
- Nghiệm thu: các test tick/kill/dry-run chạy qua service mới vẫn xanh; test "đổi
  MASTER_SWITCH → restart → giữ nguyên"; `cmd/aicos/main.go` giảm rõ rệt; build/test xanh.

### R2-W5 — YouTube dài 16:9 thật (đóng R2-06)

- Phạm vi: `internal/studio` (job thêm khổ hình), `internal/growth/production.go`,
  prompt đạo diễn.
- Việc: tham số khổ 9:16/16:9 xuyên job → assemble; biến thể YouTube dài dùng 16:9;
  Shorts/TikTok giữ 9:16.
- Nghiệm thu: test ffprobe đầu ra 1920×1080 cho job dài; test 9:16 giữ nguyên;
  nhãn trên `/growth` khớp khổ thật.

### R2-W6 — Quyết định vùng chưa nối dây (đóng R2-01, R2-12 phần code)

- Điều kiện: có đáp án Q-B (hoặc hết hạn chờ → dùng mặc định ở §6).
- Việc theo lựa chọn: park bằng build tag `parked` (giữ test chạy bằng
  `go test -tags parked ./...`), hoặc xoá khỏi `main` (giữ tag git), hoặc nối thật
  vào daemon qua cổng dry-run/cadence. Mọi trường hợp: xoá `internal/agents/config`,
  cập nhật `ARCHITECTURE §12` + bảng trạng thái README, thêm kiểm tra reachability CI.
- Nghiệm thu: `go list -deps ./cmd/aicos` được ghi vào doc kèm giải thích; CI fail
  nếu xuất hiện package mới không ai import; build/test (cả hai chế độ tag) xanh.

### R2-W7 — Vỏ phát hành 1-binary (đóng R2-07, R2-09, R2-14 một phần, R2-15)

- Phạm vi: `cmd/aicos` (version ldflags, SIGUSR1, probe), `.github/workflows`,
  Settings > Hệ thống (version/thư mục dữ liệu/sao lưu-khôi phục/trạng thái ffmpeg),
  `internal/publishers` (token về thư mục dữ liệu + di trú tự động), tài liệu.
- Việc: LICENSE theo Q-A; CI build+vet+test+cross-build 5 đích; sao lưu zip (3 DB +
  token + jobs) và khôi phục có kiểm chứng; `/api/*` sau cờ mặc định tắt; sửa số
  test trong README + ghi lệnh kiểm chứng; gắn `user_version` cho schema.
- Nghiệm thu: CI xanh trên GitHub; test vòng sao lưu → xoá → khôi phục; chạy binary
  trong thư mục trắng → app lên, wizard lần đầu hiện, token nằm trong thư mục dữ liệu.

---

## 6. Câu hỏi mở & quyết định mặc định

**Thật sự cần Ninh quyết (chặn kỹ thuật):**

- **Q-A — LICENSE loại nào?** Chặn duy nhất file LICENSE ở R2-W7. Chất lượng "bán
  được" không ép phải bán, nhưng phát hành thiếu LICENSE mặc định là "giữ mọi
  quyền" theo luật — nếu muốn người mua dùng hợp lệ thì phải ghi rõ. Các đợt khác
  không chờ câu này.
- **Q-B — Số phận 8 package chưa nối dây (agents/stream/affiliate-hunter)?**
  Chặn R2-W6. Ba lựa chọn: (i) park trong repo bằng build tag `parked`;
  (ii) xoá khỏi `main`, giữ trong git history + tag; (iii) nối thật vào daemon
  (trái ưu tiên hiện tại: live đang hạ). **Mặc định nếu không có đáp án khi tới
  R2-W6: chọn (i)** — giữ được 49 test và đường quay lại, binary vẫn sạch, repo
  không nói dối nữa.

**Hệ thống tự quyết theo nguyên tắc đã chốt (không hỏi lại):**

- `/api/*`: mặc định TẮT sau cờ cài đặt (không UI nào dùng → không mở).
- MASTER_SWITCH: thành cài đặt bền, mặc định TẮT + nhãn trung thực, tới khi R2-W6
  quyết nối live; tự động hoá affiliate/video (mảng ưu tiên) không phụ thuộc nó.
- Demo mode/dữ liệu mẫu trong app thật: KHÔNG làm (trái luật không số giả).
- YouTube OAuth tách theo kênh: hoãn tới khi quota dùng chung chạm trần thật.
- Facebook/trang Đa nền tảng: giữ nguyên trạng, không đầu tư thêm cho tới khi
  Ninh chủ động nâng cấp (đúng chỉ đạo cũ của Ninh).
- Wizard lần đầu: làm dạng chuỗi modal trên Trang chủ (không thêm route).

---

## 7. Anti-scope — những gì KHÔNG làm

- Không SaaS, không đa người dùng, không hệ tài khoản/phân quyền; bảo mật dừng ở
  bind localhost mặc định + cờ API (R2-09). Cốt lõi vẫn là 1 binary 1 người dùng.
- Không khôi phục đường TikTok Shop API chính thức (Ninh đã loại: "rất phức tạp").
- Không đầu tư live avatar realtime local (đã kết luận không khả thi trên M1 Pro,
  `docs/RESEARCH.md` §6), không hiện thực hoá Agent Team v2 cho tới khi mảng live
  được mở lại bằng quyết định mới.
- Không biến Studio thành trình dựng video tổng quát: chỉ thêm khổ 16:9 cho YouTube
  dài (R2-W5), không làm timeline editor.
- Không thêm trang ngoài blueprint; không đổi stack (giữ Go + template nhúng +
  SQLite WAL); không framework migration nặng — chỉ `user_version` tối thiểu.
- Không viết lại growth engine (đang đúng chuẩn: fail-closed, có test, số trung thực);
  chỉ đưa ngưỡng của nó lên UI (R2-W4).
- Không bịa số cho các khoảng trống đã biết (TikTok metrics, YouTube watch-hours):
  giữ stub trung thực cho tới khi có OAuth/API thật — đó là bootstrap một lần của
  Ninh, không phải việc của các đợt trên.
