# MODEL.md — Mô hình chốt: AI Creator Network

> Quyết định cuối ngày 2026-10-01. Đây là tài liệu định hướng mọi code và
> mọi quyết định vận hành tiếp theo. Khi có mâu thuẫn, file này thắng.

## 1. Mô hình kinh doanh (một câu)

**Một hệ thống, N tài khoản TikTok — mỗi tài khoản là một "AI creator" với
persona riêng, tự live giải trí theo giờ được phân bổ, thu quà tặng LIVE,
bán affiliate qua video ngắn, và nhạc AI tự sáng tác thì phát hành lấy
royalty.** Người vận hành chỉ bật ON và theo dõi.

## 2. Vai trò của con người (tối thiểu, đúng yêu cầu)

| Việc của bạn | Tần suất |
|---|---|
| Bật/tắt master switch (ON/OFF) | Khi muốn |
| Thêm account mới: nhập username + RTMP key + gợi ý niche (không bắt buộc) | Mỗi lần mở kênh mới |
| Xem dashboard / báo cáo ngày | Khi muốn |
| Phê duyệt lần đầu cho persona/niche mới (sau đó tự chạy) | Một lần mỗi persona |

Mọi thứ còn lại — research niche, gán persona, sản xuất nội dung, cày
follower, xếp lịch live, live, đối soát, tối ưu — hệ thống tự làm.

## 3. Persona: mỗi account một "con người AI" khác nhau

Ưu tiên mỗi account một persona khác nhau (chống spam filter và chống
liên đới khi 1 account bị phạt). Khi số account vượt số persona khả dụng,
tái sử dụng persona ít dùng nhất — nhưng không bao giờ để 2 account cùng
persona/nội dung na ná trong cùng khung giờ.

| Persona | Live làm gì | Nguồn thu chính | Kỹ thuật |
|---|---|---|---|
| 📖 **Storyteller** | Kể chuyện đêm khuya (truyện AI tự viết) | Gift + affiliate sách | TTS + avatar nói |
| 🎓 **AI Teacher** | Dạy tiếng Anh qua truyện/tình huống | Gift + affiliate sách/khóa học + Series sau này | TTS + avatar nói + bảng viết |
| 🎤 **AI Musician** | Hát nhạc AI **tự sáng tác** (cấm cover) | Gift + SoundOn royalty + YouTube YPP | Music pipeline + TTS hát + avatar |
| 🎮 **Game Master** | AI **tự chơi** game offline/game cho phép bot | Gift + affiliate gear | Game agent + vision + TTS bình luận |
| 💻 **AI Coder** | **Live code game/mini-app theo yêu cầu khán giả** | Gift "đặt hàng tính năng" + affiliate khóa học/laptop | Code agent + screen capture + TTS giảng |
| 💃 Dancer | Nhảy theo nhạc (phase 2) | Gift + affiliate thời trang | Motion/avatar — để sau |

**Lằn ranh đỏ cho Teacher:** không dạy y tế/tài chính/luật (AI avatar đóng
vai chuyên gia nhóm này bị cấm monetize trên YouTube và bị TikTok soi).
Nội dung dạy học phải qua lớp kiểm chứng sự thật trước khi lên sóng
(chống misinformation — TikTok xử lý bất kể vô tình hay cố ý).

## 3b. Chính sách chủ đề (topic policy) — chốt 2026-10-01

Gợi ý của người dùng là **hint**, không phải mệnh lệnh. Model tự quyết định
chủ đề live/nội dung cho từng account:

- Người dùng **có thể** set chủ đề affiliate cho từng account
  (`--account-add --topic "..."` hoặc `--account-topic`). Khi có hint, model
  tôn trọng và chỉ cụ thể hoá thành niche + danh sách tập.
- Không set → model **tự nghiên cứu** qua LLM chain (tránh trùng niche với
  các account khác trong mạng).
- Quyết định cuối cùng thuộc về model, dựa trên persona và dữ liệu hiệu quả
  thực tế (analyst). Người dùng **không can thiệp vào live**.

## 3c. Phân phối đa nền tảng

Mỗi video render xong đi qua `publishers/`:
- **TikTok** — Content Posting API, draft-first.
- **Facebook** — Page video post qua `facebook-cli` (draft-first).
- **YouTube** — Data API v3, **theo từng kênh**: mỗi account có channel riêng
  và khai báo loại nội dung kênh đó nhận (`short_film`, `ai_music`,
  `ai_remix`…); video sai loại không bao giờ đăng nhầm kênh.

## 3d. Phim AI ngắn

Storyteller sản xuất phim ngắn dọc 60–180s: LLM viết kịch bản → chia cảnh →
ảnh minh hoạ AI → Ken Burns → TTS dẫn truyện → phụ đề → 1080x1920.
Pipeline đã test end-to-end bằng FFmpeg thật (`agents/content/film.py`).

## 4. Trụ cột doanh thu (theo thứ tự ưu tiên)

1. **Quà tặng LIVE** — đã verify: mở ở VN, không phân biệt AI/người, miễn
   không phải live bán hàng. Trụ cột #1 của mọi persona.
2. **Affiliate qua video ngắn + showcase** — Hunter săn sản phẩm theo niche
   từng account; Content đăng video gắn link. Không bán hàng trong live.
3. **SoundOn royalty** — nhạc AI tự sáng tác của persona Musician được phát
   hành, giữ 100% royalty trên nền tảng ByteDance. Phải qua kiểm tra "đạo
   nhạc" (ACRCloud) trước khi phát hành.
4. **Phase 2:** TikTok Series (bán khóa học/truyện), LIVE Subscription,
   YouTube YPP (đa nền tảng cho Musician/Teacher).

Không đặt cược vào Creator Rewards ở VN cho đến khi tự kiểm tra trong
TikTok Studio (đa số nguồn chính thống không liệt kê VN).

## 5. Vòng đời một account (tự động)

```
onboarding → researching → persona_assigned → growing → live_ready
    → live ⟳ (optimize) → paused | penalized | retired
```

1. **onboarding** — bạn thêm account (username, RTMP key, gợi ý niche).
2. **researching** — hệ thống tự tìm: trend/hashtag niche, giờ vàng của
   niche, đối thủ đang live kiểu gì, sản phẩm affiliate nào hợp.
3. **persona_assigned** — gán persona phù hợp nhất (hoặc theo gợi ý của bạn):
   voice profile, phong cách avatar, trụ nội dung, niche affiliate.
4. **growing** — Content factory tự đăng video ngắn đến khi đủ 1.000 follower
   (điều kiện mở LIVE). Chưa đủ thì chưa live.
5. **live_ready → live** — Scheduler xếp lịch, Streamer live, Analyst đo.
6. **optimize** — hàng tuần: persona/account nào gift tốt được ưu tiên giờ
   vàng; persona nào kém bị giảm suất hoặc đổi niche.
7. **penalized** — account bị phạt: pause ngay, các account khác cũng pause
   hành vi tương tự (không "né" bằng account khác — đó là ban evasion).
   Chờ bạn quyết định thủ công mới mở lại.

## 6. Scheduler: tự phân bổ giờ live

Khung giờ vàng VN (ICT): **11:30–13:30, 19:00–23:00**.

Luật cứng (code, không phải gợi ý):

- `max_concurrent_lives` mặc định **2** trên Mac M1 Pro 32GB (mỗi live AI
  tốn ~1.5–2.5GB RAM render + encode; đo thực tế rồi chỉnh).
- Mỗi account: **1–2 live/ngày, 60–120 phút/live**, nghỉ ít nhất 1 ngày/tuần.
- Không xếp 2 account cùng persona/niche trong cùng khung giờ.
- Không live 2 account cùng lúc trong 15 phút đầu (tránh pattern máy móc).
- Ưu tiên giờ vàng cho account có gift/follower tốt nhất (analyst xếp hạng).
- Live so le để phủ nhiều khung giờ, không dồn cục tốn RAM.

## 7. Guardrails multi-account (sống còn)

1. **Không tương tác chéo** — các account không like/follow/comment cho nhau.
2. **Không trùng nội dung** — dedup hash trên toàn mạng; kịch bản, câu chữ,
   thumbnail phải khác nhau giữa các account.
3. **Persona khác biệt thật** — giọng, avatar, niche, khung giờ khác nhau.
4. **Không né phạt** — 1 account bị phạt → pause hành vi tương tự toàn mạng.
5. **Mỗi RTMP key 1 account** — key lưu trong Keychain/env, không trong repo.
6. **Disclosure AI** — overlay + nhãn AI trên mọi live, mọi account.

## 8. Ngôn ngữ lập trình (chốt)

Nguyên tắc: **nhẹ + nhanh + dễ** — RAM/CPU là tài nguyên khan nhất.

| Thành phần | Ngôn ngữ | Vì sao |
|---|---|---|
| Orchestrator, scheduler, account manager, control-plane API | **Go** | 1 binary ~10MB, RAM 5–20MB/service, khởi động tức thì, goroutine chạy N account song song, build 1 lệnh. Nhẹ hơn Python hàng chục lần ở cùng tải. |
| Stream supervisor | **Go** (đã có) | Đang chạy tốt, giữ nguyên. |
| Game-playing agent | **Python** | Bắt buộc vì hệ sinh thái: mss, pyautogui, OpenCV. Chạy như worker riêng, Go gọi qua subprocess. |
| Local TTS glue, music pipeline glue | **Python** | Bắt buộc vì lib TTS/music. Worker riêng, có fallback API. |
| Ledger | **SQLite WAL** | 0 ops, nhẹ SSD, đủ cho quy mô này. |
| Media | **FFmpeg** (binary ngoài) | Chuẩn ngành, không thay thế. |

**Không dùng:** Node.js (ngốn RAM), Rust (tốc độ dev chậm cho team 1 người),
stack nặng (K8s, Java, frontend build phức tạp).

Lộ trình: scaffold hiện tại bằng Python (đã test được) giữ nguyên làm
tham chiếu; các hot path (scheduler, account manager, API) migrate sang Go
khi wire production. Logic nghiệp vụ (persona, luật schedule, governance)
giữ nguyên — chỉ đổi ngôn ngữ thực thi.

## 8b. Game shortlist cho Game Master (Phase 2) — verify 2026-10-01

Nguyên tắc: chỉ chơi game **có bot/API chính thức** hoặc **offline hoàn toàn**
(không server, không anti-cheat để ban). Mỗi game kiểm tra lại ToS ngay trước
ngày live đầu tiên.

| Game | Vì sao an toàn với tool | AI chơi bằng gì | Góc content LIVE |
|---|---|---|---|
| ♟️ Cờ vua — Lichess | Bot API chính thức; Lichess cho phép BOT account (scope `bot:play`), engine được phép | Stockfish + API | Khán giả thách đấu bot, gift để gợi ý nước đi, AI bình luận tiếng Việt |
| ⛏️ Minecraft Java (server/world riêng) | Mineflayer là bot API chuẩn; chơi trên world của mình = luật của mình | Mineflayer + pathfinder | AI sinh tồn/xây nhà, chat vote công trình |
| 🃏 Balatro | Offline single-player, không anti-cheat, không server | Đọc cửa sổ game + phím ảo | Poker roguelike đang hot TikTok; gift chọn lá bài |
| 🧛 Vampire Survivors / Brotato | Offline hoàn toàn | Đọc màn hình + phím ảo | Sinh tồn càng lâu gift càng to |
| 🕹️ Retro qua emulator (Pokémon, Mario…) | Offline, không ToS server nào | Input automation | Series "AI học chơi game" |
| 🏔️ Slay the Spire | Offline + hỗ trợ mod chính thức | Mod API + AI | Chat chọn đường đi, build deck |

**Cấm tuyệt đối:** Valorant, Fortnite, Liên Minh Huyền Thoại, Genshin Impact,
PUBG, Apex — anti-cheat ban tool/macro, đụng vào là mất account.

**Quy tắc vàng:** bot Lichess PHẢI là BOT account đã upgrade (không bao giờ
chạy bot trên nick người chơi thường); bot Minecraft KHÔNG vào server công
cộng (Hypixel…) — chỉ world/server của mình.

## 9. Kiến trúc hệ thống (tổng quan)

```
                    ┌──────────────┐
                    │ MASTER SWITCH│  bạn: ON/OFF + thêm account + xem
                    │  + DASHBOARD │
                    └──────┬───────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│ ORCHESTRATOR (Go)                                           │
│  AccountManager │ PersonaEngine │ Scheduler │ Governance     │
└──┬──────────┬──────────┬─────────┴──────────┬────────────────┘
   │          │          │                    │
   ▼          ▼          ▼                    ▼
┌────────┐ ┌───────┐ ┌─────────┐      ┌──────────────┐
│HUNTER  │ │CONTENT│ │STREAMER │      │   ANALYST    │
│săn sp  │ │video  │ │live show│      │ gift/roi/    │
│affiliate│ │ngắn  │ │theo    │      │ tối ưu lịch  │
│theo    │ │theo   │ │persona  │      │              │
│niche   │ │niche  │ │        │      │              │
└───┬────┘ └───┬───┘ └────┬────┘      └──────┬───────┘
    │          │          │                   │
    ▼          ▼          ▼                   ▼
┌──────────────────────────────────────────────────────┐
│ ENGINES (free API → local → paid)                     │
│ llm │ tts │ avatar │ music │ game-vision               │
└──────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────┐
│ STREAM-ENGINE (Go) × N account — mỗi live 1 supervisor │
│ FFmpeg RTMP push, watchdog, reconnect, resource cap    │
└──────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────┐
│ LEDGER (SQLite WAL) — per-account: gift, order,       │
│ commission, session, decision, api_usage               │
└──────────────────────────────────────────────────────┘
```

## 10. Thứ tự triển khai

- **Phase 1 (hiện tại):** Storyteller + AI Teacher — kỹ thuật nhẹ nhất
  (TTS + avatar nói), policy sạch nhất, affiliate sách/khóa học rõ ràng.
- **Phase 2:** Game Master + AI Coder — game shortlist xem §8b; AI Coder
  live code game theo yêu cầu chat (vòng lặp: viết → chạy → chiếu → debug
  live, gift để "đặt hàng" tính năng).
- **Phase 3:** AI Musician — cần music pipeline + kiểm tra đạo nhạc + SoundOn.
- **Phase 4:** Dancer, YouTube đa nền tảng, Series/Subscription.

Mỗi phase chỉ mở khi phase trước có gift về (analyst xác nhận bằng số liệu
thật, không phải cảm tính).
