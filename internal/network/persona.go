package network

// Persona is the shared production profile; channel identity comes from each
// account's own reference images and niche.
type Persona struct {
	Label           string
	Description     string
	AffiliateNiches []string
	ContentPillars  []string
	RedLines        []string
}

// PERSONAS retains one neutral profile for onboarding. Per-channel character
// images and account niche provide the actual creative identity.
var PERSONAS = map[string]Persona{
	"creator": {
		Label:           "Nhân vật kênh",
		Description:     "Nhân vật riêng của kênh; nội dung bám chủ đề, ảnh tham chiếu và dữ liệu TikTok.",
		AffiliateNiches: []string{"sản phẩm phù hợp chủ đề và người xem của kênh"},
		ContentPillars: []string{
			"video xu hướng TikTok phù hợp với nhân vật kênh",
			"trải nghiệm sản phẩm affiliate có bằng chứng",
			"nội dung hữu ích theo chủ đề kênh",
		},
		RedLines: []string{"không bịa số liệu, công dụng hoặc kết quả sản phẩm"},
	},
}

// AssignPersona returns shared production profile. User-owned channel photos
// provide identity; no synthetic role is assigned to the character.
func AssignPersona(_ string, _ map[string]int) string { return "creator" }

// Describe resolves legacy account values to the shared profile without
// rewriting stored account data.
func Describe(name string) (Persona, bool) {
	if p, ok := PERSONAS[name]; ok {
		return p, true
	}
	p, ok := PERSONAS["creator"]
	return p, ok
}
