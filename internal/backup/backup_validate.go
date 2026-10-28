package backup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ValidateZipFile(path string) error {
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
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("mở zip: %w", err)
	}
	return validateZipReader(zr)
}

// validateZipReader kiểm tra cấu trúc zip + integrity_check ledger.db
// đã giải nén ra file tạm.
func validateZipReader(zr *zip.Reader) error {
	if err := Validate(zr); err != nil {
		return err
	}
	// Giải nén ledger.db ra file tạm rồi integrity_check.
	var lf *zip.File
	for _, f := range zr.File {
		if filepath.Clean(f.Name) == "ledger.db" {
			lf = f
			break
		}
	}
	if lf == nil {
		return fmt.Errorf("thiếu ledger.db trong bản sao lưu")
	}
	rc, err := lf.Open()
	if err != nil {
		return fmt.Errorf("mở ledger.db trong zip: %w", err)
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "aicos-ledger-check-*.db")
	if err != nil {
		return fmt.Errorf("tạo file tạm: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return fmt.Errorf("giải nén ledger.db: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("đóng file tạm: %w", err)
	}
	if err := IntegrityCheck(tmpName); err != nil {
		return fmt.Errorf("ledger.db trong bản sao lưu: %w", err)
	}
	return nil
}
func Validate(zr *zip.Reader) error {
	var manifestFile *zip.File
	manifestCount, ledgerCount := 0, 0
	for _, f := range zr.File {
		switch filepath.Clean(f.Name) {
		case ManifestName:
			manifestFile = f
			manifestCount++
		case "ledger.db":
			ledgerCount++
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("mở ledger.db trong zip: %w", err)
			}
			head := make([]byte, 16)
			n, _ := io.ReadFull(rc, head)
			rc.Close()
			if n < 16 || !bytes.Equal(head, sqliteMagic) {
				return fmt.Errorf("ledger.db trong bản sao lưu không phải file SQLite")
			}
		}
		// Chặn zip-slip: từ chối entry thoát khỏi thư mục đích.
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("entry đáng ngờ trong bản sao lưu: %s", f.Name)
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
