package web

import (
	"os/exec"
	"runtime"
)

// runtimeTool là một công cụ ngoài mà app phụ thuộc (ffmpeg, runtime local).
type runtimeTool struct {
	Name  string // tên hiển thị
	Found bool   // có tìm thấy trong PATH không
	Path  string // đường dẫn đầy đủ (khi tìm thấy)
	Hint  string // bước tiếp theo khi chưa tìm thấy (≤1 dòng)
}

func probeBin(name, bin, hint string) runtimeTool {
	if p, err := exec.LookPath(bin); err == nil {
		return runtimeTool{Name: name, Found: true, Path: p}
	}
	return runtimeTool{Name: name, Found: false, Hint: hint}
}

// installHint trả hướng dẫn cài theo hệ điều hành đang chạy app (Đợt T):
// trước đây mọi hint đều ghi "brew install" dù app chạy trên Windows/Linux.
func installHint(darwin, windows, linux string) string {
	switch runtime.GOOS {
	case "windows":
		return windows
	case "linux":
		return linux
	default:
		return darwin
	}
}

// probeRuntimeTools kiểm tra thật các công cụ ngoài (R2-W7, R2-07): có hay
// không + đường dẫn, không tuyên bố "sẵn sàng" khi chưa tìm thấy.
func probeRuntimeTools() []runtimeTool {
	return []runtimeTool{
		probeBin("FFmpeg", "ffmpeg", "Cần cho dựng video. "+installHint(
			"brew install ffmpeg",
			"winget install Gyan.FFmpeg",
			"sudo apt install ffmpeg")),
		probeBin("llama-server (LLM local)", "llama-server", "Cần cho LLM chạy trên máy. "+installHint(
			"brew install llama.cpp",
			"tải bản build tại github.com/ggerganov/llama.cpp/releases",
			"xem hướng dẫn build tại github.com/ggerganov/llama.cpp")),
		probeBin("uv (giọng đọc VieNeu)", "uv", "Cần cho giọng đọc VieNeu local. "+installHint(
			"brew install uv",
			"winget install astral-sh.uv",
			"curl -LsSf astral.sh/uv/install.sh | sh")),
		probeBin("Docker (VieNeu dự phòng)", "docker", "Không bắt buộc — VieNeu ưu tiên chạy bằng uv."),
	}
}
