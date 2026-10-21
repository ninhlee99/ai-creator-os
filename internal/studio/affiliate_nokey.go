package studio

// Chế độ "ảnh có sẵn" (không cần Gemini key): khi media-gen chưa sẵn sàng
// mà người dùng đã tải ảnh sản phẩm/người mẫu, Studio tự cắt mỗi ảnh
// thành vài khung (toàn cảnh, cận giữa, nửa trên, nửa dưới) rồi dựng
// montage dissolve ổn định — không thêm zoom, rung hay flash.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// photoVariantCrops: biểu thức crop ffmpeg (w:h:x:y) cho từng khung.
var photoVariantCrops = []string{
	"iw:ih:0:0",                           // toàn cảnh
	"iw*0.78:ih*0.78:(iw-ow)/2:(ih-oh)/2", // cận giữa
	"iw*0.85:ih*0.62:(iw-ow)/2:0",         // nửa trên
	"iw*0.85:ih*0.62:(iw-ow)/2:ih-oh",     // nửa dưới
}

// derivePhotoVariants cắt src thành các khung dọc 9:16 (1080x1920, nền
// mờ lấp viền) để đủ ảnh cho dựng nhịp. crops = số khung đầu tiên trong
// photoVariantCrops (1..4). Khung lỗi bị bỏ qua.
func derivePhotoVariants(ctx context.Context, src, work, prefix string, crops int) []string {
	if crops < 1 || crops > len(photoVariantCrops) {
		crops = len(photoVariantCrops)
	}
	var out []string
	for i, c := range photoVariantCrops[:crops] {
		dst := filepath.Join(work, fmt.Sprintf("%s-%02d.png", prefix, i))
		// Nền: chính ảnh phóng to + blur; tiền cảnh: khung crop vừa khít.
		filter := fmt.Sprintf("[0:v]crop=%s,split=2[a][b];"+
			"[a]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,boxblur=24:2[bg];"+
			"[b]scale=1080:1920:force_original_aspect_ratio=decrease[fg];"+
			"[bg][fg]overlay=(W-w)/2:(H-h)/2", c)
		if err := ffmpegRun(ctx, "-i", src, "-filter_complex", filter, "-frames:v", "1", dst); err != nil {
			continue
		}
		out = append(out, dst)
	}
	return out
}

func isValidImage(path string) bool {
	if s, err := os.Stat(path); err == nil && s.Size() > 50 {
		return true
	}
	return false
}

// maxOwnPhotos giới hạn số ảnh gốc dùng cho 1 video (đủ đa dạng, dựng nhanh).
const maxOwnPhotos = 8

// ownSources: ảnh người dùng/listing theo thứ tự ưu tiên (sản phẩm trước, mẫu sau).
func ownSources(p AffiliateParams) []string {
	var src []string
	all := append([]string{p.ProductPhoto, p.ModelPhoto}, p.ModelPhotos...)
	all = append(all, p.ExtraPhotos...)
	for _, s := range all {
		s = strings.TrimSpace(s)
		if s != "" && len(src) < maxOwnPhotos {
			dup := false
			for _, existing := range src {
				if existing == s {
					dup = true
					break
				}
			}
			if !dup {
				src = append(src, s)
			}
		}
	}
	return src
}

// HasOwnPhotos báo job có ảnh tải lên để dựng không cần key.
func (p AffiliateParams) HasOwnPhotos() bool { return len(ownSources(p)) > 0 }

// ownPhotoSet dựng bộ khung từ ảnh thật của người mẫu và sản phẩm.
// Khi có từ 3 ảnh trở lên (nhiều góc mặt/dáng người thật), hệ thống giữ nguyên
// độ sắc nét từng ảnh chuyển thành khung dọc 1080x1920 chuẩn 9:16, không cắt xén thô bạo.
func ownPhotoSet(ctx context.Context, p AffiliateParams, work string) []string {
	src := ownSources(p)
	if len(src) == 0 {
		return nil
	}
	var photos []string
	if len(src) >= 3 {
		for i, s := range src {
			dst := filepath.Join(work, fmt.Sprintf("own_%02d.png", i))
			filter := "[0:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,boxblur=24:2[bg];" +
				"[0:v]scale=1080:1920:force_original_aspect_ratio=decrease[fg];" +
				"[bg][fg]overlay=(W-w)/2:(H-h)/2"
			if err := ffmpegRun(ctx, "-i", s, "-filter_complex", filter, "-frames:v", "1", dst); err == nil && isValidImage(dst) {
				photos = append(photos, dst)
			} else {
				photos = append(photos, s)
			}
		}
		// Vòng lặp các góc để video đạt 6–8 phân cảnh bắt mắt cho video 30-60 giây
		if len(photos) > 0 && len(photos) < 6 {
			orig := make([]string, len(photos))
			copy(orig, photos)
			for len(photos) < 6 {
				photos = append(photos, orig...)
			}
			if len(photos) > 8 {
				photos = photos[:8]
			}
		}
	} else {
		// Ít ảnh (1–2 ảnh): chia thành các góc cận / toàn
		crops := 4
		if len(src) == 2 {
			crops = 2
		}
		for i, s := range src {
			photos = append(photos, derivePhotoVariants(ctx, s, work, fmt.Sprintf("own%d", i), crops)...)
		}
	}
	return photos
}
