package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// checkDBIntegrity kiểm tra PRAGMA integrity_check cho 5 DB chính.
// DB nào hỏng → trả lỗi kèm hướng dẫn khôi phục từ backup (tránh crash-loop).
func checkDBIntegrity(dataDir string) error {
	dbs := []string{"ledger.db", "studio.db", "products.db", "accesstrade.db", "webai.db"}
	for _, name := range dbs {
		p := filepath.Join(dataDir, name)
		if !fileExists(p) {
			continue
		}
		db, err := sql.Open("sqlite", p+"?mode=ro")
		if err != nil {
			continue
		}
		var result string
		err = db.QueryRow("PRAGMA integrity_check").Scan(&result)
		db.Close()
		if err != nil || result != "ok" {
			backupDir := filepath.Join(dataDir, "backups")
			return fmt.Errorf("%s hỏng (integrity_check: %s). Khôi phục: vào Cài đặt → Hệ thống → Khôi phục từ file, chọn bản mới nhất trong %s, rồi restart app", name, result, backupDir)
		}
	}
	return nil
}
