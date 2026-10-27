package flow

import (
	"context"
)

// PortType định nghĩa kiểu dữ liệu truyền giữa các cổng của node.
type PortType string

const (
	PortTypeAny       PortType = "any"
	PortTypeImage     PortType = "image"
	PortTypeImageList PortType = "image_list"
	PortTypeVideo     PortType = "video"
	PortTypeAudio     PortType = "audio"
	PortTypeText      PortType = "text"
	PortTypeProduct   PortType = "product"
	PortTypeMotion    PortType = "motion"
	PortTypeNumber    PortType = "number"
	PortTypeConfig    PortType = "config"
	// 3D Studio & VFX Ports:
	PortTypeMesh      PortType = "mesh"    // Dữ liệu lưới 3D, Face Landmark, UV Mesh
	PortTypePose      PortType = "pose"    // Dữ liệu khung xương 3D / DensePose
	PortTypeCamera    PortType = "camera"  // Ma trận quang học máy quay (Focal length, Aperture, Matrix)
	PortTypeLight     PortType = "light"   // Thiết lập dàn đèn 3-Point Light Rig / HDR
	PortTypeTexture   PortType = "texture" // Bề mặt chất liệu vải, Normal/Depth Map
)

// NodeCategory phân loại nhóm node trên giao diện Canvas ComfyUI / Blender / n8n.
type NodeCategory string

const (
	// Phân loại cơ bản tương thích ngược
	CategoryInput   NodeCategory = "input"
	CategoryProcess NodeCategory = "process"
	CategoryAI      NodeCategory = "ai"
	CategoryOutput  NodeCategory = "output"

	// 6 Tầng Kiến trúc Studio 3D & VFX Chuyên nghiệp:
	CategoryIdentity NodeCategory = "identity" // Avatar & Digital Human (Khuôn mặt, FaceID, Vóc dáng)
	CategoryGeometry NodeCategory = "geometry" // 3D Mesh, Khung xương DensePose & Biến dạng vải
	CategoryCamera   NodeCategory = "camera"   // Virtual Cinematography (Máy quay quang học & Điều phối góc)
	CategoryLighting NodeCategory = "lighting" // 3D Studio Environment (3-Point Lighting, HDR Relighting)
	CategoryRender   NodeCategory = "render"   // Neural Diffusion & Shader Engines (Muse AI, Veo, Shaders)
	CategoryPost     NodeCategory = "post"     // VFX Compositing, Color Grade & Master Timeline
	CategoryWorkflow NodeCategory = "workflow" // Workflow inputs, outputs and reusable subflows
)

// FlowPort đại diện cho một cổng vào hoặc ra của node.
type FlowPort struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        PortType `json:"type"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

// FlowNode đại diện cho một node trên đồ họa.
type FlowNode struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Category    NodeCategory   `json:"category"`
	X           float64        `json:"x"`
	Y           float64        `json:"y"`
	Width       float64        `json:"width,omitempty"`
	Height      float64        `json:"height,omitempty"`
	Inputs      []FlowPort     `json:"inputs"`
	Outputs     []FlowPort     `json:"outputs"`
	Params      map[string]any `json:"params"`
	State       NodeState      `json:"state,omitempty"`
}

// NodeState biểu diễn trạng thái thực thi hiện tại của một node.
type NodeState struct {
	Status        string `json:"status"` // idle, queued, running, completed, failed, skipped
	Progress      int    `json:"progress"`
	Error         string `json:"error,omitempty"`
	OutputPreview string `json:"output_preview,omitempty"`
	MuseSessionID string `json:"muse_session_id,omitempty"` // ID phiên chat riêng biệt trên Muse AI
	MuseThreadURL string `json:"muse_thread_url,omitempty"` // URL xem trực tiếp phiên chat trên Muse Web
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

// FlowEdge đại diện cho kết nối dây giữa 2 cổng node.
type FlowEdge struct {
	ID       string `json:"id"`
	FromNode string `json:"from_node"`
	FromPort string `json:"from_port"`
	ToNode   string `json:"to_node"`
	ToPort   string `json:"to_port"`
}

// FlowGraph là đồ họa hoàn chỉnh chứa các node và edges.
type FlowGraph struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Category    string     `json:"category"` // viral, affiliate, custom
	IsPreset    bool       `json:"is_preset"`
	Nodes       []FlowNode `json:"nodes"`
	Edges       []FlowEdge `json:"edges"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// NodeDefinition định nghĩa mẫu sẵn có của 1 loại node trong registry.
type NodeDefinition struct {
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Category    NodeCategory   `json:"category"`
	Description string         `json:"description"`
	Icon        string         `json:"icon"`
	Inputs      []FlowPort     `json:"inputs"`
	Outputs     []FlowPort     `json:"outputs"`
	DefaultParams map[string]any `json:"default_params"`
	Executor    NodeExecutor   `json:"-"`
}

// ExecutionContext chứa dữ liệu môi trường và các hàm callback khi chạy node.
type ExecutionContext struct {
	Context           context.Context
	RunID             string
	WorkDir           string
	OutDir            string
	AccountID         int
	MuseThreadID      string // ID Thread Muse AI duy nhất cho toàn bộ flow run này (1 Flow = 1 Thread)
	MuseThreadURL     string // URL xem trực tiếp Thread trên Muse Web
	MuseThreadStarted bool   // Đánh dấu thread mới đã được mở ở node đầu tiên chưa
	Log               func(msg string)
	NodeStatus        func(nodeID string, state NodeState)
	EngineCall        func(ctx context.Context, platform string, prompt string, images []string, aspect string, outPath string) (string, error)
	MediaGen          any
	VisionEval        func(ctx context.Context, prompt string, imagePath string) (string, error)
	FFmpegRunner      func(ctx context.Context, args ...string) error
	WorkflowInputs    map[string]any
	ResolveWorkflow   func(id string) (*FlowGraph, error)
	WorkflowStack     []string
}

// NodeExecutor là hàm thực thi một node dựa trên inputs đã tính toán từ các node trước.
type NodeExecutor func(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error)

// FlowRun ghi lại một lần thực thi của graph.
type FlowRun struct {
	ID            string               `json:"id"`
	GraphID       string               `json:"graph_id"`
	GraphName     string               `json:"graph_name"`
	Status        string               `json:"status"` // queued, running, completed, failed, cancelled
	Progress      int                  `json:"progress"`
	MuseThreadID  string               `json:"muse_thread_id,omitempty"`  // Thread Muse AI duy nhất của lần chạy flow này (1 Flow = 1 Thread)
	MuseThreadURL string               `json:"muse_thread_url,omitempty"` // URL xem trực tiếp Thread của flow
	NodeStates    map[string]NodeState `json:"node_states"`
	Outputs       map[string]any       `json:"outputs"`
	FinalOutput   string               `json:"final_output,omitempty"`
	Log           string               `json:"log"`
	CreatedAt     string               `json:"created_at"`
	FinishedAt    string               `json:"finished_at,omitempty"`
}

// FlowSummary tóm tắt nhanh để hiển thị danh sách graph.
type FlowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsPreset    bool   `json:"is_preset"`
	NodeCount   int    `json:"node_count"`
	EdgeCount   int    `json:"edge_count"`
	UpdatedAt   string `json:"updated_at"`
}
