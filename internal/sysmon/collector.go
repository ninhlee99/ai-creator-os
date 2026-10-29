package sysmon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	cachedTotalRAMMB uint64
	totalRAMOnce     sync.Once
)

// GetTotalRAMMB trả về tổng dung lượng RAM vật lý của hệ thống (MB).
func GetTotalRAMMB() uint64 {
	totalRAMOnce.Do(func() {
		if runtime.GOOS == "darwin" {
			out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
			if err == nil {
				if bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
					cachedTotalRAMMB = bytes / (1024 * 1024)
				}
			}
		} else if runtime.GOOS == "linux" {
			data, err := os.ReadFile("/proc/meminfo")
			if err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(line, "MemTotal:") {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
								cachedTotalRAMMB = kb / 1024
							}
						}
						break
					}
				}
			}
		}
		if cachedTotalRAMMB == 0 {
			cachedTotalRAMMB = 16384 // Mặc định 16GB nếu không đọc được
		}
	})
	return cachedTotalRAMMB
}

// ReadMemoryUsage đọc RAM đã dùng và RAM còn trống của hệ thống (MB).
func ReadMemoryUsage(totalMB uint64) (usedMB, freeMB uint64, percent float64) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("vm_stat").Output()
		if err == nil {
			var pagesActive, pagesWired, pagesCompressor, pagesFree, pagesInactive, pagesSpeculative, pagesPurgeable uint64
			pageSize := uint64(4096)
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "page size of") {
					parts := strings.Fields(line)
					for i, p := range parts {
						if p == "of" && i+1 < len(parts) {
							if sz, err := strconv.ParseUint(parts[i+1], 10, 64); err == nil && sz > 0 {
								pageSize = sz
							}
						}
					}
				}
				parts := strings.Split(line, ":")
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimRight(strings.TrimSpace(parts[1]), ".")
				num, _ := strconv.ParseUint(val, 10, 64)
				switch key {
				case "Pages active":
					pagesActive = num
				case "Pages wired down":
					pagesWired = num
				case "Pages occupied by compressor":
					pagesCompressor = num
				case "Pages free":
					pagesFree = num
				case "Pages inactive":
					pagesInactive = num
				case "Pages speculative":
					pagesSpeculative = num
				case "Pages purgeable":
					pagesPurgeable = num
				}
			}
			// Chuẩn Darwin (tương đồng Activity Monitor / htop):
			// RAM thực tế đang dùng = Active + Wired + Compressed (bỏ qua cache file inactive/purgeable).
			usedBytes := (pagesActive + pagesWired + pagesCompressor) * pageSize
			usedMB = usedBytes / (1024 * 1024)
			if totalMB > usedMB {
				freeMB = totalMB - usedMB
			} else {
				freeMB = (pagesFree + pagesInactive + pagesSpeculative + pagesPurgeable) * pageSize / (1024 * 1024)
			}
			if totalMB > 0 {
				percent = (float64(usedMB) / float64(totalMB)) * 100.0
			}
			return usedMB, freeMB, percent
		}
	} else if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			var memAvailKB uint64
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemAvailable:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						memAvailKB, _ = strconv.ParseUint(fields[1], 10, 64)
					}
					break
				}
			}
			freeMB = memAvailKB / 1024
			if totalMB > freeMB {
				usedMB = totalMB - freeMB
			}
			if totalMB > 0 {
				percent = (float64(usedMB) / float64(totalMB)) * 100.0
			}
			return usedMB, freeMB, percent
		}
	}

	// Fallback nếu không đọc được từ OS
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	usedMB = m.Sys / (1024 * 1024)
	if totalMB > usedMB {
		freeMB = totalMB - usedMB
	}
	if totalMB > 0 {
		percent = (float64(usedMB) / float64(totalMB)) * 100.0
	}
	return usedMB, freeMB, percent
}

// ReadCPULoad đọc tải CPU (load averages 1m, 5m) và % CPU tức thời thực tế.
func ReadCPULoad(cores int) (percent, load1m, load5m float64) {
	if cores <= 0 {
		cores = runtime.NumCPU()
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
		if err == nil {
			text := strings.Trim(strings.TrimSpace(string(out)), "{} ")
			fields := strings.Fields(text)
			if len(fields) >= 2 {
				load1m, _ = strconv.ParseFloat(fields[0], 64)
				load5m, _ = strconv.ParseFloat(fields[1], 64)
			}
		}
		// Đọc tổng % CPU tức thời của tất cả tiến trình trên máy
		if psOut, err := exec.Command("ps", "-A", "-o", "%cpu").Output(); err == nil {
			var totalCPU float64
			for _, line := range strings.Split(string(psOut), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || line == "%CPU" {
					continue
				}
				if v, err := strconv.ParseFloat(line, 64); err == nil {
					totalCPU += v
				}
			}
			if cores > 0 {
				percent = totalCPU / float64(cores)
			}
		} else if cores > 0 {
			percent = (load1m / float64(cores)) * 100.0
		}
		if percent > 100.0 {
			percent = 100.0
		}
		return percent, load1m, load5m
	} else if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/loadavg")
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 2 {
				load1m, _ = strconv.ParseFloat(fields[0], 64)
				load5m, _ = strconv.ParseFloat(fields[1], 64)
			}
		}
		if psOut, err := exec.Command("ps", "-A", "-o", "%cpu").Output(); err == nil {
			var totalCPU float64
			for _, line := range strings.Split(string(psOut), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || line == "%CPU" {
					continue
				}
				if v, err := strconv.ParseFloat(line, 64); err == nil {
					totalCPU += v
				}
			}
			if cores > 0 {
				percent = totalCPU / float64(cores)
			}
		} else if cores > 0 {
			percent = (load1m / float64(cores)) * 100.0
		}
		if percent > 100.0 {
			percent = 100.0
		}
		return percent, load1m, load5m
	}
	return 10.0, 1.0, 1.0
}

// ReadHeavyProcesses quét các tiến trình media, AI, crawler hoặc máy chủ trên OS.
func ReadHeavyProcesses() []ProcessInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ps", "-eo", "pid,%cpu,rss,comm,args")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	selfPID := os.Getpid()
	lines := strings.Split(string(out), "\n")
	var results []ProcessInfo

	targets := []string{
		"aicos", "ffmpeg", "ffprobe", "face_swapper", "ai_cutout",
		"python", "python3", "edge", "msedge", "chrome", "chromium", "yt-dlp",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cpuPct, _ := strconv.ParseFloat(fields[1], 64)
		rssKB, _ := strconv.ParseFloat(fields[2], 64)
		comm := fields[3]
		fullArgs := ""
		if len(fields) >= 5 {
			fullArgs = strings.Join(fields[4:], " ")
		}

		// Kiểm tra có khớp với danh sách tiến trình cần giám sát không
		isTarget := false
		lowerArgs := strings.ToLower(fullArgs)
		lowerComm := strings.ToLower(comm)

		for _, t := range targets {
			if strings.Contains(lowerComm, t) || strings.Contains(lowerArgs, t) {
				isTarget = true
				break
			}
		}

		if !isTarget {
			continue
		}

		// Bỏ qua chính lệnh ps / grep
		if strings.Contains(lowerArgs, "ps -eo") || strings.Contains(lowerArgs, "grep ") {
			continue
		}

		// Xác định vai trò của tiến trình
		role := identifyProcessRole(pid, selfPID, lowerComm, lowerArgs)

		// Lọc bớt các tiến trình Edge/Chrome thông thường (chỉ giữ headless / crawler của aicos)
		if strings.Contains(lowerComm, "edge") || strings.Contains(lowerComm, "chrome") ||
			strings.Contains(lowerArgs, "edge") || strings.Contains(lowerArgs, "chrome") {
			if !strings.Contains(lowerArgs, "aicos") && !strings.Contains(lowerArgs, "headless") &&
				!strings.Contains(lowerArgs, "crawler") && !strings.Contains(lowerArgs, "remote-debugging") {
				continue
			}
		}

		// Lọc bớt python hệ thống không liên quan
		if strings.Contains(lowerComm, "python") || strings.Contains(lowerArgs, "python") {
			if !strings.Contains(lowerArgs, "face_swapper") && !strings.Contains(lowerArgs, "ai_cutout") &&
				!strings.Contains(lowerArgs, "aicos") {
				continue
			}
		}

		// Lọc bỏ siri, spotlight, teams, knowledge-agent nếu lọt vào
		if strings.Contains(lowerArgs, "teams") || strings.Contains(lowerComm, "siriknowledge") ||
			strings.Contains(lowerComm, "spotlight") || strings.Contains(lowerComm, "knowledge-agent") {
			continue
		}

		memMB := rssKB / 1024.0
		results = append(results, ProcessInfo{
			PID:        pid,
			Name:       cleanProcessName(comm, fullArgs),
			Role:       role,
			CPUPercent: cpuPct,
			MemoryMB:   memMB,
			Command:    truncateString(fullArgs, 120),
			IsSelf:     pid == selfPID,
			IsKillable: pid != selfPID && pid > 1,
		})
	}

	// Sắp xếp theo RAM chiếm dụng giảm dần
	sort.Slice(results, func(i, j int) bool {
		return results[i].MemoryMB > results[j].MemoryMB
	})

	return results
}

func identifyProcessRole(pid, selfPID int, comm, args string) string {
	if pid == selfPID || strings.Contains(args, "cmd/aicos") || strings.Contains(comm, "aicos") {
		return "MiuNova AI Engine (Máy chủ chính)"
	}
	if strings.Contains(args, "face_swapper") {
		return "AI Face Swap (InsightFace / ONNX)"
	}
	if strings.Contains(args, "ai_cutout") {
		return "AI Bóc Tách Phông Nền (Cutout)"
	}
	if strings.Contains(comm, "ffmpeg") {
		return "FFmpeg Video Render / Remaster 2K"
	}
	if strings.Contains(comm, "ffprobe") {
		return "FFprobe Đọc Metadata Video"
	}
	if strings.Contains(args, "aicos-tt-crawler") || (strings.Contains(args, "headless") && strings.Contains(args, "remote-debugging")) {
		return "Trình Duyệt Headless (TikTok Crawler / WebAI)"
	}
	if strings.Contains(comm, "yt-dlp") || strings.Contains(args, "yt-dlp") {
		return "Bộ Tải Video (yt-dlp)"
	}
	return "Tiến trình phụ trợ"
}

func cleanProcessName(comm, args string) string {
	lowerArgs := strings.ToLower(args)
	if strings.Contains(lowerArgs, "cmd/aicos") || strings.Contains(comm, "aicos") {
		return "aicos (Server)"
	}
	if strings.Contains(lowerArgs, "face_swapper") {
		return "python3 (face_swapper)"
	}
	if strings.Contains(lowerArgs, "ai_cutout") {
		return "python3 (ai_cutout)"
	}
	if strings.Contains(lowerArgs, "aicos-tt-crawler") || strings.Contains(lowerArgs, "headless") {
		if strings.Contains(comm, "msedge") || strings.Contains(lowerArgs, "edge") {
			return "msedge (headless-crawler)"
		}
		return "chromium (headless)"
	}
	if strings.Contains(comm, "ffmpeg") {
		return "ffmpeg"
	}
	if strings.Contains(comm, "ffprobe") {
		return "ffprobe"
	}
	if strings.Contains(comm, "yt-dlp") {
		return "yt-dlp"
	}
	parts := strings.Split(comm, "/")
	return parts[len(parts)-1]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ClassifyNodeResource phân loại mức độ ngốn tài nguyên của từng loại node.
func ClassifyNodeResource(nodeType string) (tier, label string) {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "video_upscale", "face_swap", "faceswap":
		return "danger", "🔴 Tải CPU/RAM Rất Cao (FFmpeg 2K / AI Model)"
	case "muse_render", "neural_render", "ai_cutout", "image_composite":
		return "warn", "🟡 Tải Trung Bình (Xử lý đồ họa & Media)"
	case "tts", "audio_tts", "audio_mix", "prompt", "director":
		return "normal", "🟢 Tải Nhẹ (Audio / LLM API)"
	default:
		return "normal", "🟢 Tải Bình Thường"
	}
}

var (
	cacheMu        sync.RWMutex
	cachedSnapshot *SystemSnapshot
	cachedAt       time.Time
)

// CollectFast trả về snapshot gần nhất đã cache nếu còn mới (<3s), giúp render trang siêu nhanh.
func CollectFast() *SystemSnapshot {
	return Collect(nil)
}

// Collect thu thập toàn bộ ảnh chụp hệ thống, CPU, RAM, node và tiến trình.
// Có in-memory cache ngắn 800ms để tối ưu tài nguyên nhưng luôn đảm bảo realtime khi poll 2s.
func Collect(activeNodes []ActiveNodeInfo) *SystemSnapshot {
	cacheMu.RLock()
	if cachedSnapshot != nil && time.Since(cachedAt) < 800*time.Millisecond {
		// Tái sử dụng dữ liệu OS nặng từ cache, chỉ cập nhật danh sách activeNodes
		base := *cachedSnapshot
		cacheMu.RUnlock()

		base.Timestamp = time.Now()
		base.ActiveNodes = activeNodes
		if len(activeNodes) > 0 && base.OverallStatus == "ok" {
			base.OverallStatus = "warn"
			base.StatusMessage = fmt.Sprintf("⚡ ĐANG CHẠY: Có %d node workflow đang xử lý, hệ thống đang tiêu thụ tài nguyên.", len(activeNodes))
		}
		return &base
	}
	cacheMu.RUnlock()

	totalRAM := GetTotalRAMMB()
	usedRAM, freeRAM, ramPct := ReadMemoryUsage(totalRAM)
	cores := runtime.NumCPU()
	cpuPct, load1, load5 := ReadCPULoad(cores)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	processes := ReadHeavyProcesses()

	// Đọc RSS của process hiện tại
	var appRSSMB uint64
	for _, p := range processes {
		if p.IsSelf {
			appRSSMB = uint64(p.MemoryMB)
			break
		}
	}
	if appRSSMB == 0 {
		appRSSMB = m.Sys / (1024 * 1024)
	}

	ramStatus := "ok"
	if ramPct >= 85.0 {
		ramStatus = "danger"
	} else if ramPct >= 70.0 {
		ramStatus = "warn"
	}

	cpuStatus := "ok"
	if cpuPct >= 85.0 {
		cpuStatus = "danger"
	} else if cpuPct >= 70.0 {
		cpuStatus = "warn"
	}

	overallStatus := "ok"
	statusMsg := "✓ ỔN ĐỊNH: Tài nguyên máy tính an toàn, sẵn sàng xử lý workflow."

	if ramStatus == "danger" || cpuStatus == "danger" {
		overallStatus = "danger"
		statusMsg = "⚠️ NGUY HIỂM: Tải RAM/CPU đang ở mức báo động! Đang có nguy cơ cạn bộ nhớ gây treo máy."
	} else if ramStatus == "warn" || cpuStatus == "warn" || len(activeNodes) > 0 {
		overallStatus = "warn"
		if len(activeNodes) > 0 {
			statusMsg = fmt.Sprintf("⚡ ĐANG CHẠY: Có %d node workflow đang xử lý, hệ thống đang tiêu thụ tài nguyên.", len(activeNodes))
		} else {
			statusMsg = "⚡ TẢI CAO: Hệ thống đang sử dụng hơn 70% bộ nhớ, hãy chú ý khi chạy thêm tác vụ nặng."
		}
	}

	diskTotal, diskUsed, diskFree, diskPct, diskStatus := ReadDiskUsage(".")

	snap := &SystemSnapshot{
		Timestamp:      time.Now(),
		TotalRAMMB:     totalRAM,
		UsedRAMMB:      usedRAM,
		FreeRAMMB:      freeRAM,
		RAMPercent:     ramPct,
		RAMStatus:      ramStatus,
		CPUPercent:     cpuPct,
		CPUCores:       cores,
		CPULoad1m:      load1,
		CPULoad5m:      load5,
		CPUStatus:      cpuStatus,
		DiskTotalGB:    diskTotal,
		DiskUsedGB:     diskUsed,
		DiskFreeGB:     diskFree,
		DiskPercent:    diskPct,
		DiskStatus:     diskStatus,
		AppAllocMB:     m.Alloc / (1024 * 1024),
		AppSysMB:       m.Sys / (1024 * 1024),
		AppGoroutines:  runtime.NumGoroutine(),
		AppPID:         os.Getpid(),
		AppRSSMB:       appRSSMB,
		ActiveNodes:    activeNodes,
		HeavyProcesses: processes,
		OverallStatus:  overallStatus,
		StatusMessage:  statusMsg,
	}

	cacheMu.Lock()
	cachedSnapshot = snap
	cachedAt = time.Now()
	cacheMu.Unlock()

	return snap
}

// KillProcess chấm dứt một tiến trình phụ trợ nặng để giải phóng RAM cho hệ thống.
func KillProcess(pid int) error {
	if pid <= 1 || pid == os.Getpid() {
		return fmt.Errorf("không được phép chấm dứt tiến trình an toàn này (PID: %d)", pid)
	}

	// Xác thực PID thuộc danh sách tiến trình được phép quản lý
	processes := ReadHeavyProcesses()
	allowed := false
	for _, p := range processes {
		if p.PID == pid && p.IsKillable {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("tiến trình PID %d không nằm trong danh sách kiểm soát của Studio", pid)
	}

	// Gửi tín hiệu SIGTERM trước
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		// Thử SIGKILL nếu SIGTERM thất bại
		if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
			return fmt.Errorf("không thể kill tiến trình PID %d: %w", pid, killErr)
		}
	}

	return nil
}
