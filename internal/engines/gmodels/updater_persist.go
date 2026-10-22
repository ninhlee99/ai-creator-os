package gmodels

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

type persistedModels struct {
	UpdatedAt  time.Time         `json:"updated_at"`
	Discovered map[string]string `json:"discovered"`
}

const persistedFile = "discovered_models.json"

// LoadPersisted tải các model đã tự dò được lưu trước đó từ thư mục dataDir.
func LoadPersisted(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	p := filepath.Join(dataDir, persistedFile)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var pm persistedModels
	if err := json.Unmarshal(data, &pm); err != nil {
		return err
	}
	mu.Lock()
	for kStr, model := range pm.Discovered {
		if model == "" {
			continue
		}
		switch kStr {
		case "text":
			discovered[Text] = model
		case "image":
			discovered[Image] = model
		case "tts":
			discovered[TTS] = model
		case "video":
			discovered[Video] = model
		}
	}
	mu.Unlock()
	log.Printf("[gmodels] Đã nạp danh sách model đã cập nhật từ cache: Text=%s, Image=%s, TTS=%s, Video=%s",
		Resolve(Text), Resolve(Image), Resolve(TTS), Resolve(Video))
	return nil
}

// SavePersisted lưu danh sách model hiện tại vào dataDir.
func SavePersisted(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	mu.RLock()
	discCopy := make(map[string]string)
	for k, v := range discovered {
		if v != "" {
			switch k {
			case Text:
				discCopy["text"] = v
			case Image:
				discCopy["image"] = v
			case TTS:
				discCopy["tts"] = v
			case Video:
				discCopy["video"] = v
			}
		}
	}
	mu.RUnlock()

	pm := persistedModels{
		UpdatedAt:  time.Now(),
		Discovered: discCopy,
	}
	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(dataDir, persistedFile)
	return os.WriteFile(p, data, 0644)
}

// CheckAndUpdate quét Google API để tìm các model mới nhất và tự động nâng cấp nếu phát hiện bản mới hơn.
