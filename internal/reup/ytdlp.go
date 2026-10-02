package reup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// yt-dlp sidecar manager.
//
// App KHÔNG bundle yt-dlp vào binary (giữ 1 binary Go duy nhất, không thêm
// runtime/cgo). Thay vào đó app tự quản lý yt-dlp theo đúng mẫu sidecar
// models của engines/local:
//
//   - tải 1 lần từ GitHub release vào <dataDir>/bin/ (Ensure)
//   - health check bằng --version (Check)
//   - phát hiện extractor gãy từ stderr → tự update một lần rồi thử lại
//     (IsExtractorBroken + Update)
//
// LƯU Ý đã kiểm chứng: yt-dlp gốc KHÔNG có flag --no-watermark — truyền
// flag không tồn tại sẽ làm mọi lượt tải lỗi "no such option", nên Download
// không truyền nó. Provenance watermark được ghi trung thực từng video ở
// download.go (via=ytdlp → "có thể có watermark").
// ---------------------------------------------------------------------------

// defaultReleaseBase là trang release GitHub của yt-dlp (asset tải trực tiếp).
const defaultReleaseBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"

// downloadTimeout là timeout cho mỗi lượt tải video / tải binary.
const downloadTimeout = 10 * time.Minute

// minBinaryBytes là kích thước tối thiểu chấp nhận cho binary tải về
// (chống link release hỏng trả về trang HTML nhỏ). Test hạ xuống được.
var minBinaryBytes = int64(1_000_000)

// Manager quản lý binary yt-dlp trong binDir.
type Manager struct {
	binDir string
	http   *http.Client
	// ReleaseBase cho phép trỏ sang server giả trong test.
	ReleaseBase string
}

// NewManager tạo manager với thư mục chứa binary (thường <dataDir>/bin).
func NewManager(binDir string) *Manager {
	return &Manager{
		binDir:      binDir,
		http:        &http.Client{Timeout: downloadTimeout},
		ReleaseBase: defaultReleaseBase,
	}
}

// assetName trả về tên asset GitHub release đúng OS/arch. Trả lỗi trung
// thực cho combo không được yt-dlp hỗ trợ (thay vì đoán).
func assetName() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		// yt-dlp_macos là binary universal (arm64 + x86_64).
		return "yt-dlp_macos", nil
	case "windows":
		return "yt-dlp.exe", nil
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "yt-dlp", nil
		case "arm64":
			return "yt-dlp_linux_aarch64", nil
		case "arm":
			return "yt-dlp_linux_armv7l", nil
		default:
			return "", fmt.Errorf("yt-dlp: không có binary cho linux/%s", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("yt-dlp: không hỗ trợ %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// BinaryPath là đường dẫn binary yt-dlp trong binDir.
func (m *Manager) BinaryPath() (string, error) {
	name, err := assetName()
	if err != nil {
		return "", err
	}
	return filepath.Join(m.binDir, name), nil
}

// Installed báo binary đã có và chạy được (--version thành công).
func (m *Manager) Installed() bool { return m.Check(context.Background()) == nil }

// Version chạy `<binary> --version`.
func (m *Manager) Version(ctx context.Context) (string, error) {
	bin, err := m.BinaryPath()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp: health check (--version) thất bại: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Check là health check: binary tồn tại + chạy được.
func (m *Manager) Check(ctx context.Context) error {
	bin, err := m.BinaryPath()
	if err != nil {
		return err
	}
	st, serr := os.Stat(bin)
	if serr != nil || st.Size() == 0 {
		return fmt.Errorf("yt-dlp: chưa tải binary (bấm \"Tải yt-dlp\" ở trang Reup)")
	}
	if _, verr := m.Version(ctx); verr != nil {
		return verr
	}
	return nil
}

// Ensure tải binary từ GitHub release nếu chưa có/chạy không được.
// Idempotent: đã có và health OK thì không tải lại.
func (m *Manager) Ensure(ctx context.Context) error {
	if err := m.Check(ctx); err == nil {
		return nil
	}
	return m.Update(ctx)
}

// Update tải lại binary mới nhất (dùng khi extractor gãy hoặc bấm tay).
func (m *Manager) Update(ctx context.Context) error {
	name, err := assetName()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.binDir, 0o755); err != nil {
		return fmt.Errorf("yt-dlp: tạo thư mục bin: %w", err)
	}
	url := strings.TrimSuffix(m.ReleaseBase, "/") + "/" + name
	tmp := filepath.Join(m.binDir, name+".part")
	if err := m.fetch(ctx, url, tmp); err != nil {
		return fmt.Errorf("yt-dlp: tải binary: %w", err)
	}
	st, err := os.Stat(tmp)
	if err != nil || st.Size() < minBinaryBytes {
		size := int64(-1)
		if err == nil {
			size = st.Size()
		}
		os.Remove(tmp)
		return fmt.Errorf("yt-dlp: file tải về quá nhỏ (%d bytes) — có thể link release đổi, thử lại sau", size)
	}
	dest := filepath.Join(m.binDir, name)
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("yt-dlp: lưu binary: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dest, 0o755); err != nil {
			return fmt.Errorf("yt-dlp: chmod +x: %w", err)
		}
	}
	ver, verr := m.Version(ctx)
	if verr != nil {
		return fmt.Errorf("yt-dlp: binary mới tải về chạy không được: %w", verr)
	}
	// Ghi version để UI hiện + debug khi extractor gãy.
	_ = os.WriteFile(filepath.Join(m.binDir, name+".version"), []byte(ver), 0o644)
	return nil
}

// fetch tải url vào tmpPath.
func (m *Manager) fetch(ctx context.Context, url, tmpPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

// DownloadResult là kết quả một lượt tải yt-dlp.
type DownloadResult struct {
	FilePath string // file video đã tải
	VideoID  string // id yt-dlp trích được (để dedupe)
	Title    string
	Duration float64
}

// Download tải 1 URL Douyin bằng yt-dlp. outDir phải tồn tại.
// Trả (result, extractorBroken, err): extractorBroken=true khi stderr cho
// thấy extractor gãy (Douyin đổi chữ ký) — caller nên Update() rồi thử lại.
func (m *Manager) Download(ctx context.Context, url, outDir string) (DownloadResult, bool, error) {
	var zero DownloadResult
	bin, err := m.BinaryPath()
	if err != nil {
		return zero, false, err
	}
	if st, serr := os.Stat(bin); serr != nil || st.Size() == 0 {
		return zero, false, fmt.Errorf("yt-dlp: chưa có binary — chạy Ensure() trước")
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	// --no-playlist: chỉ tải đúng 1 video. --print after_move:filepath để
	// biết file thật (yt-dlp tự chọn đuôi). Không truyền --no-watermark
	// (flag không tồn tại trong yt-dlp gốc — xem ghi chú đầu file).
	tmpl := filepath.Join(outDir, "%(id)s.%(ext)s")
	cmd := exec.CommandContext(ctx, bin,
		"--no-playlist",
		"--no-progress",
		"--print", "after_move:filepath",
		"--print", "id",
		"--print", "title",
		"--print", "duration",
		"-o", tmpl,
		url,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if IsExtractorBroken(errText) {
			return zero, true, fmt.Errorf("yt-dlp: extractor có thể đã gãy (Douyin đổi chữ ký): %s",
				firstLine(errText))
		}
		return zero, false, fmt.Errorf("yt-dlp: tải thất bại: %s", firstLine(errText))
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	// stdout: filepath, id, title, duration (mỗi --print một dòng).
	var res DownloadResult
	if len(lines) > 0 {
		res.FilePath = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		res.VideoID = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		res.Title = strings.TrimSpace(lines[2])
	}
	if len(lines) > 3 {
		fmt.Sscanf(strings.TrimSpace(lines[3]), "%f", &res.Duration)
	}
	if res.FilePath == "" {
		return zero, false, fmt.Errorf("yt-dlp: không xác định được file đầu ra")
	}
	if _, serr := os.Stat(res.FilePath); serr != nil {
		return zero, false, fmt.Errorf("yt-dlp: file đầu ra không tồn tại: %s", res.FilePath)
	}
	return res, false, nil
}

// IsExtractorBroken nhận diện stderr của extractor gãy (Douyin xoay chữ
// ký / fingerprint). Best effort — mẫu chữ lấy từ lỗi yt-dlp thường gặp.
func IsExtractorBroken(stderr string) bool {
	low := strings.ToLower(stderr)
	markers := []string{
		"unable to extract",
		"fresh cookies",
		"login required",
		"unsupported url",
		"signature",
		"x-gorgon",
		"x-khronos",
		"empty media response",
		"http error 403",
		"http error 401",
		"did not match any",
	}
	for _, mk := range markers {
		if strings.Contains(low, mk) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
