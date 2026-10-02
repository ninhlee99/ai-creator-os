package publishers

import (
	"log"
	"os"
	"path/filepath"
)

// tokenDir là thư mục chứa file OAuth token (tiktok_token_*.json,
// youtube_token_*.json). R2-W7: token nằm trong thư mục dữ liệu của app
// (<dataDir>/tokens), không còn rải ở thư mục làm việc hiện tại.
var tokenDir = "."

// SetTokenDir đặt thư mục token; app gọi một lần lúc khởi động với
// <dataDir>/tokens. Mặc định "." để tương thích test và hành vi cũ.
func SetTokenDir(dir string) { tokenDir = dir }

// TokenDir trả về thư mục token hiện tại (cho UI/log kiểm chứng).
func TokenDir() string { return tokenDir }

func tokenPath(name string) string { return filepath.Join(tokenDir, name) }

// MigrateTokensFromCWD di trú một lần các file token còn sót ở thư mục
// làm việc hiện tại vào thư mục token. Bỏ qua file đã có ở đích; ghi log
// mỗi file đã di trú. Không bao giờ ghi đè token hiện có.
func MigrateTokensFromCWD() {
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		log.Printf("publishers: thư mục token: %v", err)
		return
	}
	for _, pat := range []string{"tiktok_token_*.json", "youtube_token_*.json"} {
		matches, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		for _, src := range matches {
			dst := filepath.Join(tokenDir, filepath.Base(src))
			if _, err := os.Stat(dst); err == nil {
				continue // đích đã có — không ghi đè
			}
			if err := os.Rename(src, dst); err != nil {
				log.Printf("publishers: di trú token %s: %v", src, err)
				continue
			}
			log.Printf("publishers: đã di trú token %s -> %s", src, dst)
		}
	}
}
