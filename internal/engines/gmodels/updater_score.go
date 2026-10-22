package gmodels

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var versionMatcher = regexp.MustCompile(`(\d+)(?:\.(\d+))?`)

type candidateScore struct {
	name    string
	major   int
	minor   int
	stable  int // 2 = GA/stable, 1 = preview, 0 = exp
	nonLite int // 1 = standard, 0 = lite
}

func parseScore(name string) candidateScore {
	l := strings.ToLower(name)
	m := versionMatcher.FindStringSubmatch(l)
	major, minor := 0, 0
	if len(m) > 1 {
		major, _ = strconv.Atoi(m[1])
	}
	if len(m) > 2 && m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}

	// Xếp hạng ưu tiên cho dòng Nano Banana tạo ảnh & Veo video
	if strings.Contains(l, "nano-banana-2.1") {
		major = 3
		minor = 9
	} else if strings.Contains(l, "nano-banana-2") {
		major = 3
		minor = 2
	} else if strings.Contains(l, "banana") && major < 3 {
		major = 3
		minor = 1
	}

	stable := 2
	if strings.Contains(l, "preview") {
		stable = 1
	} else if strings.Contains(l, "exp") {
		stable = 0
	}

	nonLite := 1
	if strings.Contains(l, "lite") {
		nonLite = 0
	}

	return candidateScore{
		name:    name,
		major:   major,
		minor:   minor,
		stable:  stable,
		nonLite: nonLite,
	}
}

func compareScores(a, b candidateScore) int {
	if a.major != b.major {
		if a.major > b.major {
			return 1
		}
		return -1
	}
	if a.minor != b.minor {
		if a.minor > b.minor {
			return 1
		}
		return -1
	}
	if a.stable != b.stable {
		if a.stable > b.stable {
			return 1
		}
		return -1
	}
	if a.nonLite != b.nonLite {
		if a.nonLite > b.nonLite {
			return 1
		}
		return -1
	}
	return 0
}

// PickLatest chọn model mới nhất khả dụng từ danh sách API trả về.
// So sánh theo: Phiên bản (major.minor) > Độ ổn định (GA > Preview > Exp) > Phiên bản đầy đủ (Chuẩn > Lite).
func PickLatest(names []string, k Kind) string {
	var cands []candidateScore
	for _, raw := range names {
		n := strings.ToLower(raw)
		ok := false
		switch k {
		case Text:
			ok = strings.HasPrefix(n, "gemini-") && strings.Contains(n, "flash") &&
				!has(n, "image", "tts", "audio", "live", "embedding", "banana", "vision", "veo")
		case Image:
			ok = has(n, "banana", "image") && !has(n, "imagen", "tts", "audio", "live", "embedding", "veo")
		case TTS:
			ok = strings.Contains(n, "tts")
		case Video:
			ok = strings.Contains(n, "veo") || strings.Contains(n, "video")
		}
		if ok {
			cands = append(cands, parseScore(raw))
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.SliceStable(cands, func(i, j int) bool {
		cmp := compareScores(cands[i], cands[j])
		if cmp != 0 {
			return cmp > 0
		}
		// Nếu điểm bằng nhau, ưu tiên tên ngắn gọn (alias chính thức như gemini-3.8-flash thay vì gemini-3.8-flash-001)
		return len(cands[i].name) < len(cands[j].name)
	})
	return cands[0].name
}
