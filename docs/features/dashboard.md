# Trang chủ (`/`)

## 1. Mục đích

Trang chủ là lối vào tạo nội dung và quan sát nhanh: job đang xử lý, số kênh,
commission Accesstrade đã duyệt, công tắc lịch tự động, cấu hình nền tảng,
heartbeat worker, quota và video gần đây.

## 2. Kích hoạt

- Người dùng mở `/` trên trình duyệt.
- `handleDashboard` (`internal/web/dashboard.go`) render `dashboard.html`.

## 3. Dữ liệu hiển thị

`handleDashboard` đọc:

1. `s.Mgr.List()` → số hồ sơ kênh nội bộ; không khẳng định OAuth đang hoạt động.
2. `s.atOrderSummary()` → đơn đã duyệt/chờ duyệt trong Accesstrade store. Số
   tiền chỉ hiện sau lần sync thành công (`orders.until`). Chưa sync thì UI
   hiện “—”; lỗi đọc stats chỉ ảnh hưởng ô Accesstrade, các panel khác vẫn
   render và dashboard không giả lỗi thành số 0.
3. `Studio.CountActiveJobs()` → tổng job Studio ở trạng thái `queued` hoặc
   `running`, không cắt theo giới hạn danh sách gần đây. Studio chưa sẵn sàng
   thì UI hiện “—”, không hiện số 0.
4. Sáu job Studio gần nhất → trạng thái, tiến độ và file output.
   Job truyện đã dựng xong kèm marker đăng YouTube từ `story.published.<job>`;
   marker không khẳng định video đang công khai.
5. `s.autoSwitches()` → công tắc lịch tự động. Đây là cấu hình được lưu,
   không khẳng định worker đang chạy hoặc pipeline đã sẵn sàng.
6. `s.moneyReadiness()` → credentials cục bộ theo từng nền tảng. Trạng thái
   không kiểm tra API live, quyền publish, YPP, publisher approval hoặc payout.
7. `s.tickBeatViews()` + `s.quotaCircuitView()` → lần chạy tick gần nhất,
   lỗi/quá hạn và Gemini quota circuit. Callback chưa nối thì UI báo
   “chưa có dữ liệu”, không coi là bình thường.
8. Sáu quyết định gần nhất từ `decisions`, bỏ thao tác `human` và `test`; UI
   dịch agent/action, hiện target, lý do và thời điểm ghi UTC. Với agent/action
   đã có allowlist, UI hiện scalar evidence đã chọn và link bước tiếp theo;
   không render nguyên `inputs_json` vì payload có thể chứa URL/lỗi/private data.
9. Sáu cảnh báo Growth gần nhất từ `growth_alerts`, giữ mức độ, tài khoản,
   thông điệp và hành động đã ghi. Đây không phải blocker feed toàn hệ thống.
10. Sáu mục từ active Growth plans: đang sản xuất, chờ publish, lỗi hoặc quá
    ngày kế hoạch. Mục đã lên lịch trong tương lai ở trang Growth, không tính
    là blocker.
11. Sáu kết quả đăng gần nhất, gộp trạng thái mới nhất mỗi chân Studio
    Autopilot và event YouTube Growth/Story trong `decisions`; có trạng thái,
    nguồn, kênh, mã video, lý do và thời điểm UTC. Kết quả Story bắt đầu có
    event từ bản này; marker lịch sử cũ không có timestamp để dựng lại. API
    thành công không đồng nghĩa đã công khai. TikTok ghi rõ API nhận draft;
    Threads ghi bài chữ đã đăng khi có post ID, báo cần đối chiếu nếu thiếu ID,
    và không khẳng định video đã được gửi. YouTube dùng privacy
    trong event Growth/Story để phân biệt riêng tư/không công khai; event chỉ có
    cờ draft hiện “chưa công khai”. Studio leg chưa lưu draft/privacy, nên các
    nền tảng khác giữ nhãn API thành công thay vì đoán từ config hiện tại.
12. Thẻ “Cần xử lý” gộp tối đa 10 trạng thái còn mở: job Studio lỗi, chân
    Autopilot lỗi, Story có lần đăng gần nhất thất bại chưa có success marker,
    Growth đã dựng/chờ đăng, chờ kết nối/quota, lỗi/quá lịch, tick lỗi/quá hạn
    và Gemini quota đang mở. Mỗi mục liên kết tới khu vực xử lý. Alert và
    publish event lịch sử không tự được coi là blocker; lần đăng Story chỉ còn
    mở khi chưa có marker thành công trong app. Trạng thái chưa nối dữ liệu vẫn
    hiện ở panel nguồn phía dưới.
13. Thẻ “Hiệu quả video · YouTube” lấy snapshot metrics thật theo video từ
    `growth.RecentVideoMetrics`: views trong report 30 ngày, snapshot phải được
    đồng bộ trong 7 ngày gần nhất. Trung bình được tính theo số video; kênh lỗi
    đọc được báo riêng. Không có snapshot hiện là thiếu dữ liệu, không phải 0.
    Phạm vi hiện chỉ có YouTube Analytics; không đại diện hiệu quả đa nền tảng.

Thẻ commission link tới `/products#at-orders`, nơi có số đơn bị từ chối, lần
cập nhật gần nhất và thao tác đồng bộ khi đã cấu hình key.

## 4. Giới hạn UX hiện tại

- Form “Bắt đầu tạo nội dung” chỉ tạo job kể chuyện. Yêu cầu affiliate/viral
  được chuyển sang luồng tương ứng kèm lý do; media gửi nhầm qua form nhanh bị
  từ chối, không lưu và không tạo job.
- Thẻ “Cần xử lý” gộp các trạng thái hiện hành có CTA từ Studio, Story, Growth,
  Autopilot, worker và quota; hiển thị tối đa 10 mục. Lỗi đăng Story được
  reduce theo lần thử mới nhất và loại khi có success marker app. Thẻ không
  bao phủ mọi pipeline, và đăng thủ công bên ngoài app không tạo marker.
  Decision feed, cảnh báo Growth, marker Story và kết quả đăng vẫn là các feed
  riêng. Marker Story cũ chưa có timestamp để backfill; credentials chưa thay
  thế kiểm tra API live.
- Hiệu quả video chỉ có YouTube Analytics. TikTok/Meta chưa có metrics video-level
  trong thẻ này; OAuth/API chưa nối hoặc snapshot quá hạn phải hiện thiếu dữ liệu.
- CTA từ video đã QC chỉ mở trang bàn giao/tải video; không publish qua API. Video
  thiếu bằng chứng QC không có CTA bàn giao ở dashboard hoặc modal xem trước.
- “Kênh kết nối” hiện là số account nội bộ; không khẳng định mọi OAuth đã hoạt
  động.
- “Radar video xu hướng” chỉ bật/tắt tìm và lưu nguồn tham khảo; không tự dựng
  video viral. AT Hunter hiện thiếu sales/rating evidence từ feed để tạo video.
- Kill switch/dry-run áp dụng theo logic từng workflow; `MASTER_SWITCH` chỉ
  điều khiển network daemon. Một số thao tác thủ công trong UI vẫn có thể chạy.

## 5. Fail-closed & an toàn

- Trang đọc dữ liệu vận hành; công tắc lịch chỉ ghi cấu hình qua
  `POST /automation/switch`. Công tắc không chứng minh worker đang hoạt động;
  mỗi workflow có gate riêng.
- Lỗi đọc account → `s.fail` trả HTTP 500. Lỗi đọc Accesstrade order stats
  được log và chỉ hiện trạng thái lỗi tại ô commission; các panel khác vẫn
  render, không hiển thị số tiền mặc định bằng 0.
- Chưa có lần đồng bộ đơn → hiển thị trạng thái chưa có dữ liệu, không kết luận
  doanh thu bằng 0.
