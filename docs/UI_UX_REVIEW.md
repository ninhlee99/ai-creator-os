# Đánh giá UI/UX toàn app — góc người dùng khó tính (2026-10-02)

Phạm vi: toàn bộ dashboard (Trang chủ, Tài khoản, Studio AI, Agent Team, Cài đặt…), soi cả mã nguồn (CSS/template) lẫn ảnh chụp UI thật (bản preview Vercel, cùng CSS).

## Chẩn đoán tổng quan
- Khung tốt: dark admin nhất quán, không vỡ layout ở 1440px, sidebar có nhóm + trạng thái trang hiện tại, trạng thái an toàn (DRY-RUN/kill) nhìn thấy ở mọi trang.
- Nhưng nhìn **như bản dựng nội bộ**: empty state toàn câu chữ xám nghiêng trần trụi, Settings trùng lặp và dài, Studio reload trang giữa chừng, vài chỗ chữ nguyên tiếng Anh lọt vào, bảng trending có dòng text bị "lộn ngược" (font fallback), tiền hiển thị `$`.

## P0 — bug/hỏng phải sửa
1. **Trang Agent Team bản preview: avatar vỡ** — 6 agent (Hunter, Director, Producer, QC, Publisher, Analyst) và cả 6 thẻ Persona show icon ảnh vỡ + alt text trùng tên. (App thật OK — ảnh .jpg đều có; đây là bug export preview, nhưng bản preview là cái Ninh mang đi review.)
2. **Studio: badge trạng thái job thiếu CSS** — class `badge-running/queued/done/failed` chưa được định nghĩa → viên trạng thái không có màu (vẫn xám trần) dù job đang chạy/lỗi. Cùng lỗi ở badge trạng thái từng shot trong storyboard (cả 2 chỗ vẽ bằng JS).
3. **Dashboard: doanh thu hiển thị `$`** (`$0.00`) — người dùng VN nên là VNĐ/định dạng VN; kèm 5 thẻ thống kê toàn số 0 nhìn như "app trống rỗng" ngay cả khi hệ đang có dữ liệu thật từng phần.

## P1 — trúc trang & luồng công việc
4. **Empty state chưa là sản phẩm**: Trang chủ/Tài khoản/Studio đều là câu xám nghiêng không icon, không nút CTA nổi bật; phần dưới trang chết không gian đen. Làm component empty-state thống nhất: icon + 1 câu + nút hành động.
5. **Settings có HAI mục "Biến môi trường"**: bảng chỉ-đọc ở trên + bảng khóa nhập ~30 dòng ở dưới → rối. Gộp một chỗ; ẩn đường dẫn kỹ thuật (`/tmp/.../ledger.db`) khỏi thẻ trạng thái; 30 ô "Nhập giá trị, Enter để lưu" thiếu affordance lưu rõ ràng.
6. **Studio tự reload mỗi 15s** khi có job chạy → mất vị trí cuộn, khó chịu. Đổi sang AJAX cập nhật từng card job.
7. **Studio form dài một cột**, input file là control trình duyệt mặc định (Choose File) lạc tông UI; nhãn nhồi 2 ý ("Ảnh nhân vật (giữ khuôn mặt)"); emoji 🎬 trên nút chính bé xíu màu xám như đang load.
8. **Hiển thị trạng thái còn tiếng Anh gốc**: `live_ready` có gạch dưới, job/shot status còn `running/failed` thô; activity log dùng badge xám `badge-draft` cho mọi action (mất giá trị thông tin).

## P2 — màu sắc / font / chi tiết
9. **Quá nhiều đỏ/hồng chồng nhau**: hồng thương hiệu `#EF476F`, kill `#a50f2c`, lỗi `#f85149`, lỗi-team `#ef4444`, `#b91c1c`, hover `#c9305a` — người dùng khó phân biệt "hồng thương hiệu" với "đỏ nguy hiểm". Chốt 1 accent + bộ màu semantic (ok/warn/err/info) dùng thống nhất.
10. **Font**: dùng system stack (ổn, nhanh) nhưng chưa có thang type rõ ràng (h1 24/h2 18/body 15), helper text italic xám sát ngưỡng tương phản; placeholder trong input quá nhạt; chưa thấy focus-visible rõ cho bàn phím.
11. **Bảng trending**: dòng tên bài bị glyph lộn ngược (dữ liệu/font fallback) → cần sanitize hiển thị + font dự phòng tốt cho ký tự lạ.
12. Chữ lẫn Anh/Việt ở vài nút/label ("sidecar", "Voice preset", "Restart", "Media-gen").

## Điểm cộng (giữ nguyên)
- Sidebar nhóm rõ (Vận hành/Sản xuất/Hệ thống) + highlight trang hiện tại; kill switch luôn trong tầm mắt; chip DRY-RUN.
- Bảng không tràn ngang; responsive có media query; form Settings có mục lục pill bám đầu trang; keyring masked + toast đã có.
- Team tree: icon trạng thái (✓/!/+viền nhịp) + chú thích rõ — chỉ thiếu ảnh thật.

## Đề xuất lộ trình (làm theo đợt, mỗi đợt 1 commit có UI)
- **Đợt A (nửa ngày)**: sửa badge CSS job/shot; bỏ `$` → định dạng tiền VN; sanitize tiêu đề trending; sửa export preview (avatar .png→.jpg); component empty-state + nút CTA cho 3 trang trống nhất.
- **Đợt B (1 ngày)**: gộp 2 mục env trong Settings + ẩn path kỹ thuật + affordance lưu khóa rõ; Studio chuyển AJAX refresh (bỏ reload); style input file + nhãn rõ 1 ý; dịch nốt nhãn Anh→Việt + map trạng thái sang tiếng Việt.
- **Đợt C (thiết kế)**: chốt design token (1 accent, semantic colors, thang type 12/13/15/18/24, spacing 4/8/12/16/24, radius 8/14) dùng chung; focus-visible; kiểm tra tương phản ≥ 4.5:1 cho text phụ.

## Điểm tự chấm (khó tính)
- Cấu trúc/điều hướng: 7/10 · Màu sắc: 5/10 · Font/chữ: 6/10 · Luồng công việc: 5/10 · Độ tin cậy/trình bày: 5/10 → **Tổng ~5.5/10**: "chạy được, tin được, nhưng chưa đã mắt và chưa giống sản phẩm bán được".

---

# Kết quả thực hiện (2026-10-02)

## Thay đổi hướng thiết kế (theo yêu cầu Ninh)
Đợt C đổi hướng: **hệ màu cơ bản kiểu macOS, sáng là mặc định** (không còn đen toàn bộ). Nền sáng `#f5f5f7`, thẻ trắng `#ffffff`, chữ `#1d1d1f`/`#6e6e73`; chế độ tối macOS (`#1c1c1e`/`#2c2c2e`) qua nút ☀/🌙 ở topbar, lưu localStorage, theo `prefers-color-scheme` lần đầu, script chống chớp sáng trước khi vẽ. Màu accent đổi từ hồng `#EF476F` sang **xanh hệ thống macOS `#007AFF`** (tối: `#0A84FF`); semantic: xanh lá `#34C759`, cam `#FF9500` (đang chạy), đỏ `#FF3B30`, vàng `#FFCC00`. Toàn bộ qua biến CSS; chỉ vùng log/video giữ nền tối kiểu terminal ở cả hai theme.

## Trạng thái từng mục
| # | Mục | Trạng thái |
|---|-----|-----------|
| 1 | Badge job/shot Studio có màu (running cam nhấp nháy, queued xám, done xanh, failed đỏ) + audit mọi badge-* | ✅ ĐÃ LÀM |
| 2 | Tiền `$` → VNĐ (`fmtVND`: 1.234.500 ₫); USD giữ nguyên ở mục vốn là USD (quà/chi phí API) với `fmtUSD` | ✅ ĐÃ LÀM |
| 3 | Component `.empty-state` (icon tròn + tiêu đề + mô tả + nút CTA) cho dashboard/tài khoản/studio | ✅ ĐÃ LÀM |
| 3b | Checklist "Bắt đầu trong 3 bước" ở trang chủ khi chưa có tài khoản (check theo dữ liệu thật: có key? có job?) | ✅ ĐÃ LÀM |
| 4 | Form affiliate Studio: trường chính trước, nâng cao (BPM/link nhạc/điểm bắt đầu) vào `<details>`; ô chọn file có nút tối + hiện tên file | ✅ ĐÃ LÀM |
| 5 | Settings gộp 1 mục khóa/biến: tên + badge Đã đặt/Chưa + đuôi đã che (••••abcd) + ô nhập + nút Lưu từng dòng (lưu vào DB, áp dụng ngay + nạp lại khi khởi động); thẻ DB ẩn path (chỉ tooltip) | ✅ ĐÃ LÀM |
| 6 | Studio bỏ reload 15s → polling AJAX 5s cập nhật badge/tiến độ/video tại chỗ | ✅ ĐÃ LÀM |
| 7 | Dịch trạng thái sang tiếng Việt có dấu toàn app (statusLabel/statusClass): tài khoản, slot, job, asset, team, action log | ✅ ĐÃ LÀM |
| 8 | Chuẩn hóa palette 1 accent + semantic, xóa các class badge-<status> cũ | ✅ ĐÃ LÀM (đổi sang hệ macOS, xem trên) |
| 9 | Thang type 12/13/15/18/24/28, tăng tương phản chữ phụ, placeholder sáng hơn, `:focus-visible` rõ | ✅ ĐÃ LÀM |
| 10 | Sanitize tiêu đề trending (bỏ ký tự điều khiển/format, gấp chữ toán học/fullwidth về ASCII) + font fallback cho bảng | ✅ ĐÃ LÀM |
| 11 | Dịch nhãn Anh→Việt (giọng đọc/hình đại diện chạy trên máy, Voice preset→Giọng đọc…, Media-gen để trong ngoặc) | ✅ ĐÃ LÀM |
| 12 | Preview avatar .png→.jpg + xóa 16 PNG thừa | ✅ ĐÃ LÀM |
| 13 | Nhãn "realtime" avatar local gây hiểu lầm → local ghi rõ "Dựng offline (không realtime), clip 60s ≈ 6–10 phút trên M1"; badge realtime chỉ tính khi bật provider đám mây | ✅ ĐÃ LÀM |

## Kiểm chứng
- `go build ./...` OK; `go test ./...`: **19/19 package pass** (2 test kỳ vọng nhãn cũ "VieNeu sidecar"/"Avatar sidecar" đã cập nhật theo nhãn Việt mới).
- Smoke test local (binary mới, data tạm): `/`, `/accounts`, `/studio`, `/team`, `/settings` đều HTTP 200; render đúng checklist, `0 ₫`, badge realtime trung thực, không còn `location.reload`.
- **Preview Vercel**: tái xuất toàn bộ trang từ app mới + CSS theme mới, deploy production mới `dpl_3inmhMXq4dqhhUQioA1tjBzemG6J` (READY) tại https://ai-creator-os-preview.vercel.app — kiểm chứng 200 cho trang chủ/team/studio/settings/CSS và `assets/agents/hunter.jpg`.

## Còn lại (chưa làm, có chủ đích)
- Bản preview tĩnh vẫn là dữ liệu trống/mẫu (đúng bản chất preview); thao tác thật chạy ở app local.
- Điểm số >9/10 **không tự khẳng định**: cần Ninh soi trực tiếp bản sáng mới (đặc biệt tương phản ở theme sáng và mật độ trang Settings) rồi chấm lại.
