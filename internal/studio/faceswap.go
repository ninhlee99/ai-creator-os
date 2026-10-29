package studio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FaceSwapEngineStatus lưu trữ trạng thái của engine Face Swap cục bộ.
type FaceSwapEngineStatus struct {
	Ready       bool   `json:"ready"`
	CanRun      bool   `json:"can_run"`
	ModelsReady bool   `json:"models_ready"`
	Python      string `json:"python"`
	OpenCV      string `json:"cv2"`
	ONNXRuntime string `json:"onnxruntime"`
	InsightFace string `json:"insightface"`
	ModelDir    string `json:"model_dir"`
	LastError   string `json:"last_error,omitempty"`
}

var (
	cachedFSStatus *FaceSwapEngineStatus
	cachedFSAt     time.Time
	fsStatusMu     sync.RWMutex
)

// findPython311 tìm đường dẫn binary Python 3.11 hoặc Python có hỗ trợ InsightFace/OpenCV.
func findPython311() string {
	candidates := []string{
		".venv/bin/python3",
		"data/venv/bin/python3",
		filepath.Join(os.Getenv("HOME"), ".local/bin/python3.11"),
		filepath.Join(os.Getenv("HOME"), ".local/bin/python3"),
		"/opt/homebrew/bin/python3.11",
		"/usr/local/bin/python3.11",
		"/opt/homebrew/bin/python3",
		"python3.11",
		"python3",
	}
	for _, c := range candidates {
		if filepath.IsAbs(c) && fileExists(c) {
			return c
		}
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return "python3"
}

// CheckFaceSwapEngine kiểm tra trạng thái hoạt động của engine Face Swap.
func CheckFaceSwapEngine(ctx context.Context) FaceSwapEngineStatus {
	fsStatusMu.RLock()
	if cachedFSStatus != nil && time.Since(cachedFSAt) < 30*time.Second {
		st := *cachedFSStatus
		fsStatusMu.RUnlock()
		return st
	}
	fsStatusMu.RUnlock()

	py := findPython311()
	script := "scripts/face_swapper.py"
	if !fileExists(script) {
		// Tìm script từ thư mục gốc
		script = filepath.Join(filepath.Dir(os.Args[0]), "scripts", "face_swapper.py")
		if !fileExists(script) {
			script = "scripts/face_swapper.py"
		}
	}

	cmd := exec.CommandContext(ctx, py, script, "--check")
	out, err := cmd.CombinedOutput()
	if err != nil {
		st := FaceSwapEngineStatus{
			Ready:     false,
			CanRun:    false,
			LastError: fmt.Sprintf("Lỗi chạy kiểm tra engine: %v (%s)", err, strings.TrimSpace(string(out))),
		}
		return st
	}

	var st FaceSwapEngineStatus
	if jerr := json.Unmarshal(out, &st); jerr != nil {
		st.LastError = fmt.Sprintf("Không phân tích được phản hồi JSON: %v", jerr)
		return st
	}

	fsStatusMu.Lock()
	cachedFSStatus = &st
	cachedFSAt = time.Now()
	fsStatusMu.Unlock()
	return st
}

// RunFaceSwap thực thi hoán đổi khuôn mặt nhân vật từ sourceFacePhoto vào targetVideo,
// xuất ra outVideo và cập nhật tiến trình qua callback.
// sourceFacePhoto có thể là 1 đường dẫn hoặc danh sách nhiều đường dẫn (phân tách bởi dấu phẩy)
// để engine tự động phân tích và chọn góc mặt chính diện / sắc nét nhất.
func RunFaceSwap(ctx context.Context, sourceFacePhoto, targetVideo, outVideo string, onProgress func(pct int, logMsg string)) error {
	return runFaceSwap(ctx, sourceFacePhoto, targetVideo, outVideo, false, onProgress)
}

// RunFaceSwapStrict requires the complete InsightFace model stack and never
// falls back to OpenCV face alignment. Use for production identity replacement.
func RunFaceSwapStrict(ctx context.Context, sourceFacePhoto, targetVideo, outVideo string, onProgress func(pct int, logMsg string)) error {
	status := CheckFaceSwapEngine(ctx)
	if !status.Ready || !status.ModelsReady {
		if status.LastError != "" {
			return fmt.Errorf("InsightFace production engine chưa sẵn sàng: %s", status.LastError)
		}
		return fmt.Errorf("InsightFace production engine hoặc model chưa sẵn sàng")
	}
	return runFaceSwap(ctx, sourceFacePhoto, targetVideo, outVideo, true, onProgress)
}

func runFaceSwap(ctx context.Context, sourceFacePhoto, targetVideo, outVideo string, strict bool, onProgress func(pct int, logMsg string)) error {
	subPaths := strings.Split(sourceFacePhoto, ",")
	var validPaths []string
	for _, sp := range subPaths {
		sp = strings.TrimSpace(sp)
		if fileExists(sp) {
			validPaths = append(validPaths, sp)
		}
	}
	if len(validPaths) == 0 {
		return fmt.Errorf("không tìm thấy bất kỳ ảnh khuôn mặt nhân vật hợp lệ nào: %s", sourceFacePhoto)
	}
	if !fileExists(targetVideo) {
		return fmt.Errorf("video chuyển động đích không tồn tại: %s", targetVideo)
	}

	py := findPython311()
	script := "scripts/face_swapper.py"

	_ = os.MkdirAll(filepath.Dir(outVideo), 0o755)

	sourceArg := strings.Join(validPaths, ",")
	args := []string{script, "--source", sourceArg, "--target", targetVideo, "--output", outVideo}
	if strict {
		args = append(args, "--mode", "insightface", "--strict")
	}
	cmd := exec.CommandContext(ctx, py, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("lỗi khởi tạo stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // hợp nhất stderr vào stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("lỗi khởi chạy script face_swapper: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "PROGRESS: ") {
			pctStr := strings.TrimPrefix(line, "PROGRESS: ")
			if pct, perr := strconv.Atoi(pctStr); perr == nil && onProgress != nil {
				onProgress(pct, "")
			}
		} else if strings.HasPrefix(line, "LOG: ") {
			msg := strings.TrimPrefix(line, "LOG: ")
			if onProgress != nil {
				onProgress(-1, msg)
			}
		} else if strings.HasPrefix(line, "LỖI") {
			if onProgress != nil {
				onProgress(-1, line)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("tiến trình face_swapper kết thúc với lỗi: %w", err)
	}

	if !fileExists(outVideo) {
		return fmt.Errorf("không tìm thấy file kết quả sau khi face swap: %s", outVideo)
	}

	return nil
}
