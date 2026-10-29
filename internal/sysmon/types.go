package sysmon

import "time"

// SystemSnapshot lưu toàn bộ trạng thái tài nguyên hệ thống, tiến trình và node đang chạy.
type SystemSnapshot struct {
	Timestamp     time.Time        `json:"timestamp"`
	TotalRAMMB    uint64           `json:"total_ram_mb"`
	UsedRAMMB     uint64           `json:"used_ram_mb"`
	FreeRAMMB     uint64           `json:"free_ram_mb"`
	RAMPercent    float64          `json:"ram_percent"`
	RAMStatus     string           `json:"ram_status"` // "ok", "warn", "danger"

	CPUPercent    float64          `json:"cpu_percent"`
	CPUCores      int              `json:"cpu_cores"`
	CPULoad1m     float64          `json:"cpu_load_1m"`
	CPULoad5m     float64          `json:"cpu_load_5m"`
	CPUStatus     string           `json:"cpu_status"` // "ok", "warn", "danger"

	DiskTotalGB   float64          `json:"disk_total_gb"`
	DiskUsedGB    float64          `json:"disk_used_gb"`
	DiskFreeGB    float64          `json:"disk_free_gb"`
	DiskPercent   float64          `json:"disk_percent"`
	DiskStatus    string           `json:"disk_status"` // "ok", "warn", "danger"

	AppAllocMB    uint64           `json:"app_alloc_mb"`
	AppSysMB      uint64           `json:"app_sys_mb"`
	AppGoroutines int              `json:"app_goroutines"`
	AppPID        int              `json:"app_pid"`
	AppRSSMB      uint64           `json:"app_rss_mb"`

	ActiveNodes    []ActiveNodeInfo `json:"active_nodes"`
	HeavyProcesses []ProcessInfo   `json:"heavy_processes"`

	OverallStatus string           `json:"overall_status"` // "ok", "warn", "danger"
	StatusMessage string           `json:"status_message"`
}

// ActiveNodeInfo chứa thông tin về node workflow đang chạy thực tế trên máy.
type ActiveNodeInfo struct {
	RunID        string `json:"run_id"`
	WorkflowName string `json:"workflow_name"`
	NodeID       string `json:"node_id"`
	NodeTitle    string `json:"node_title"`
	NodeType     string `json:"node_type"`
	Category     string `json:"category"`
	StartedAt    string `json:"started_at"`
	ElapsedSec   int64  `json:"elapsed_sec"`
	ResourceTier string `json:"resource_tier"` // "danger", "warn", "normal"
	TierLabel    string `json:"tier_label"`
}

// ProcessInfo chứa thông tin về tiến trình thực thi media / AI nặng trên máy.
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	Command    string  `json:"command"`
	IsSelf     bool    `json:"is_self"`
	IsKillable bool    `json:"is_killable"`
}
