# TikTok Affiliate OS

**Hệ điều hành "công ty một người": đội AI tự vận hành vòng kinh doanh affiliate TikTok — săn sản phẩm → làm nội dung → livestream → đối soát hoa hồng → tái đầu tư.**

![Tổng quan hệ thống](docs/assets/hero-overview.webp)

> 🖼️ **Ghi chú trung thực:** các ảnh và video trong README này là **concept minh họa do AI tạo**, giúp hình dung ý tưởng — không phải ảnh chụp sản phẩm thật. Sơ đồ kiến trúc (mermaid) mô tả đúng code hiện tại.

---

## 🎬 Xem nhanh

<video src="docs/assets/concept-teaser.mp4" controls width="100%"></video>

*Video concept: đội agent vận hành trung tâm điều khiển affiliate.*

### Dashboard điều khiển (mockup giao diện dự kiến)

![Dashboard mockup](docs/assets/dashboard-mockup.webp)

*Mockup: GMV, đơn hàng, hoa hồng, phiên live đang chạy, trạng thái 4 agent và nút kill switch.*

---

## 💰 Vòng tiền — một vòng duy nhất

```mermaid
flowchart LR
    D[🔍 Discover<br/>Hunter quét sản phẩm] --> A2[🎣 Attract<br/>Video + Live kéo view]
    A2 --> C2[💰 Convert<br/>viewer thành đơn hàng]
    C2 --> R[🧾 Reconcile<br/>đối soát hoa hồng]
    R --> R2[🔁 Reinvest<br/>đổ tiền vào sản phẩm thắng]
    R2 --> D
```

Mọi agent chỉ phục vụ một mục tiêu: **đồng hoa hồng quay vòng nhanh hơn, to hơn**.

---

## 🏗️ Kiến trúc tổng quan

```mermaid
flowchart TB
    subgraph Agents["Bốn agent"]
        H[🎯 Hunter<br/>săn sản phẩm]
        C[🎬 Content<br/>làm video]
        S[📡 Streamer<br/>điều khiển live]
        A[📊 Analyst<br/>phân tích, kill/scale]
    end
    subgraph Core["Core — Python stdlib, không lib ngoài"]
        O[Orchestrator<br/>điều phối lịch chạy]
        G[Governance<br/>luật cứng, không LLM]
        L[(Ledger<br/>SQLite WAL)]
    end
    subgraph Engines["Engines — chuỗi provider"]
        LLM[LLM<br/>Gemini free → Ollama local]
        TTS[TTS<br/>VieNeu local → Gemini → Azure]
        AV[Avatar<br/>stylized realtime]
    end
    subgraph Platform["TikTok — API chính thức"]
        SHOP[Shop Open API<br/>săn + đối soát]
        POST[Content Posting API<br/>đăng video]
        RTMP[RTMP<br/>đẩy live]
    end
    O --> H & C & S & A
    H & C & S & A --> G --> L
    H --> SHOP
    A --> SHOP
    C --> POST
    S --> RTMP
    H & C & S --> LLM & TTS & AV
```

**Nguyên tắc chọn công nghệ:** nhẹ, ít RAM/CPU/SSD → Python cho não, Go cho stream-engine, SQLite cho sổ cái. API miễn phí trước, local fallback sau, trả phí chỉ khi tùy chọn.

![Đội agent](docs/assets/agents-team.webp)

---

## 🤖 Từng agent làm gì

### 🎯 Hunter — thợ săn sản phẩm (`agents/hunter/agent.py`)

```mermaid
flowchart TB
    S1[Quét Affiliate Marketplace<br/>qua Shop Open API] --> S2{Lọc: commission,<br/>giá, uy tín shop}
    S2 -->|đạt| S3[Tính điểm **earning kỳ vọng**<br/>không theo % hoa hồng]
    S2 -->|loại| S4[Bỏ qua]
    S3 --> S5{Điểm >= ngưỡng?}
    S5 -->|có| S6[Đưa vào danh sách thử nghiệm]
    S5 -->|không| S4
```

- Điểm khắt khe: sản phẩm 15% hoa hồng × bán chạy **thắng** sản phẩm 40% hoa hồng × ế.
- Dùng endpoint chính thức `Creator Search Open Collaboration Product` (sort theo commission, units_sold).

### 🎬 Content — xưởng video (`agents/content/agent.py`)

```mermaid
flowchart TB
    C1[Nhận sản phẩm từ Hunter] --> C2[LLM viết kịch bản<br/>hook + demo + CTA]
    C2 --> C3[TTS đọc giọng<br/>VieNeu local → Gemini → Azure]
    C3 --> C4[FFmpeg dựng video<br/>caption + overlay giá]
    C4 --> C5{Kiểm duyệt<br/>policy + chất lượng}
    C5 -->|đạt| C6[Đăng qua Content Posting API<br/>Direct Post]
    C5 -->|chưa đạt| C2
```

- Đăng hands-free sau khi app qua audit TikTok (~1 tháng). Giới hạn ~15 video/ngày.

### 📡 Streamer — đạo diễn live (`agents/streamer/agent.py`)

```mermaid
flowchart TB
    T1[Chuẩn bị phiên live] --> T2{Chế độ live?}
    T2 -->|Game / giải trí<br/>không bán hàng| T3[Gameplay + TTS + chatbot<br/>đọc và trả lời comment]
    T2 -->|Bán hàng Shop| T4[⚠️ Bắt buộc host người thật<br/>TikTok cấm giọng AI]
    T3 --> T5[Đẩy RTMP qua<br/>stream-engine Go + FFmpeg]
    T4 --> T5
    T5 --> T6[Giám sát realtime<br/>kill switch khi vi phạm]
```

![Concept live](docs/assets/livestream-concept.webp)

*Concept: live có host người + AI điều khiển overlay, ánh sáng, analytics.*

> ⚠️ **Phát hiện policy quan trọng (10/2026):** TikTok Shop cấm giọng AI, audio thu sẵn và avatar hoạt hình >50% màn hình trong **livestream bán hàng**. Live game/giải trí không dùng tính năng Shop nằm ngoài phạm vi cấm này. Chi tiết: `docs/RESEARCH/tiktok_live_policy.md`.

### 📊 Analyst — kế toán lạnh lùng (`agents/analyst/agent.py`)

```mermaid
flowchart TB
    A1[Đối soát đơn hàng<br/>qua Shop API] --> A2[Tính GMV, CR,<br/>EPC, ROAS]
    A2 --> A3{Đạt ngưỡng scale?}
    A3 -->|có| A4[Nhân rộng:<br/>thêm video + phiên live]
    A3 -->|không| A5{Thua lỗ / 0 đơn?}
    A5 -->|có| A6[⛔ Cắt — kill rule cứng]
    A5 -->|chưa rõ| A7[Tiếp tục theo dõi]
```

- Kill rule không cảm xúc: 0 đơn sau X view/phiên live là cắt, không tranh cãi.
- Số liệu chỉ ghi từ API TikTok — **không bao giờ tự bịa doanh thu**.

---

## 🛡️ Governance — luật cứng, không phải lời khuyên

```mermaid
flowchart TB
    G1[Mọi hành động của agent] --> G2{Dry-run?}
    G2 -->|bật| G3[Chặn mọi tác động ngoài<br/>chỉ ghi log]
    G2 -->|tắt| G4{Kill switch?}
    G4 -->|bật| G5[Dừng toàn hệ thống]
    G4 -->|tắt| G6{Vi phạm luật?}
    G6 -->|0 đơn sau X view| G7[Cắt sản phẩm]
    G6 -->|vượt budget| G8[Chặn chi tiêu]
    G6 -->|ok| G9[Cho phép + ghi audit]
```

- Governance **không chứa LLM** — toàn bộ là code xác định, test được.
- Sổ cái append-only: tiền vào/ra đều có chứng từ.

---

## 🖥️ Chạy trên Mac của bạn

```mermaid
flowchart TB
    subgraph Mac["Mac M1 Pro 32GB"]
        LD[launchd<br/>orchestrator]
        LS[launchd<br/>stream-engine Go]
        FF[FFmpeg<br/>đẩy RTMP]
        DB[(SQLite<br/>ledger)]
        LO[VieNeu-TTS<br/>chạy local]
    end
    LD --> LS --> FF
    LD --> DB
    LO --> FF
    FF -->|RTMP| TK[TikTok LIVE]
    LD -->|HTTPS API| API2[Shop + Posting API]
```

- Hai service `launchd` tự chạy khi mở máy, tự khởi động lại khi sập.
- Stream-engine giới hạn tài nguyên, tự reconnect khi rớt mạng.

---

## 🧪 Demo thử (rehearsal offline)

```bash
make rehearse   # chạy toàn bộ vòng tiền với dữ liệu giả, không chạm TikTok thật
```

Video demo thật sẽ được quay lại sau khi chạy rehearsal trên máy bạn.

---

## 📌 Trạng thái thật của dự án (v0.1)

| Phần | Trạng thái |
|---|---|
| Kiến trúc, 4 agent, governance, ledger, stream-engine | ✅ Code xong, test governance pass |
| Research API TikTok Shop / LIVE policy / TTS+avatar | ✅ Xong, trong `docs/RESEARCH/` |
| Wire provider thật (TTS, Shop API, Posting API) | ⏳ Chờ quyết định hướng đi |
| Streamer live | ⚠️ Chờ quyết định sau phát hiện policy TikTok |

**Các hướng đang cân nhắc:** A) thuê host người + AI làm còn lại · B) chỉ làm video ngắn AI · C) live full-AI chấp nhận rủi ro · D) **live game không lộ mặt** (AI voice + chatbot, không bán hàng trong live) + bán qua video ngắn.

📄 Tài liệu chi tiết: `docs/ARCHITECTURE.md` · `docs/OPERATIONS.md` · `docs/POLICY_AND_SAFETY.md`
