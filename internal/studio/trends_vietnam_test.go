package studio

import (
	"testing"
)

func TestVietnameseSoundFilter(t *testing.T) {
	cases := []struct {
		artist string
		title  string
		wantVN bool
	}{
		// Vietnamese songs with diacritics
		{"Dunghoangpham", "Mật Ngọt - Nam Con Remix", true},
		{"Vũ.", "Vì Anh Đâu Có Biết", true},
		{"", "Không Dấu Gạch Nối", true},
		{"Sơn Tùng M-TP", "Đừng Làm Trái Tim Anh Đau", true},
		{"Tăng Duy Tân", "Cắt Đôi Nỗi Sầu", true},
		{"Đen", "Nấu Ăn Cho Em", true},
		{"Hoàng Thùy Linh", "See Tình", true},
		{"Hòa Minzy", "Thị Mầu", true},
		{"Quang Hùng MasterD", "Thủy Triều", true},

		// Vietnamese songs without diacritics but known artists/keywords
		{"HIEUTHUHAI", "Khong The Say", true},
		{"Tang Duy Tan", "Cat Doi Noi Sau", true},
		{"Son Tung M-TP", "Chung Ta Cua Tuong Lai", true},
		{"Low G", "Co Em", true},
		{"MONO", "Waiting For You", true},
		{"Dai Meo", "Lac Vao Trong Mo Remix", true},

		// Non-Vietnamese: Thai songs
		{"FLI:P", "มองนานๆ", false},
		{"Bowkylion", "วาดไว้", false},

		// Non-Vietnamese: Chinese songs
		{"时代少年团", "特别的爱", false},
		{"周杰伦", "青花瓷", false},

		// Non-Vietnamese: Korean songs
		{"BTS", "Dynamite", false},
		{"NewJeans", "Super Shy", false},

		// Non-Vietnamese: International pop artists
		{"Lady Gaga & Bruno Mars", "Die With A Smile", false},
		{"Sabrina Carpenter", "Espresso", false},
		{"Taylor Swift", "Cruel Summer", false},
		{"Billie Eilish", "Birds of a Feather", false},
		{"Coldplay", "Viva La Vida", false},
	}

	for _, c := range cases {
		got := isVietnameseSound(c.artist, c.title)
		if got != c.wantVN {
			t.Errorf("isVietnameseSound(%q, %q) = %v; want %v", c.artist, c.title, got, c.wantVN)
		}
	}
}

func TestParseKworbVNFallbackAndReRank(t *testing.T) {
	// Mixed HTML with Thai and Chinese songs mixed with Vietnamese
	html := `<table>
<tr><td>1</td><td>=</td><td class="mp text"><div>FLI:P - มองนานๆ</div></td></tr>
<tr><td>2</td><td>+5</td><td class="mp text"><div>Dunghoangpham - Mật Ngọt - Nam Con Remix</div></td></tr>
<tr><td>3</td><td>=</td><td class="mp text"><div>时代少年团 - 特别的爱</div></td></tr>
<tr><td>4</td><td>+2</td><td class="mp text"><div>Vũ. - Vì Anh Đâu Có Biết</div></td></tr>
<tr><td>5</td><td>-1</td><td class="mp text"><div>Lady Gaga - Die With A Smile</div></td></tr>
</table>`

	sounds := parseKworbVN(html)
	if len(sounds) != 2 {
		t.Fatalf("expected 2 filtered Vietnamese sounds from kworb, got %d", len(sounds))
	}
	if sounds[0].Artist != "Dunghoangpham" || sounds[0].Rank != 1 {
		t.Errorf("expected sound #1 Dunghoangpham rank 1, got %+v", sounds[0])
	}
	if sounds[1].Artist != "Vũ." || sounds[1].Rank != 2 {
		t.Errorf("expected sound #2 Vũ. rank 2, got %+v", sounds[1])
	}

	enriched := enrichVietnameseSounds(sounds)
	if len(enriched) < 15 {
		t.Fatalf("expected at least 15 songs with enrichment, got %d", len(enriched))
	}

	// Verify all returned sounds are strictly Vietnamese
	for i, s := range enriched {
		if s.Rank != i+1 {
			t.Errorf("expected sound %d to have rank %d, got %d", i, i+1, s.Rank)
		}
		if !isVietnameseSound(s.Artist, s.Title) {
			t.Errorf("sound #%d (%s - %s) is not recognized as Vietnamese", s.Rank, s.Artist, s.Title)
		}
	}
}
