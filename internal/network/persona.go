package network

import (
	"sort"
	"strings"
)

// Persona is one distinct AI creator identity. Never run two accounts on the
// same persona with near-identical content — TikTok spam filters and
// cross-account linkage make that the fastest way to lose the whole network.
type Persona struct {
	Phase           int // 1 = build now, 2 = later (needs harder tech); only <= 2 is assignable
	Label           string
	LiveStyle       string
	Revenue         []string
	AffiliateNiches []string
	ContentPillars  []string
	VoiceStyle      string
	AvatarStyle     string
	Keywords        []string
	RedLines        []string
}

// PERSONAS mirrors persona.py, including the 6th persona "coder" (AI Coder,
// phase 2): the AI live-codes games/mini-apps per viewer request, with a
// gift-to-request-feature mechanic.
var PERSONAS = map[string]Persona{
	"storyteller": {
		Phase:           1,
		Label:           "📖 Storyteller",
		LiveStyle:       "Kể chuyện đêm khuya, truyện AI tự viết, giọng TTS ấm",
		Revenue:         []string{"live_gift", "affiliate_sach", "series_later"},
		AffiliateNiches: []string{"sách", "đèn đọc sách", "trà", "nến thơm"},
		ContentPillars:  []string{"truyện ngắn", "truyện ma", "câu chuyện đời"},
		VoiceStyle:      "warm-female",
		AvatarStyle:     "stylized-reader",
		Keywords:        []string{"truyện", "kể chuyện", "story", "đêm khuya", "sách"},
		RedLines:        []string{"không đọc sách có bản quyền nguyên văn"},
	},
	"teacher": {
		Phase:           1,
		Label:           "🎓 AI Teacher",
		LiveStyle:       "Dạy tiếng Anh qua truyện/tình huống, bảng viết minh họa",
		Revenue:         []string{"live_gift", "affiliate_sach_khoahoc", "series_later"},
		AffiliateNiches: []string{"sách tiếng Anh", "khóa học", "văn phòng phẩm"},
		ContentPillars:  []string{"từ vựng theo chủ đề", "tiếng Anh qua truyện", "luyện nghe"},
		VoiceStyle:      "clear-teacher",
		AvatarStyle:     "stylized-teacher",
		Keywords:        []string{"tiếng anh", "học", "dạy", "english", "learn", "từ vựng"},
		RedLines: []string{
			"KHÔNG dạy y tế / tài chính / luật (AI expert ban)",
			"mọi bài học qua lớp kiểm chứng sự thật trước khi live",
		},
	},
	"musician": {
		Phase:           3,
		Label:           "🎤 AI Musician",
		LiveStyle:       "Hát nhạc AI TỰ SÁNG TÁC (cấm cover), giao lưu yêu cầu",
		Revenue:         []string{"live_gift", "soundon_royalty", "ypp_later"},
		AffiliateNiches: []string{"mic karaoke", "loa bluetooth", "tai nghe"},
		ContentPillars:  []string{"bài hát mới mỗi tuần", "live acoustic AI", "behind-the-song"},
		VoiceStyle:      "singer",
		AvatarStyle:     "stylized-performer",
		Keywords:        []string{"nhạc", "hát", "music", "sing", "karaoke", "lofi"},
		RedLines: []string{
			"CẤM cover nhạc có sẵn",
			"chỉ dùng model đã license + kiểm tra đạo nhạc trước phát hành",
		},
	},
	"gamer": {
		Phase:           2,
		Label:           "🎮 Game Master",
		LiveStyle:       "AI TỰ CHƠI game + bình luận tiếng Việt",
		Revenue:         []string{"live_gift", "affiliate_gear"},
		AffiliateNiches: []string{"chuột", "bàn phím", "tai nghe gaming", "ghế gaming", "đồ ăn vặt"},
		ContentPillars:  []string{"full gameplay", "thử thách", "highlight"},
		VoiceStyle:      "energetic-caster",
		AvatarStyle:     "pngtuber-caster",
		Keywords:        []string{"game", "chơi game", "gaming", "esport"},
		RedLines: []string{
			"chỉ chơi game offline / game cho phép bot (check ToS từng game)",
			"không cheat game online competitive",
		},
	},
	"coder": {
		Phase:     2,
		Label:     "💻 AI Coder",
		LiveStyle: "LIVE CODE game/mini-app theo yêu cầu khán giả (viết → chạy → chiếu → debug live); gift-to-request-feature: viewer tặng quà để yêu cầu tính năng",
		Revenue:   []string{"live_gift", "gift_to_request_feature", "affiliate_khoahoc", "affiliate_laptop"},
		AffiliateNiches: []string{"khóa học lập trình", "laptop", "bàn phím cơ",
			"sách code"},
		ContentPillars: []string{"code game theo yêu cầu", "debug live",
			"thử thách 30 phút 1 game", "tính năng theo yêu cầu quà tặng"},
		VoiceStyle:  "clear-teacher",
		AvatarStyle: "pngtuber-coder",
		Keywords:    []string{"code", "lập trình", "coding", "python", "game dev", "dev"},
		RedLines: []string{
			"chỉ chạy code trong sandbox, không chạy lệnh hệ thống nguy hiểm",
			"không nhận code từ chat rồi chạy mù — kiểm duyệt trước",
		},
	},
	"dancer": {
		Phase:           4,
		Label:           "💃 Dancer",
		LiveStyle:       "Avatar AI nhảy theo nhạc",
		Revenue:         []string{"live_gift", "affiliate_thoitrang"},
		AffiliateNiches: []string{"quần áo", "giày sneaker", "đồ tập"},
		ContentPillars:  []string{"dance cover", "trend dance"},
		VoiceStyle:      "upbeat-host",
		AvatarStyle:     "stylized-dancer",
		Keywords:        []string{"nhảy", "dance", "múa", "thời trang"},
		RedLines:        []string{},
	},
}

// personaOrder is the Python dict insertion order, kept so keyword matching
// stays deterministic across ports.
var personaOrder = []string{"storyteller", "teacher", "musician", "gamer", "coder", "dancer"}

// assignable returns the personas with phase <= 2, in deterministic order.
func assignable() []string {
	var out []string
	for _, name := range personaOrder {
		if PERSONAS[name].Phase <= 2 {
			out = append(out, name)
		}
	}
	return out
}

func keywordHit(hint string) string {
	h := strings.ToLower(hint)
	for _, name := range assignable() {
		for _, kw := range PERSONAS[name].Keywords {
			if strings.Contains(h, kw) {
				return name
			}
		}
	}
	return ""
}

// AssignPersona picks a persona for a new account.
//
//  1. Keyword match on the human's niche hint wins (assignable personas only).
//  2. Otherwise the least-used assignable persona (diversity first): a persona
//     with zero active accounts is always preferred over reusing one.
//  3. Ties broken alphabetically. Deterministic.
//  4. Never assigns a persona with phase > 2.
//
// used maps persona -> number of active (non-retired) accounts on it.
func AssignPersona(hint string, used map[string]int) string {
	pool := assignable()

	if hit := keywordHit(hint); hit != "" && used[hit] == 0 {
		return hit
	}

	var unused []string
	for _, p := range pool {
		if used[p] == 0 {
			unused = append(unused, p)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		return unused[0]
	}

	best := pool[0]
	for _, p := range pool[1:] {
		if used[p] < used[best] || (used[p] == used[best] && p < best) {
			best = p
		}
	}
	return best
}

// Describe returns the persona definition; ok is false for unknown names.
func Describe(name string) (p Persona, ok bool) {
	p, ok = PERSONAS[name]
	return p, ok
}
