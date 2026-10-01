"""Persona engine: one distinct AI creator per account.

Never run two accounts on the same persona with near-identical content —
TikTok spam filters and cross-account linkage make that the fastest way
to lose the whole network at once.
"""
from __future__ import annotations

# phase: 1 = build now, 2 = later (needs harder tech)
PERSONAS: dict[str, dict] = {
    "storyteller": {
        "phase": 1,
        "label": "📖 Storyteller",
        "live_style": "Kể chuyện đêm khuya, truyện AI tự viết, giọng TTS ấm",
        "revenue": ["live_gift", "affiliate_sach", "series_later"],
        "affiliate_niches": ["sách", "đèn đọc sách", "trà", "nến thơm"],
        "content_pillars": ["truyện ngắn", "truyện ma", "câu chuyện đời"],
        "voice_style": "warm-female" ,
        "avatar_style": "stylized-reader",
        "keywords": ["truyện", "kể chuyện", "story", "đêm khuya", "sách"],
        "red_lines": ["không đọc sách có bản quyền nguyên văn"],
    },
    "teacher": {
        "phase": 1,
        "label": "🎓 AI Teacher",
        "live_style": "Dạy tiếng Anh qua truyện/tình huống, bảng viết minh họa",
        "revenue": ["live_gift", "affiliate_sach_khoahoc", "series_later"],
        "affiliate_niches": ["sách tiếng Anh", "khóa học", "văn phòng phẩm"],
        "content_pillars": ["từ vựng theo chủ đề", "tiếng Anh qua truyện",
                            "luyện nghe"],
        "voice_style": "clear-teacher",
        "avatar_style": "stylized-teacher",
        "keywords": ["tiếng anh", "học", "dạy", "english", "learn",
                     "từ vựng"],
        # Hard policy lines (YouTube YPP + TikTok misinformation rules)
        "red_lines": [
            "KHÔNG dạy y tế / tài chính / luật (AI expert ban)",
            "mọi bài học qua lớp kiểm chứng sự thật trước khi live",
        ],
    },
    "musician": {
        "phase": 3,
        "label": "🎤 AI Musician",
        "live_style": "Hát nhạc AI TỰ SÁNG TÁC (cấm cover), giao lưu yêu cầu",
        "revenue": ["live_gift", "soundon_royalty", "ypp_later"],
        "affiliate_niches": ["mic karaoke", "loa bluetooth", "tai nghe"],
        "content_pillars": ["bài hát mới mỗi tuần", "live acoustic AI",
                            "behind-the-song"],
        "voice_style": "singer",
        "avatar_style": "stylized-performer",
        "keywords": ["nhạc", "hát", "music", "sing", "karaoke", "lofi"],
        "red_lines": [
            "CẤM cover nhạc có sẵn",
            "chỉ dùng model đã license + kiểm tra đạo nhạc trước phát hành",
        ],
    },
    "gamer": {
        "phase": 2,
        "label": "🎮 Game Master",
        "live_style": "AI TỰ CHƠI game + bình luận tiếng Việt",
        "revenue": ["live_gift", "affiliate_gear"],
        "affiliate_niches": ["chuột", "bàn phím", "tai nghe gaming",
                             "ghế gaming", "đồ ăn vặt"],
        "content_pillars": ["full gameplay", "thử thách", "highlight"],
        "voice_style": "energetic-caster",
        "avatar_style": "pngtuber-caster",
        "keywords": ["game", "chơi game", "gaming", "esport"],
        "red_lines": [
            "chỉ chơi game offline / game cho phép bot (check ToS từng game)",
            "không cheat game online competitive",
        ],
    },
    "dancer": {
        "phase": 4,
        "label": "💃 Dancer",
        "live_style": "Avatar AI nhảy theo nhạc",
        "revenue": ["live_gift", "affiliate_thoitrang"],
        "affiliate_niches": ["quần áo", "giày sneaker", "đồ tập"],
        "content_pillars": ["dance cover", "trend dance"],
        "voice_style": "upbeat-host",
        "avatar_style": "stylized-dancer",
        "keywords": ["nhảy", "dance", "múa", "thời trang"],
        "red_lines": [],
    },
}

ASSIGNABLE = [k for k, v in PERSONAS.items() if v["phase"] <= 2]


def _keyword_hit(niche_hint: str) -> str | None:
    hint = (niche_hint or "").lower()
    for name, p in PERSONAS.items():
        if name not in ASSIGNABLE:
            continue
        if any(kw in hint for kw in p["keywords"]):
            return name
    return None


def assign_persona(niche_hint: str = "",
                   active_personas: list[str] | None = None,
                   allow_phase: int = 2) -> str:
    """Pick a persona for a new account.

    1. Keyword match on the human's niche hint wins.
    2. Otherwise the least-used assignable persona (diversity first).
    3. Never assigns the same persona twice while an unused one exists.
    Deterministic: ties broken alphabetically.
    """
    active = list(active_personas or [])
    pool = [k for k in ASSIGNABLE
            if PERSONAS[k]["phase"] <= allow_phase]

    hit = _keyword_hit(niche_hint)
    if hit and hit not in active:
        return hit

    # diversity: prefer personas with zero active accounts
    unused = sorted(set(pool) - set(active))
    if unused:
        return unused[0]
    # all used: reuse the least-used one
    counts = {p: active.count(p) for p in pool}
    return sorted(pool, key=lambda p: (counts[p], p))[0]


def describe(persona: str) -> dict:
    return PERSONAS[persona]
