package backup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

const maxBackupUncompressedBytes uint64 = 512 << 20

// MaxBackupArchiveBytes caps compressed archive size accepted by restore.
const MaxBackupArchiveBytes uint64 = 512 << 20

func ValidateZipFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("đọc thông tin zip: %w", err)
	}
	if uint64(info.Size()) > MaxBackupArchiveBytes {
		return fmt.Errorf("file sao lưu vượt giới hạn 512 MiB")
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("mở zip: %w", err)
	}
	defer zr.Close()
	return validateZipReader(&zr.Reader)
}

// ValidateZipBytes kiểm tra bytes zip trong bộ nhớ — cùng pipeline với
// ValidateZipFile, dùng cho nút "Tải bản sao lưu" bấm tay.
func ValidateZipBytes(raw []byte) error {
	if uint64(len(raw)) > MaxBackupArchiveBytes {
		return fmt.Errorf("file sao lưu vượt giới hạn 512 MiB")
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("mở zip: %w", err)
	}
	return validateZipReader(zr)
}

// validateZipReader kiểm tra cấu trúc zip + integrity_check mọi database
// đã giải nén ra file tạm.
func validateZipReader(zr *zip.Reader) error {
	if err := Validate(zr); err != nil {
		return err
	}
	for _, f := range zr.File {
		name, err := validatedBackupEntryName(f.Name)
		if err != nil || !isSnapshotDatabase(name) {
			continue
		}
		if err := validateSQLiteArchiveEntry(f, name); err != nil {
			return err
		}
	}
	return nil
}

func validateSQLiteArchiveEntry(file *zip.File, name string) error {
	if file.UncompressedSize64 > maxBackupUncompressedBytes {
		return fmt.Errorf("%s trong bản sao lưu vượt giới hạn 512 MiB", name)
	}
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("mở %s trong zip: %w", name, err)
	}
	tmp, err := os.CreateTemp("", "aicos-db-check-*.db")
	if err != nil {
		rc.Close()
		return fmt.Errorf("tạo file tạm: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	n, copyErr := io.Copy(tmp, io.LimitReader(rc, int64(maxBackupUncompressedBytes)+1))
	rc.Close()
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("giải nén %s: %w", name, copyErr)
	}
	if uint64(n) > maxBackupUncompressedBytes {
		return fmt.Errorf("%s trong bản sao lưu vượt giới hạn 512 MiB", name)
	}
	if closeErr != nil {
		return fmt.Errorf("đóng file tạm: %w", closeErr)
	}
	if err := IntegrityCheck(tmpName); err != nil {
		return fmt.Errorf("%s trong bản sao lưu: %w", name, err)
	}
	return nil
}

func Validate(zr *zip.Reader) error {
	if len(zr.File) > maxBackupArchiveEntries {
		return fmt.Errorf("bản sao lưu vượt giới hạn %d entry", maxBackupArchiveEntries)
	}
	var manifestFile *zip.File
	manifestCount, ledgerCount := 0, 0
	seenNames := make(map[string]struct{}, len(zr.File))
	var totalUncompressed uint64
	var totalCompressed uint64
	for _, f := range zr.File {
		name, err := validatedBackupEntryName(f.Name)
		if err != nil {
			return fmt.Errorf("entry không hợp lệ trong bản sao lưu: %q", f.Name)
		}
		key := strings.ToLower(name)
		if _, exists := seenNames[key]; exists {
			return fmt.Errorf("bản sao lưu có entry trùng hoặc xung đột tên: %q", f.Name)
		}
		seenNames[key] = struct{}{}
		if f.UncompressedSize64 > maxBackupUncompressedBytes || totalUncompressed > maxBackupUncompressedBytes-f.UncompressedSize64 {
			return fmt.Errorf("bản sao lưu vượt giới hạn giải nén 512 MiB")
		}
		totalUncompressed += f.UncompressedSize64
		if f.CompressedSize64 > MaxBackupArchiveBytes || totalCompressed > MaxBackupArchiveBytes-f.CompressedSize64 {
			return fmt.Errorf("bản sao lưu vượt giới hạn file nén 512 MiB")
		}
		totalCompressed += f.CompressedSize64
		switch name {
		case ManifestName:
			manifestFile = f
			manifestCount++
		case "ledger.db":
			ledgerCount++
		}
		if isSnapshotDatabase(name) {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("mở %s trong zip: %w", name, err)
			}
			head := make([]byte, 16)
			n, _ := io.ReadFull(rc, head)
			rc.Close()
			if n < 16 || !bytes.Equal(head, sqliteMagic) {
				return fmt.Errorf("%s trong bản sao lưu không phải file SQLite", name)
			}
		}
	}
	for _, f := range zr.File {
		name, _ := validatedBackupEntryName(f.Name)
		for _, suffix := range []string{"-wal", "-shm"} {
			if strings.HasSuffix(name, suffix) {
				databaseName := strings.TrimSuffix(name, suffix)
				if _, exists := seenNames[strings.ToLower(databaseName)]; !exists {
					return fmt.Errorf("%s trong bản sao lưu không có database đi kèm", name)
				}
			}
		}
	}
	if manifestCount == 0 {
		return fmt.Errorf("thiếu %s — không phải bản sao lưu của aicos", ManifestName)
	}
	if manifestCount != 1 {
		return fmt.Errorf("bản sao lưu có %d manifest; cần đúng một", manifestCount)
	}
	if ledgerCount == 0 {
		return fmt.Errorf("thiếu ledger.db trong bản sao lưu")
	}
	if ledgerCount != 1 {
		return fmt.Errorf("bản sao lưu có %d ledger.db; cần đúng một", ledgerCount)
	}
	if err := validateManifest(manifestFile); err != nil {
		return err
	}
	return nil
}

// validatedBackupEntryName accepts only paths emitted by Create. Archive
// paths use slash separators on every OS; rejecting volumes, absolute paths,
// alternate separators and unknown targets prevents restore from writing
// outside dataDir (or over unrelated files inside it) on Windows and Unix.
func validatedBackupEntryName(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || path.IsAbs(name) {
		return "", fmt.Errorf("absolute, volume or invalid path")
	}
	if path.Clean(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("non-canonical path")
	}
	for _, db := range snapshotDBs {
		if name == db || name == db+"-wal" || name == db+"-shm" {
			return name, nil
		}
	}
	for _, file := range snapshotFiles {
		if name == file {
			return name, nil
		}
	}
	for _, dir := range snapshotTreeDirs {
		prefix := dir + "/"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(name, prefix), "/")
		if (dir == "output/studio/uploads" || dir == "uploads") && len(parts) == 1 && validStudioUploadAsset(parts[0]) {
			return name, nil
		}
		if dir == "output/studio/models" && len(parts) == 2 && positiveAccountDirectory(parts[0]) && validModelPhotoName(parts[1]) {
			return name, nil
		}
	}
	if name == ManifestName {
		return name, nil
	}
	return "", fmt.Errorf("unknown backup target")
}

func isSnapshotDatabase(name string) bool {
	for _, db := range snapshotDBs {
		if name == db {
			return true
		}
	}
	return false
}

func validPortableFilename(name string) bool {
	if name == "" || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, "<>\"|?*") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$":
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func positiveAccountDirectory(name string) bool {
	if name == "" {
		return false
	}
	positive := false
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
		positive = positive || r != '0'
	}
	return positive
}

func validModelPhotoName(name string) bool {
	return validPortableFilename(name) && allowedAssetExtension(name, ".jpg", ".jpeg", ".png", ".webp")
}

func validStudioUploadAsset(name string) bool {
	return validPortableFilename(name) && allowedAssetExtension(name,
		".jpg", ".jpeg", ".png", ".webp", ".mp4", ".mov", ".webm", ".mkv", ".avi",
		".mp3", ".aac", ".wav", ".m4a", ".ogg", ".flac")
}

func allowedAssetExtension(name string, allowed ...string) bool {
	ext := strings.ToLower(path.Ext(name))
	for _, candidate := range allowed {
		if ext == candidate {
			return true
		}
	}
	return false
}

func validateManifest(file *zip.File) error {
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("mở %s: %w", ManifestName, err)
	}
	defer rc.Close()
	const maxManifestBytes = 64 << 10
	raw, err := io.ReadAll(io.LimitReader(rc, maxManifestBytes+1))
	if err != nil {
		return fmt.Errorf("đọc %s: %w", ManifestName, err)
	}
	if len(raw) > maxManifestBytes {
		return fmt.Errorf("%s vượt quá giới hạn kích thước", ManifestName)
	}
	var manifest struct {
		App     string `json:"app"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("%s không hợp lệ: %w", ManifestName, err)
	}
	if manifest.App != "aicos" {
		return fmt.Errorf("%s không phải bản sao lưu của aicos", ManifestName)
	}
	if _, err := time.Parse(time.RFC3339, manifest.Created); err != nil {
		return fmt.Errorf("%s thiếu thời điểm tạo hợp lệ", ManifestName)
	}
	return nil
}

// Stage giải nén bản sao lưu đã kiểm chứng vào thư mục chờ và đặt cờ
// RESTORE_PENDING. Dữ liệu thật chưa bị đụng tới.
