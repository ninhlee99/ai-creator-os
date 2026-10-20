package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestAccountCreationWithPresetCharacterAndRandomDefault(t *testing.T) {
	s := newTestServer(t)

	// Ensure studio is wired
	st, err := studio.New(filepath.Join(s.DataDir, "studio.db"), nil, nil, nil, s.DataDir)
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	defer st.Close()
	s.Studio = st

	// Ensure presets directory has at least 1 mock character file for test
	presetsDir := filepath.Join(s.DataDir, "presets", "characters")
	if err := os.MkdirAll(presetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, pc := range DefaultPresetCharacters {
		testImgPath := filepath.Join(presetsDir, pc.File)
		if _, err := os.Stat(testImgPath); os.IsNotExist(err) {
			_ = os.WriteFile(testImgPath, []byte("fake-image-content-for-"+pc.Title), 0644)
		}
	}

	// 1. Tạo kênh không chọn nhân vật -> Hệ thống phải tự động Random 1 trong 10 nhân vật mẫu
	rec1 := postForm(t, s, "/accounts", url.Values{
		"username":   {"@channel_random_char"},
		"niche_hint": {"Thần tiên tỉ tỉ review"},
	})
	if rec1.Code != http.StatusSeeOther {
		t.Fatalf("POST /accounts code = %d, want 303", rec1.Code)
	}

	accts, err := s.Mgr.List()
	if err != nil || len(accts) == 0 {
		t.Fatalf("List accounts failed: %v", err)
	}
	id1 := accts[0].ID

	// Kiểm tra nhân vật đã được gán tự động vào ModelLibrary của kênh id1
	ml, err := s.Studio.ModelLibrary()
	if err != nil {
		t.Fatalf("ModelLibrary: %v", err)
	}
	photos1, err := ml.List(id1)
	if err != nil || len(photos1) == 0 {
		t.Fatalf("Kênh tạo mới để trống nhân vật nhưng không được gán nhân vật ngẫu nhiên: %v", err)
	}
	if photos1[0].Path == "" {
		t.Fatalf("Ảnh nhân vật rỗng")
	}

	// 2. Tạo kênh có chọn rõ nhân vật mẫu (vd char_01.jpg)
	rec2 := postForm(t, s, "/accounts", url.Values{
		"username":     {"@channel_specific_char"},
		"niche_hint":   {"Thời trang dạ hội"},
		"preset_model": {"char_01.jpg"},
	})
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("POST /accounts specific char code = %d, want 303", rec2.Code)
	}

	accts, _ = s.Mgr.List()
	var id2 int64
	for _, a := range accts {
		if a.Username == "channel_specific_char" {
			id2 = a.ID
			break
		}
	}
	if id2 == 0 {
		t.Fatalf("không tìm thấy kênh channel_specific_char")
	}

	photos2, err := ml.List(id2)
	if err != nil || len(photos2) == 0 {
		t.Fatalf("kênh chỉ định preset char không có ảnh: %v", err)
	}

	// 3. Kiểm tra trang danh sách kênh /accounts: phải render ảnh nhân vật và nút Xóa
	recAccounts := get(t, s, "/accounts")
	if recAccounts.Code != http.StatusOK {
		t.Fatalf("GET /accounts code = %d, want 200", recAccounts.Code)
	}
	body := recAccounts.Body.String()
	if !strings.Contains(body, "Nhân vật riêng") {
		t.Fatalf("GET /accounts thiếu badge Nhân vật riêng: %s", body)
	}
	if !strings.Contains(body, "Xóa kênh vĩnh viễn") {
		t.Fatalf("GET /accounts thiếu nút Xóa kênh: %s", body)
	}

	// 4. Kiểm tra trang /accounts/new có render lưới 10 nhân vật có sẵn
	recNew := get(t, s, "/accounts/new")
	if recNew.Code != http.StatusOK {
		t.Fatalf("GET /accounts/new code = %d, want 200", recNew.Code)
	}
	newBody := recNew.Body.String()
	if !strings.Contains(newBody, "Thần Tiên Tỉ Tỉ") {
		t.Fatalf("GET /accounts/new thiếu danh sách nhân vật mẫu: %s", newBody)
	}
	if !strings.Contains(newBody, "char_01.jpg") {
		t.Fatalf("GET /accounts/new thiếu file char_01.jpg: %s", newBody)
	}

	// 5. Kiểm tra xóa kênh POST /accounts/{id}/delete
	recDel := postForm(t, s, "/accounts/"+string(rune('0'+id2))+"/delete", url.Values{})
	if recDel.Code != http.StatusSeeOther {
		t.Fatalf("POST /accounts/{id}/delete code = %d, want 303", recDel.Code)
	}
}
