package studio

// Chế độ "ảnh có sẵn" (không cần Gemini key): khi media-gen chưa sẵn sàng
// mà người dùng đã tải ảnh sản phẩm/người mẫu, Studio tự cắt mỗi ảnh
// thành vài khung (toàn cảnh, cận giữa, nửa trên, nửa dưới) rồi dựng
// giật-giật theo nhịp như bản AI — không chèn chữ.

import (
	"context"
	"fmt"
	"path/filepath"
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

// maxOwnPhotos giới hạn số ảnh gốc dùng cho 1 video (đủ đa dạng, dựng nhanh).
const maxOwnPhotos = 8

// ownSources: ảnh người dùng/listing theo thứ tự ưu tiên (sản phẩm trước).
func ownSources(p AffiliateParams) []string {
	var src []string
	for _, s := range append([]string{p.ProductPhoto, p.ModelPhoto}, p.ExtraPhotos...) {
		if s != "" && len(src) < maxOwnPhotos {
			src = append(src, s)
		}
	}
	return src
}

// HasOwnPhotos báo job có ảnh tải lên để dựng không cần key.
func (p AffiliateParams) HasOwnPhotos() bool { return len(ownSources(p)) > 0 }

// ownPhotoSet dựng bộ khung từ ảnh thật của sản phẩm. Ít ảnh (1–2) thì mỗi ảnh cắt
// 4 khung cho đủ nhịp; từ 3 ảnh trở lên lấy 2 khung mỗi ảnh — video là nhiều góc
// ảnh thật khác nhau.
func ownPhotoSet(ctx context.Context, p AffiliateParams, work string) []string {
	src := ownSources(p)
	crops := 4
	if len(src) >= 3 {
		crops = 2
	}
	var photos []string
	for i, s := range src {
		photos = append(photos, derivePhotoVariants(ctx, s, work, fmt.Sprintf("own%d", i), crops)...)
	}
	return photos
}
