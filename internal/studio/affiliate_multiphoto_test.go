package studio

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 1 ảnh → 4 khung cắt; ≥3 ảnh → 2 khung/ảnh (video là nhiều ảnh thật khác
// nhau, không lặp 1 ảnh).
func TestOwnPhotoSetUsesAllPhotos(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("cần ffmpeg")
	}
	dir := t.TempDir()
	mk := func(i int) string {
		p := filepath.Join(dir, fmt.Sprintf("in%d.png", i))
		if err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i",
			fmt.Sprintf("color=c=0x%02x4080:s=600x800", i*40), "-frames:v", "1", p).Run(); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ctx := context.Background()
	one := ownPhotoSet(ctx, AffiliateParams{ProductPhoto: mk(1)}, t.TempDir())
	if len(one) != 4 {
		t.Fatalf("1 ảnh → 4 khung, got %d", len(one))
	}
	three := ownPhotoSet(ctx, AffiliateParams{ProductPhoto: mk(2), ExtraPhotos: []string{mk(3), mk(4)}}, t.TempDir())
	if len(three) != 6 {
		t.Fatalf("3 ảnh → 6 khung, got %d", len(three))
	}
	if !(AffiliateParams{ExtraPhotos: []string{"x"}}).HasOwnPhotos() {
		t.Fatal("chỉ có ảnh thêm vẫn dựng được không cần key")
	}
}

func TestTemplateCaptionProductTagAndHonestCTA(t *testing.T) {
	c := TemplateCaption("Túi kem quilted", "thời trang/phụ kiện")
	for _, want := range []string{"#tuikemquilted", "#thoitrang", "bio/mô tả"} {
		if !strings.Contains(c, want) {
			t.Fatalf("caption thiếu %q:\n%s", want, c)
		}
	}
	if strings.Contains(c, "giỏ") || strings.Contains(c, "#tiktokshop") {
		t.Fatalf("không được hứa giỏ hàng khi video không gắn giỏ:\n%s", c)
	}
	if TemplateCaption("Áo A", "") == TemplateCaption("Nồi chiên B", "") {
		t.Fatal("caption phải khác nhau theo sản phẩm")
	}
}
