# UI_UX_BLUEPRINT.md — Thiết kế lại UI/UX: mỗi phần chỉ xử lý một vấn đề

> Chốt 2026-10-02 theo yêu cầu của Ninh: Settings/Config/Sử dụng phải hài
> hoà, **mỗi phần chỉ tập trung xử lý đúng một vấn đề**. Tài liệu này là bản
> thiết kế đích; phần "lộ trình" ghi rõ bước nào đã làm, bước nào làm tiếp.
>
> Nguyên tắc xuyên suốt: cấu hình hệ thống ở Settings; tài sản nội dung ở
> Studio/Accounts; cấu hình autopilot có đúng một nhà toàn mạng + một tab
> theo tài khoản. Không trang nào gánh hai việc.

## 1. Hiện trạng đã audit (2026-10-02, theo file thật)

| Trang hiện tại | Đang gánh mấy vấn đề | Vấn đề chính |
|---|---|---|
| `settings.html` (714 dòng) | ~8: trạng thái hệ thống, env chỉ-đọc, RTMP theo tài khoản, thư viện nhân vật AI, 3 chuỗi provider, 2 sidecar local, dry-run, kill switch | Trộn chỉ-đọc với ghi/xóa; nhân vật AI là tài sản Studio nhưng nằm ở Settings; kill switch trùng với topbar |
| `products.html` | 5: tìm sản phẩm, thêm thủ công, **lịch autopilot toàn mạng**, **nhạc autopilot toàn mạng**, kho sản phẩm | Cấu hình hệ thống bị chôn trong trang nghiệp vụ |
| `shop.html` | Thêm sản phẩm lần thứ hai (USD, hoa hồng 0–1) | Trùng `products.html` (VND + %) — 2 nơi thêm cùng một thứ, 2 đơn vị tiền |
| `account_detail.html` | ~8: persona, topic, vòng đời, followers, YouTube, autopilot theo tài khoản, ảnh mẫu, lịch sử quyết định | Autopilot bị xé 3 nơi: theo tài khoản ở đây, lịch+nhạc ở Products, RTMP ở Settings |
| `studio.html` | 4: trạng thái sẵn sàng, form affiliate 13 trường, form phim ngắn, chart trending (xuất hiện 2 lần), job list + storyboard | Một màn hình vừa tạo vừa theo dõi vừa xem chart |
| `publishers.html` | Giám sát kết nối + OAuth + runbook dài | Đọc trạng thái và làm thao tác chen nhau |
| `base.html` | Sidebar phẳng 10 mục, không nhóm, không trạng thái active | "Sản xuất video" (cũ) cạnh "Studio AI 🎬" gây tưởng là 2 hệ thống video khác nhau |

## 2. Information Architecture đích — mỗi trang trả lời đúng 1 câu hỏi

| Trang / mục | Câu hỏi duy nhất nó trả lời | Lấy nội dung từ |
|---|---|---|
| `/` Tổng quan | "Hôm nay hệ thống có khỏe không, có gì đang chặn live?" | `dashboard.html` (bỏ chi tiết doanh thu sang Phân tích) |
| `/accounts` | "Có những creator nào, đang ở trạng thái gì?" | `accounts.html` |
| `/accounts/{id}` · tab Tổng quan | "Creator này là ai — persona, niche, topic gì?" | Phần đọc của `account_detail.html` |
| `/accounts/{id}` · tab Autopilot | "Creator này đã sẵn sàng chạy tự động chưa?" (theme, hoa hồng tối thiểu, ảnh mẫu, bật/tắt, chạy ngay) | Phần autopilot + thư viện ảnh mẫu của `account_detail.html` |
| `/accounts/{id}` · tab Kết nối | "Creator này nối được những nền tảng nào?" (RTMP, TikTok token, YouTube) | Form YouTube ở `account_detail.html` + hàng RTMP ở `settings.html` + hàng ở `publishers.html` |
| `/studio` | "Tạo một video/phim mới" (2 tab: Affiliate \| Phim ngắn) | 2 form của `studio.html` |
| `/studio/jobs` | "Job đang chạy tới đâu, storyboard/kết quả ở đâu?" | Mục Job của `studio.html` |
| `/trends` | "Nhạc nào đang trending VN lúc này?" | Mục trending của `studio.html` |
| `/products` (Kho sản phẩm) | "Kho có sản phẩm nào dùng được?" (gồm cả kệ Shop, gộp làm một) | Bảng kho `products.html` + `shop.html` |
| `/products/tim-kiem` | "Tìm sản phẩm hoa hồng cao mới theo chủ đề ở đâu?" | Form tìm + thêm thủ công của `products.html` |
| `/autopilot` (Tự động toàn mạng) | "Toàn mạng chạy tự động theo lịch nào, nhạc gì, có tự đăng không?" | Lịch + nhạc từ `products.html` |
| `/settings/he-thong` | "App đang chạy chế độ gì, còn thiếu biến môi trường nào?" (chỉ đọc + dry-run) | 2 mục đầu + dry-run của `settings.html` |
| `/settings/nha-cung-cap` | "AI dùng nhà cung cấp nào trước, key nào còn sống?" (3 tab con: TTS / LLM / Avatar) | 3 chuỗi provider + keyring của `settings.html` |
| `/settings/model-local` | "Model local (VieNeu/MuseTalk) đã sẵn sàng chưa?" (tải/restart/tiến độ) | 2 mục sidecar của `settings.html` |
| `/settings/nhan-vat` | "Có những khuôn mặt AI nào, render thử ra sao?" | Mục Nhân vật AI của `settings.html` (sau sẽ cân nhắc chuyển về Studio) |
| `/settings/an-toan` | "Dừng khẩn cấp bằng cách nào, điều kiện gì mới được live?" | Kill switch + dry-run của `settings.html` (topbar giữ 1 nút khẩn cấp duy nhất) |

Quy ước đặt tên: tiêu đề trang ghi rõ phạm vi — ví dụ "Autopilot — theo tài
khoản này" (tab trong account) khác "Autopilot — toàn mạng" (trang riêng),
kèm link chéo giữa hai nơi để không ai nhầm tầng cấu hình.

## 3. Nguyên tắc hài hoà áp dụng cho mọi trang

1. **Một trang = một quyết định.** Đầu trang luôn là câu hỏi nó trả lời.
2. **Chỉ-đọc tách khỏi ghi/xóa.** Trạng thái (env, RTMP, health) không nằm
   xen giữa form thêm/xóa key.
3. **Vùng nguy hiểm nhìn là biết nguy hiểm.** Dry-run, kill switch, xóa key
   nằm trong khối viền đỏ riêng, không lẫn giữa form thường. Ô nhập key luôn
   là `type="password"`.
4. **Form dài thì chia bước có tên.** Form affiliate 13 trường → 4 nhóm:
   1·Sản phẩm, 2·Ảnh khóa mặt/khóa sản phẩm, 3·Nhạc, 4·Định dạng & thời lượng.
5. **Việc đang chạy phải nhìn thấy được.** Mọi trang có việc nền đều có khu
   trạng thái riêng (job, sidecar, autopilot) — không bắt người dùng đoán.
6. **Không hai nơi cho cùng một thao tác.** Thêm sản phẩm chỉ ở một nơi, một
   đơn vị tiền (VND + %); kill switch chỉ một nút thường trực ở topbar + một
   trang An toàn.

7. **Trực quan là chính, ít chữ tối đa (Ninh chốt 2026-10-02).** Ưu tiên
   **box/thẻ, slide/carousel, popup/modal**; càng ít chữ càng tốt, bỏ các
   đoạn giải thích chức năng dài trong UI. Trạng thái thể hiện bằng **màu +
   icon**; chi tiết dài chỉ hiện khi bấm vào (modal/tooltip); thông báo kết
   quả dùng toast góc màn hình. Nhãn nút/hành động để động từ ngắn.
   Nguyên tắc này thắng mọi thói quen "viết đoạn mô tả cho rõ" từ trước.

8. **Bảng màu cổ điển, hoài cổ (Ninh chốt 2026-10-02).** KHÔNG dùng màu
   chói (đỏ/vàng/xanh lá/cam tươi) khi không thật cần. Hướng đích:
   - Nền giấy ấm ngà, thẻ trắng ngà, chữ mực nâu-đen; dark mode dùng
     charcoal-nâu ấm thay vì đen/xám lạnh thuần.
   - Màu chính: xanh navy cổ điển trầm; điểm nhấn đồng (brass) trầm.
   - Màu trạng thái cũng dùng sắc trầm: sage (ok), ochre (chú ý), gạch
     nung (lỗi); nút Kill giữ burgundy sẫm để vẫn nhận ra khẩn cấp.
   Sắc trầm thống nhất toàn app, chỉ dùng màu trạng thái khi thật cần
   (trạng thái quan trọng/emergency), không trang trí bằng màu trạng thái.
   *Trạng thái hiện tại trước đợt màu:* theme sáng mặc định kiểu macOS
   (`#f5f5f7`/`#007AFF`, dark `#1c1c1e`/`#0A84FF`) với semantic tươi
   (xanh lá `#34C759`, cam `#FF9500`, đỏ `#FF3B30`) — đợt màu cổ điển sẽ
   thay design tokens này; các điểm tốt đã làm (thang type 12/13/15/18/24/28,
   tương phản chữ phụ, `:focus-visible`, badge trạng thái tiếng Việt,
   `.empty-state`, Studio polling AJAX, tiền VNĐ `fmtVND`) giữ nguyên và
   là chuẩn UI hiện hành.

## 4. Lộ trình thực hiện

**Đã làm trong đợt 2026-10-02 (template/CSS thuần, không đổi backend):**
- Ô nhập key của keyring đổi sang `type="password"` (không lộ key khi dán).
- Khối dry-run + kill switch trong Settings được bọc "vùng nguy hiểm" viền đỏ.
- Sidebar `base.html`: nhóm 3 cụm (Vận hành / Sản xuất / Hệ thống) + tự tô
  đậm mục đang mở.
- Trang Settings thêm thanh mục lục dính nhảy tới từng mục (hết cuộn vô tận).

**Đã làm thêm (đợt gộp trang 2026-10-02, theo PROJECT_REVIEW Đợt 2):** gộp
`shop` thành tab trong `/products`; `/content` thành tab trong Studio;
giải thể `/analytics` (doanh thu về Trang chủ, chi phí API vào Settings,
số liệu theo video ở `/growth`); sidebar 12 → 9 mục.

**Làm tiếp (cần đổi route/handler, làm theo đợt riêng có test đi kèm):**
1. Tách `/autopilot` (lịch + nhạc toàn mạng) ra khỏi `products.html`.
2. ~~Gộp `shop.html` vào `/products`~~ — đã làm ở đợt gộp trang (tab "Kệ hàng").
3. Tab hoá `account_detail.html`: Tổng quan / Autopilot / Kết nối.
4. Tách `/studio/jobs` và `/trends` ra khỏi `studio.html`; form affiliate
   chia 4 fieldset như §3.4.
5. Tách Settings thành 5 trang con theo §2 (giữ nguyên handler, chỉ chuyển
   template), chuyển Nhân vật AI về Studio — đồng thời áp dụng §3.7 (box/
   slide/modal, ít chữ) và §3.8 (bảng màu cổ điển) khi đụng vào template.

Mỗi bước trên làm riêng một commit, chạy `go test ./...` xanh mới push —
không gộp nhiều bước vào một lần sửa lớn.
