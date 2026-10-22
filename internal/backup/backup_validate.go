package backup

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	var manifest, ledger bool
	for _, f := range zr.File {
		switch filepath.Clean(f.Name) {
		case ManifestName:
			manifest = true
		case "ledger.db":
			ledger = true
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
	if !manifest {
		return fmt.Errorf("thiếu %s — không phải bản sao lưu của aicos", ManifestName)
	}
	if !ledger {
		return fmt.Errorf("thiếu ledger.db trong bản sao lưu")
	}
	return nil
}

// Stage giải nén bản sao lưu đã kiểm chứng vào thư mục chờ và đặt cờ
// RESTORE_PENDING. Dữ liệu thật chưa bị đụng tới.
