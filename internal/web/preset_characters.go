package web

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PresetCharacter đại diện cho 1 nhân vật đại diện có sẵn trong hệ thống.
type PresetCharacter struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// DefaultPresetCharacters là danh sách 10 nhân vật chuẩn chất lượng cao được lưu sẵn trong hệ thống.
var DefaultPresetCharacters = []PresetCharacter{
	{"char_01", "char_01.jpg", "Thần Tiên Tỉ Tỉ", "Thuần khiết, làn da trắng phát sáng, mắt hạnh trong veo", "/static/presets/char_01.jpg"},
	{"char_02", "char_02.jpg", "Tiểu Thư Lookbook", "Váy dạ hội & thời trang thanh lịch cao cấp", "/static/presets/char_02.jpg"},
	{"char_03", "char_03.jpg", "Street Style Chic", "Thời trang đường phố sành điệu, hiện đại", "/static/presets/char_03.jpg"},
	{"char_04", "char_04.jpg", "KOC Túi Xách & Phụ Kiện", "Phong cách sành điệu, phối đồ phụ kiện tinh tế", "/static/presets/char_04.jpg"},
	{"char_05", "char_05.jpg", "Boutique Luxury Walk", "Góc máy bước đi boutique sang trọng, tự tin", "/static/presets/char_05.jpg"},
	{"char_06", "char_06.jpg", "Cafe Chill Vibe", "Ánh sáng tự nhiên ấm áp, phong cách đời thường cuốn hút", "/static/presets/char_06.jpg"},
	{"char_07", "char_07.jpg", "Rooftop Hoàng Hôn", "Điện ảnh ngược sáng hoàng hôn, quyến rũ", "/static/presets/char_07.jpg"},
	{"char_08", "char_08.jpg", "Nụ Cười Tỏa Nắng", "Nét mặt rạng rỡ, năng động, tạo thiện cảm", "/static/presets/char_08.jpg"},
	{"char_09", "char_09.jpg", "Dạo Phố Hiện Đại", "Bước đi tự tin, phong thái KOC triệu view", "/static/presets/char_09.jpg"},
	{"char_10", "char_10.jpg", "Lookbook Trẻ Trung", "Lookbook trang phục trẻ trung, năng động, cá tính", "/static/presets/char_10.jpg"},
}

// RandomPresetCharacter trả về ngẫu nhiên 1 trong 10 nhân vật mẫu có sẵn.
func RandomPresetCharacter() PresetCharacter {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return DefaultPresetCharacters[r.Intn(len(DefaultPresetCharacters))]
}

// FindPresetCharacter tìm nhân vật theo ID hoặc tên file.
func FindPresetCharacter(idOrFile string) *PresetCharacter {
	idOrFile = strings.TrimSpace(idOrFile)
	if idOrFile == "" {
		return nil
	}
	for i := range DefaultPresetCharacters {
		c := &DefaultPresetCharacters[i]
		if c.ID == idOrFile || c.File == idOrFile || strings.EqualFold(c.Title, idOrFile) {
			return c
		}
	}
	return nil
}

// AssignCharacterToAccount gán ảnh nhân vật cho kênh (sao chép vào thư viện ModelLibrary của kênh đó).
func (s *Server) AssignCharacterToAccount(accountID int64, srcPath string) error {
	if s.Studio == nil {
		return fmt.Errorf("studio chưa sẵn sàng")
	}
	ms, err := s.Studio.ModelLibrary()
	if err != nil {
		return fmt.Errorf("model library: %w", err)
	}
	photo, err := ms.Add(accountID, srcPath)
	if err != nil {
		return fmt.Errorf("thêm ảnh nhân vật: %w", err)
	}
	log.Printf("[accounts] Đã gán nhân vật thành công cho kênh #%d: id=%d path=%s", accountID, photo.ID, photo.Path)
	return nil
}

// ResolvePresetPath tìm đường dẫn file vật lý của nhân vật mẫu trên đĩa.
func (s *Server) ResolvePresetPath(filename string) string {
	filename = filepath.Base(filename)
	candidates := []string{
		filepath.Join(s.DataDir, "presets", "characters", filename),
		filepath.Join("data", "presets", "characters", filename),
		filepath.Join("internal", "web", "static", "presets", filename),
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// handleStaticPresetImage phục vụ file ảnh nhân vật mẫu tĩnh cho web UI.
func (s *Server) handleStaticPresetImage(w http.ResponseWriter, r *http.Request) {
	file := filepath.Base(r.PathValue("file"))
	p := s.ResolvePresetPath(file)
	if p == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, p)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
