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

// saveUpload stores one multipart file into dir, returning its path.
func saveUpload(r *http.Request, field, dir string) (string, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return "", nil // field absent is fine
	}
	defer f.Close()
	if hdr.Size == 0 {
		return "", nil
	}
	// Đợt AC: báo rõ khi file quá lớn thay vì cắt im lặng 50MB.
	if hdr.Size > 50<<20 {
		return "", fmt.Errorf("file %s quá lớn (%.1fMB, tối đa 50MB)", field, float64(hdr.Size)/(1<<20))
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".mp3", ".m4a", ".wav", ".ogg":
	default:
		return "", fmt.Errorf("file %s: định dạng %s không hỗ trợ", field, ext)
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), field, ext)
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(f, 50<<20)); err != nil {
		out.Close()
		return "", err
	}
	out.Close()
	return dst, nil
}

// saveUploads lưu tối đa max ảnh từ 1 ô chọn nhiều file (chỉ nhận ảnh).
func saveUploads(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	var out []string
	for i, hdr := range r.MultipartForm.File[field] {
		if i >= max {
			break
		}
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > 50<<20 {
			return nil, fmt.Errorf("ảnh %s quá lớn (tối đa 50MB)", hdr.Filename)
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			return nil, fmt.Errorf("ảnh %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		f, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, fmt.Sprintf("%d-%s-%d%s", time.Now().UnixNano(), field, i, ext))
		out2, err := os.Create(dst)
		if err != nil {
			f.Close()
			return nil, err
		}
		_, err = io.Copy(out2, f)
		out2.Close()
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, nil
}

// saveMotionClips stores generated motion footage supplied by the operator.
// Affiliate video APIs stay disabled under the project's free-only policy.
func saveMotionClips(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	headers := r.MultipartForm.File[field]
	if len(headers) > max {
		return nil, fmt.Errorf("tối đa %d clip chuyển động mỗi video", max)
	}
	const maxClipBytes = 100 << 20
	const maxTotalBytes = 600 << 20
	var total int64
	paths := make([]string, 0, len(headers))
	for i, hdr := range headers {
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > maxClipBytes {
			return nil, fmt.Errorf("clip %s quá lớn (tối đa 100MB)", hdr.Filename)
		}
		total += hdr.Size
		if total > maxTotalBytes {
			return nil, fmt.Errorf("tổng dung lượng clip vượt 600MB")
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".mp4", ".mov", ".webm", ".mkv":
		default:
			return nil, fmt.Errorf("clip %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		in, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, fmt.Sprintf("%d-motion-%02d%s", time.Now().UnixNano(), i, ext))
		out, err := os.Create(path)
		if err != nil {
			in.Close()
			return nil, err
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, maxClipBytes+1))
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil || closeInErr != nil || closeOutErr != nil {
			_ = os.Remove(path)
			if copyErr != nil {
				return nil, copyErr
			}
			if closeInErr != nil {
				return nil, closeInErr
			}
			return nil, closeOutErr
		}
		if written > maxClipBytes || written != hdr.Size {
			_ = os.Remove(path)
			return nil, fmt.Errorf("clip %s bị cắt hoặc vượt giới hạn tải lên", hdr.Filename)
		}
		paths = append(paths, path)
	}
	return paths, nil
}
