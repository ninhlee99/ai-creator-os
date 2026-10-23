package web

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

const (
	bioProductImagePrefix    = "bio-upload:"
	bioProductImageMaxBytes  = 8 << 20
	bioProductImageMaxPixels = 12_000_000
)

var bioProductImageFilePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png)$`)

func bioProductImageMarker(file string) string {
	return bioProductImagePrefix + file
}

func parseBioProductImageMarker(marker string) (string, bool) {
	file, ok := strings.CutPrefix(marker, bioProductImagePrefix)
	if !ok || !bioProductImageFilePattern.MatchString(file) {
		return "", false
	}
	return file, true
}

func validBioProductImageFile(file string) bool {
	return bioProductImageFilePattern.MatchString(file)
}

func sanitizeBioProductImage(data []byte, format string) ([]byte, error) {
	switch format {
	case "jpeg":
		return stripBioJPEGMetadata(data)
	case "png":
		return stripBioPNGMetadata(data)
	default:
		return nil, fmt.Errorf("unsupported image format")
	}
}

func stripBioJPEGMetadata(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, fmt.Errorf("invalid JPEG header")
	}

	var out bytes.Buffer
	out.Write(data[:2])
	var orientation uint16
	littleEndian := true
	orientationWritten := false
	inScan := false
	ended := false
	for i := 2; i < len(data); {
		if inScan {
			scanStart := i
			for i < len(data) {
				if data[i] != 0xff {
					i++
					continue
				}
				markerStart := i
				codeIndex := i + 1
				for codeIndex < len(data) && data[codeIndex] == 0xff {
					codeIndex++
				}
				if codeIndex >= len(data) {
					return nil, fmt.Errorf("truncated JPEG scan")
				}
				code := data[codeIndex]
				if code == 0x00 || code >= 0xd0 && code <= 0xd7 {
					i = codeIndex + 1
					continue
				}
				out.Write(data[scanStart:markerStart])
				i = markerStart
				inScan = false
				break
			}
			if inScan {
				out.Write(data[scanStart:])
				break
			}
			continue
		}

		markerStart := i
		if data[i] != 0xff {
			return nil, fmt.Errorf("invalid JPEG marker")
		}
		codeIndex := i + 1
		for codeIndex < len(data) && data[codeIndex] == 0xff {
			codeIndex++
		}
		if codeIndex >= len(data) {
			return nil, fmt.Errorf("truncated JPEG marker")
		}
		code := data[codeIndex]
		if code == 0xd9 {
			out.Write(data[markerStart : codeIndex+1])
			ended = true
			break
		}
		if code == 0xd8 || code == 0x01 || code >= 0xd0 && code <= 0xd7 {
			out.Write(data[markerStart : codeIndex+1])
			i = codeIndex + 1
			continue
		}
		if codeIndex+2 >= len(data) {
			return nil, fmt.Errorf("truncated JPEG segment length")
		}
		segmentLength := int(binary.BigEndian.Uint16(data[codeIndex+1 : codeIndex+3]))
		if segmentLength < 2 {
			return nil, fmt.Errorf("invalid JPEG segment length")
		}
		segmentEnd := codeIndex + 1 + segmentLength
		if segmentEnd > len(data) {
			return nil, fmt.Errorf("truncated JPEG segment")
		}

		if code == 0xe1 && orientation == 0 {
			if found, little, ok := bioJPEGExifOrientation(data[codeIndex+3 : segmentEnd]); ok {
				orientation = found
				littleEndian = little
			}
		}
		if code == 0xda && orientation != 0 && !orientationWritten {
			out.Write(bioJPEGExifOrientationSegment(orientation, littleEndian))
			orientationWritten = true
		}

		if bioKeepJPEGSegment(code, data[codeIndex+3:segmentEnd]) {
			out.Write(data[markerStart:segmentEnd])
		}
		i = segmentEnd
		if code == 0xda {
			inScan = true
		}
	}
	if !ended {
		return nil, fmt.Errorf("JPEG has no end marker")
	}
	return out.Bytes(), nil
}

func bioKeepJPEGSegment(code byte, payload []byte) bool {
	switch {
	case code == 0xe0 || code == 0xee:
		return true // JFIF and Adobe color transform markers.
	case code == 0xe2:
		return bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00"))
	case code >= 0xe0 && code <= 0xef:
		return false
	case code == 0xfe:
		return false
	default:
		return true
	}
}

func bioJPEGExifOrientation(payload []byte) (uint16, bool, bool) {
	if !bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
		return 0, true, false
	}
	return bioTIFFOrientation(payload[6:])
}

func bioTIFFOrientation(data []byte) (uint16, bool, bool) {
	if len(data) < 8 {
		return 0, true, false
	}
	var little bool
	switch string(data[:2]) {
	case "II":
		little = true
	case "MM":
		little = false
	default:
		return 0, true, false
	}
	read16 := binary.BigEndian.Uint16
	read32 := binary.BigEndian.Uint32
	if little {
		read16 = binary.LittleEndian.Uint16
		read32 = binary.LittleEndian.Uint32
	}
	if read16(data[2:4]) != 42 {
		return 0, little, false
	}
	offset := uint64(read32(data[4:8]))
	if offset+2 > uint64(len(data)) {
		return 0, little, false
	}
	count := uint64(read16(data[offset : offset+2]))
	entriesStart := offset + 2
	if entriesStart+count*12 > uint64(len(data)) {
		return 0, little, false
	}
	for n := uint64(0); n < count; n++ {
		entry := data[entriesStart+n*12 : entriesStart+(n+1)*12]
		if read16(entry[:2]) != 0x0112 || read16(entry[2:4]) != 3 {
			continue
		}
		valueCount := uint64(read32(entry[4:8]))
		if valueCount == 0 {
			continue
		}
		valueOffset := entriesStart + n*12 + 8
		if valueCount > 1 {
			valueOffset = uint64(read32(entry[8:12]))
		}
		if valueOffset+2 > uint64(len(data)) {
			continue
		}
		orientation := read16(data[valueOffset : valueOffset+2])
		if orientation >= 1 && orientation <= 8 {
			return orientation, little, true
		}
	}
	return 0, little, false
}

func bioJPEGExifOrientationSegment(orientation uint16, little bool) []byte {
	tiff := bioTIFFOrientationData(orientation, little)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, 0, byte(len(payload) + 2)}
	segment[2] = byte((len(payload) + 2) >> 8)
	return append(segment, payload...)
}

func bioTIFFOrientationData(orientation uint16, little bool) []byte {
	data := make([]byte, 26)
	if little {
		copy(data[:2], "II")
		binary.LittleEndian.PutUint16(data[2:4], 42)
		binary.LittleEndian.PutUint32(data[4:8], 8)
		binary.LittleEndian.PutUint16(data[8:10], 1)
		binary.LittleEndian.PutUint16(data[10:12], 0x0112)
		binary.LittleEndian.PutUint16(data[12:14], 3)
		binary.LittleEndian.PutUint32(data[14:18], 1)
		binary.LittleEndian.PutUint16(data[18:20], orientation)
	} else {
		copy(data[:2], "MM")
		binary.BigEndian.PutUint16(data[2:4], 42)
		binary.BigEndian.PutUint32(data[4:8], 8)
		binary.BigEndian.PutUint16(data[8:10], 1)
		binary.BigEndian.PutUint16(data[10:12], 0x0112)
		binary.BigEndian.PutUint16(data[12:14], 3)
		binary.BigEndian.PutUint32(data[14:18], 1)
		binary.BigEndian.PutUint16(data[18:20], orientation)
	}
	return data
}

func stripBioPNGMetadata(data []byte) ([]byte, error) {
	const signature = "\x89PNG\r\n\x1a\n"
	if len(data) < len(signature) || string(data[:len(signature)]) != signature {
		return nil, fmt.Errorf("invalid PNG header")
	}
	var out bytes.Buffer
	out.WriteString(signature)
	ended := false
	for offset := len(signature); offset+12 <= len(data); {
		chunkLength := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		dataStart := uint64(offset + 8)
		chunkEnd := dataStart + chunkLength
		crcEnd := chunkEnd + 4
		if crcEnd > uint64(len(data)) {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		chunkType := string(data[offset+4 : offset+8])
		chunkData := data[dataStart:chunkEnd]
		switch chunkType {
		case "tEXt", "zTXt", "iTXt", "tIME":
			// Text, timestamps, and EXIF can carry author or location data.
		case "eXIf":
			if orientation, little, ok := bioTIFFOrientation(chunkData); ok {
				bioWritePNGChunk(&out, "eXIf", bioTIFFOrientationData(orientation, little))
			}
		default:
			out.Write(data[offset:crcEnd])
		}
		if chunkType == "IEND" {
			ended = true
			break
		}
		offset = int(crcEnd)
	}
	if !ended {
		return nil, fmt.Errorf("PNG has no end chunk")
	}
	return out.Bytes(), nil
}

func bioWritePNGChunk(out *bytes.Buffer, chunkType string, data []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	out.Write(length[:])
	out.WriteString(chunkType)
	out.Write(data)
	checksum := crc32.ChecksumIEEE(append([]byte(chunkType), data...))
	binary.BigEndian.PutUint32(length[:], checksum)
	out.Write(length[:])
}

func listingHasBioProductImage(listing products.BioListing, marker string) bool {
	for _, image := range listing.ImageURLs {
		if image == marker {
			return true
		}
	}
	return false
}

func (s *Server) bioProductImageRoot() (string, error) {
	if strings.TrimSpace(s.DataDir) == "" {
		return "", fmt.Errorf("Bio data directory is not configured")
	}
	root := filepath.Join(s.DataDir, "bio-product-images")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("Bio product image path is not a directory")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func (s *Server) bioProductImagePath(marker string) (string, error) {
	file, ok := parseBioProductImageMarker(marker)
	if !ok {
		return "", fmt.Errorf("invalid Bio product image marker")
	}
	root, err := s.bioProductImageRoot()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, file)
	if filepath.Dir(path) != root {
		return "", fmt.Errorf("Bio product image path escapes its directory")
	}
	return path, nil
}

func (s *Server) bioProductImageExists(marker string) bool {
	path, err := s.bioProductImagePath(marker)
	if err != nil {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}

func (s *Server) saveBioProductImage(header *multipart.FileHeader) (string, error) {
	if header == nil || header.Size <= 0 || header.Size > bioProductImageMaxBytes {
		return "", fmt.Errorf("Ảnh sản phẩm phải lớn hơn 0 và không quá 8 MB")
	}
	file, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("Không mở được ảnh sản phẩm: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, bioProductImageMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > bioProductImageMaxBytes {
		return "", fmt.Errorf("Không đọc được ảnh sản phẩm hoặc file vượt quá 8 MB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return "", fmt.Errorf("Chỉ nhận ảnh JPEG hoặc PNG hợp lệ")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 6000 || config.Height > 6000 ||
		int64(config.Width)*int64(config.Height) > bioProductImageMaxPixels {
		return "", fmt.Errorf("Kích thước ảnh vượt giới hạn 6000×6000 hoặc 12 triệu điểm ảnh")
	}
	if _, decodedFormat, err := image.Decode(bytes.NewReader(data)); err != nil || decodedFormat != format {
		return "", fmt.Errorf("Ảnh sản phẩm bị lỗi hoặc không đầy đủ")
	}
	data, err = sanitizeBioProductImage(data, format)
	if err != nil {
		return "", fmt.Errorf("Không thể loại metadata riêng tư khỏi ảnh sản phẩm: %w", err)
	}
	if len(data) == 0 || len(data) > bioProductImageMaxBytes {
		return "", fmt.Errorf("Ảnh sản phẩm sau khi loại metadata vượt giới hạn 8 MB")
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("Không tạo được mã ảnh: %w", err)
	}
	fileName := hex.EncodeToString(token[:]) + ext
	root, err := s.bioProductImageRoot()
	if err != nil {
		return "", fmt.Errorf("Không mở được thư mục ảnh Bio: %w", err)
	}
	path := filepath.Join(root, fileName)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("Không lưu được ảnh sản phẩm: %w", err)
	}
	if _, err := out.Write(data); err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("Không lưu được ảnh sản phẩm: %w", err)
	}
	return bioProductImageMarker(fileName), nil
}

func (s *Server) removeBioProductImage(marker string) error {
	path, err := s.bioProductImagePath(marker)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("Bio product image is not a regular file")
	}
	return os.Remove(path)
}

func (s *Server) serveBioProductImage(w http.ResponseWriter, r *http.Request, marker string, isPublic bool) {
	path, err := s.bioProductImagePath(marker)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType != "image/jpeg" && contentType != "image/png" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if isPublic {
		w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	http.ServeContent(w, r, filepath.Base(path), openedInfo.ModTime(), file)
}

func (s *Server) handleBioProductImage(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	file := r.PathValue("asset")
	marker := bioProductImageMarker(file)
	if err != nil || productID <= 0 || !s.bioProductImageExists(marker) || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	account, _, err := s.resolvePublicBioAccount(channel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	listing, err := s.Products.BioListing(account.ID, productID, true)
	if err != nil || !s.bioListingIsPublic(listing) || !listingHasBioProductImage(listing, marker) {
		http.NotFound(w, r)
		return
	}
	s.serveBioProductImage(w, r, marker, true)
}

func (s *Server) handleBioAdminProductImage(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("asset")
	marker := bioProductImageMarker(file)
	if !s.bioProductImageExists(marker) || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	used, err := s.Products.HasBioProductImage(marker)
	if err != nil || !used {
		http.NotFound(w, r)
		return
	}
	s.serveBioProductImage(w, r, marker, false)
}
