package web

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	oneCommandMaxUploadBytes = 200 << 20
	oneCommandMaxMediaFiles  = 8
)

type productMediaUploads struct {
	photos []string
	clips  []string
}

func hasOneCommandProductMedia(r *http.Request) bool {
	if r.MultipartForm == nil {
		return false
	}
	return len(r.MultipartForm.File["product_media"]) > 0 || len(r.MultipartForm.File["product_photo"]) > 0
}

func saveOneCommandProductMedia(r *http.Request, dir string) (productMediaUploads, error) {
	var result productMediaUploads
	if r.MultipartForm == nil {
		return result, nil
	}
	files := r.MultipartForm.File["product_media"]
	legacy := r.MultipartForm.File["product_photo"]
	if len(files) > 0 && len(legacy) > 0 {
		return result, fmt.Errorf("chỉ dùng một ô chọn media")
	}
	if len(files) == 0 {
		photo, err := saveUpload(r, "product_photo", dir)
		if err != nil {
			return result, err
		}
		if photo != "" {
			result.photos = []string{photo}
		}
		return result, nil
	}
	if len(files) > oneCommandMaxMediaFiles {
		return result, fmt.Errorf("chọn tối đa %d tệp", oneCommandMaxMediaFiles)
	}

	kind := ""
	var total int64
	for _, header := range files {
		if header.Size <= 0 {
			continue
		}
		ext := strings.ToLower(filepath.Ext(header.Filename))
		fileKind := oneCommandMediaKind(ext)
		if fileKind == "" {
			return result, fmt.Errorf("định dạng %s không hỗ trợ", ext)
		}
		if kind != "" && fileKind != kind {
			return result, fmt.Errorf("chọn nhiều ảnh hoặc nhiều video, không trộn hai loại")
		}
		kind = fileKind
		limit := int64(50 << 20)
		if fileKind == "video" {
			limit = oneCommandMaxUploadBytes
		}
		if header.Size > limit {
			return result, fmt.Errorf("%s vượt giới hạn %d MB", header.Filename, limit>>20)
		}
		total += header.Size
		if total > oneCommandMaxUploadBytes {
			return result, fmt.Errorf("tổng dung lượng vượt 200 MB")
		}
	}
	if kind == "" {
		return result, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result, err
	}
	var saved []string
	cleanup := func(err error) (productMediaUploads, error) {
		for _, path := range saved {
			_ = os.Remove(path)
		}
		return productMediaUploads{}, err
	}
	for i, header := range files {
		if header.Size <= 0 {
			continue
		}
		file, err := header.Open()
		if err != nil {
			return cleanup(err)
		}
		ext := strings.ToLower(filepath.Ext(header.Filename))
		dst := filepath.Join(dir, fmt.Sprintf("%d-product-media-%d%s", time.Now().UnixNano(), i, ext))
		out, err := os.Create(dst)
		if err != nil {
			_ = file.Close()
			return cleanup(err)
		}
		written, copyErr := io.Copy(out, io.LimitReader(file, oneCommandMaxUploadBytes+1))
		closeErr := out.Close()
		_ = file.Close()
		if copyErr != nil {
			return cleanup(copyErr)
		}
		if closeErr != nil {
			return cleanup(closeErr)
		}
		if written != header.Size || written > oneCommandMaxUploadBytes {
			_ = os.Remove(dst)
			return cleanup(fmt.Errorf("tệp %s tải lên không đầy đủ", header.Filename))
		}
		saved = append(saved, dst)
		if kind == "image" {
			result.photos = append(result.photos, dst)
		} else {
			result.clips = append(result.clips, dst)
		}
	}
	return result, nil
}

func oneCommandMediaKind(ext string) string {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return "image"
	case ".mp4", ".mov", ".m4v", ".webm":
		return "video"
	default:
		return ""
	}
}
