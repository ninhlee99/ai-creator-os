package web

import "os/exec"

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

// probeRuntimeTools kiểm tra thật các công cụ ngoài (R2-W7, R2-07): có hay
// không + đường dẫn, không tuyên bố "sẵn sàng" khi chưa tìm thấy.
func probeRuntimeTools() []runtimeTool {
	return []runtimeTool{
		probeBin("FFmpeg", "ffmpeg", "Cần cho dựng video. Cài: brew install ffmpeg"),
		probeBin("llama-server (LLM local)", "llama-server", "Cần cho LLM chạy trên máy. Cài: brew install llama.cpp"),
		probeBin("uv (giọng đọc VieNeu)", "uv", "Cần cho giọng đọc VieNeu local. Cài: brew install uv"),
		probeBin("Docker (VieNeu dự phòng)", "docker", "Không bắt buộc — VieNeu ưu tiên chạy bằng uv."),
	}
}
