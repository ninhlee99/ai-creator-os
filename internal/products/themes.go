package products

// Theme is one affiliate topic Ninh can assign to an account. Keywords
// drive provider searches; DefaultMinCommission is the suggested floor.
type Theme struct {
	Slug                 string
	Label                string
	Keywords             []string
	DefaultMinCommission float64
}

// THEMES is the curated catalog Ninh picks from per account.
var THEMES = map[string]Theme{
	"thoi-trang-nu": {
		Slug:  "thoi-trang-nu",
		Label: "👗 Thời trang nữ",
		Keywords: []string{"váy", "đầm", "quần áo nữ", "túi xách",
			"balo nữ", "giày cao gót", "giày nữ", "phụ kiện thời trang"},
		DefaultMinCommission: 0.12,
	},
	"my-pham": {
		Slug:  "my-pham",
		Label: "💄 Mỹ phẩm",
		Keywords: []string{"son môi", "kem nền", "skincare",
			"mỹ phẩm", "chăm sóc da"},
		DefaultMinCommission: 0.15,
	},
	"gia-dung": {
		Slug:  "gia-dung",
		Label: "🏠 Gia dụng",
		Keywords: []string{"đồ gia dụng", "nhà bếp", "decor",
			"đồ dùng nhà cửa"},
		DefaultMinCommission: 0.10,
	},
	"me-va-be": {
		Slug:  "me-va-be",
		Label: "🍼 Mẹ & bé",
		Keywords: []string{"đồ sơ sinh", "mẹ và bé", "bỉm sữa",
			"đồ chơi trẻ em"},
		DefaultMinCommission: 0.10,
	},
	"cong-nghe": {
		Slug:  "cong-nghe",
		Label: "🎧 Công nghệ",
		Keywords: []string{"tai nghe", "loa bluetooth", "phụ kiện điện thoại",
			"đồ công nghệ dễ thương", "đồ công nghệ xinh"},
		DefaultMinCommission: 0.08,
	},
	"the-thao": {
		Slug:                 "the-thao",
		Label:                "👟 Thể thao",
		Keywords:             []string{"đồ tập gym", "giày thể thao", "quần áo thể thao"},
		DefaultMinCommission: 0.10,
	},
	"sach-van-phong": {
		Slug:                 "sach-van-phong",
		Label:                "📚 Sách & văn phòng",
		Keywords:             []string{"sách", "văn phòng phẩm", "đồ dùng học tập"},
		DefaultMinCommission: 0.10,
	},
}

// ThemeOrder keeps UI listing deterministic.
var ThemeOrder = []string{
	"thoi-trang-nu", "my-pham", "gia-dung", "me-va-be",
	"cong-nghe", "the-thao", "sach-van-phong",
}

// DescribeTheme returns the catalog entry; ok=false for unknown slugs
// (custom themes are allowed — they just carry no curated keywords).
func DescribeTheme(slug string) (Theme, bool) {
	t, ok := THEMES[slug]
	return t, ok
}

// QueryForTheme builds a product query from a theme slug + account policy.
func QueryForTheme(slug string, minCommission float64, limit int) Query {
	q := Query{Theme: slug, MinCommission: minCommission, Limit: limit}
	if t, ok := THEMES[slug]; ok {
		q.Keywords = t.Keywords
	} else {
		q.Keywords = []string{slug}
	}
	return q
}
