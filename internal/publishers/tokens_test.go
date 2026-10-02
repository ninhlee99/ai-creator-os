package publishers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTokenDirAndMigration — token nằm trong <dataDir>/tokens; file còn
// sót ở CWD được di trú một lần, không ghi đè file đã có ở đích.
func TestTokenDirAndMigration(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work) // giả lập CWD có token cũ
	tokDir := filepath.Join(t.TempDir(), "tokens")

	old := TokenDir()
	SetTokenDir(tokDir)
	t.Cleanup(func() { SetTokenDir(old) })

	legacy := "tiktok_token_shop_vn.json"
	if err := os.WriteFile(legacy, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// File đã có ở đích thì không bị ghi đè.
	if err := os.MkdirAll(tokDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(tokDir, "youtube_token_shop_vn.json")
	if err := os.WriteFile(keep, []byte(`{"refresh_token":"keep"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("youtube_token_shop_vn.json", []byte(`{"refresh_token":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	MigrateTokensFromCWD()

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("file token cũ chưa được di trú khỏi CWD")
	}
	if _, err := os.Stat(filepath.Join(tokDir, legacy)); err != nil {
		t.Fatal("file token chưa tới thư mục token mới")
	}
	if raw, _ := os.ReadFile(keep); string(raw) != `{"refresh_token":"keep"}` {
		t.Fatal("MigrateTokensFromCWD đã ghi đè token ở đích")
	}
	if got := TikTokTokenPath("shop_vn"); got != filepath.Join(tokDir, "tiktok_token_shop_vn.json") {
		t.Fatalf("TikTokTokenPath = %q, want trong %s", got, tokDir)
	}
	if got := YouTubeTokenPath("shop_vn"); got != filepath.Join(tokDir, "youtube_token_shop_vn.json") {
		t.Fatalf("YouTubeTokenPath = %q, want trong %s", got, tokDir)
	}
}
