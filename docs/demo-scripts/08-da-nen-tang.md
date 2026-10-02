# Video 8 — Đa nền tảng (`/publishers`)

**Mục tiêu:** bảng trạng thái kết nối thật từng nền tảng của từng kênh; nối TikTok OAuth (mô phỏng).

## Chuẩn bị

- Đã có 1 kênh. Không cần key thật — demo ở mức badge "thiếu" là đúng.

## Kịch bản

**Bước 1 — Bảng trạng thái.**
[Thao tác: mở Đa nền tảng.]
"Mỗi dòng là một kênh, mỗi cột là một nền tảng: TikTok, YouTube, Facebook, RTMP. Ô nào đã nối thì xanh, thiếu thì ghi 'thiếu' — app đọc trạng thái thật, không đoán."

**Bước 2 — Ý nghĩa từng cột.**
[Thao tác: chỉ từng cột.]
"TikTok cần token OAuth và client key. YouTube cần file token và client ID. Facebook chỉ cần nhập Page ID — đăng qua tài khoản đã liên kết, không cần tạo Facebook App."

**Bước 3 — Nối TikTok (mô phỏng tới bước redirect).**
[Thao tác: bấm "Kết nối TikTok", dừng ở màn hình redirect sang TikTok.]
"Tôi bấm kết nối — app tạo mã chống giả mạo có hạn 15 phút rồi chuyển sang TikTok để bạn đăng nhập. Hết hạn hoặc sai mã thì từ chối, không đổi token bừa."
[Thao tác: quay lại, không hoàn tất OAuth thật.]

**Bước 4 — Liên hệ với luồng đăng.**
[Thao tác: chỉ vào 1 ô "thiếu".]
"Kênh nào thiếu YouTube thì video kể chuyện của kênh đó đứng ở 'chờ kết nối' — máy không đăng bừa sang chỗ khác."

## Lưu ý trung thực

- Không hoàn tất OAuth bằng tài khoản thật trên video demo nếu không muốn lộ — dừng ở bước redirect.
- Nói rõ Facebook Page login từng bị hoãn theo yêu cầu — hiện chỉ cần Page ID.
