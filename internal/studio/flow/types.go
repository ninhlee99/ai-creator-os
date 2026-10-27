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
	PortTypeTextList  PortType = "text_list"
	PortTypeVideo     PortType = "video"
	PortTypeAudio     PortType = "audio"
	PortTypeText      PortType = "text"
	PortTypeProduct   PortType = "product"
	PortTypeMotion    PortType = "motion"
	PortTypeNumber    PortType = "number"
	PortTypeConfig    PortType = "config"
)

// NodeCategory phân loại nhóm node trên giao diện Canvas ComfyUI / Blender / n8n.
type NodeCategory string

const (
	// Phân loại cơ bản tương thích ngược
	CategoryInput   NodeCategory = "input"
	CategoryProcess NodeCategory = "process"
	CategoryAI      NodeCategory = "ai"
	CategoryOutput  NodeCategory = "output"

	CategoryCamera   NodeCategory = "camera"
	CategoryLighting NodeCategory = "lighting"
	CategoryRender   NodeCategory = "render"   // Remote video generation providers
	CategoryPost     NodeCategory = "post"     // FFmpeg finishing operations
	CategoryWorkflow NodeCategory = "workflow" // Workflow inputs, outputs and reusable subflows
	CategoryCustom   NodeCategory = "custom"   // Universal custom nodes (Video, Image, Audio, AI, Webhook)
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
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Category NodeCategory   `json:"category"`
	X        float64        `json:"x"`
	Y        float64        `json:"y"`
	Width    float64        `json:"width,omitempty"`
	Height   float64        `json:"height,omitempty"`
	Inputs   []FlowPort     `json:"inputs"`
	Outputs  []FlowPort     `json:"outputs"`
	Params   map[string]any `json:"params"`
	State    NodeState      `json:"state,omitempty"`
}

// NodeState biểu diễn trạng thái thực thi hiện tại của một node.
type NodeState struct {
	Status        string         `json:"status"` // idle, queued, running, completed, failed, skipped
	Progress      int            `json:"progress"`
	Error         string         `json:"error,omitempty"`
	OutputPreview string         `json:"output_preview,omitempty"`
	Outputs       map[string]any `json:"outputs,omitempty"`
	StartedAt     string         `json:"started_at,omitempty"`
	FinishedAt    string         `json:"finished_at,omitempty"`
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
	Type          string         `json:"type"`
	Title         string         `json:"title"`
	Category      NodeCategory   `json:"category"`
	Description   string         `json:"description"`
	Icon          string         `json:"icon"`
	Inputs        []FlowPort     `json:"inputs"`
	Outputs       []FlowPort     `json:"outputs"`
	DefaultParams map[string]any `json:"default_params"`
	Hidden        bool           `json:"hidden,omitempty"`
	Executor      NodeExecutor   `json:"-"`
}

// ExecutionContext chứa dữ liệu môi trường và các hàm callback khi chạy node.
type ExecutionContext struct {
	Context           context.Context
	RunID             string
	WorkDir           string
	OutDir            string
	AccountID         int
	MuseThreadStarted bool // Đánh dấu thread mới đã được mở ở node đầu tiên chưa
	Log               func(msg string)
	NodeStatus        func(nodeID string, state NodeState)
	EngineCall        func(ctx context.Context, platform string, prompt string, images []string, aspect string, outPath string) (string, error)
	MuseRender        func(ctx context.Context, isNewThread bool, threadTitle, prompt string, files []string, outPath string) error
	MediaGen          any
	VisionEval        func(ctx context.Context, prompt string, imagePath string) (string, error)
	AnalyzeImages     func(ctx context.Context, prompt string, imagePaths []string) (string, error)
	GenerateText      func(ctx context.Context, systemPrompt, prompt string) (string, error)
	SynthesizeSpeech  func(ctx context.Context, text, voice string) ([]byte, error)
	GenerateImage     func(ctx context.Context, prompt string, imagePaths []string, outPath string) error
	CharacterPhotos   func(accountID int64) ([]string, error)
	FFmpegRunner      func(ctx context.Context, args ...string) error
	WorkflowInputs    map[string]any
	ResolveWorkflow   func(id string) (*FlowGraph, error)
	ResolveProduct    func(ctx context.Context, id int64) (FlowProduct, error)
	ResolveRadarVideo func(ctx context.Context, sourceURL string, requireDance bool) (FlowRadarVideo, error)
	WorkflowStack     []string
}

// FlowProduct contains approved catalog data available to product_input nodes.
type FlowProduct struct {
	ID                 int64
	Title              string
	ImageURLs          []string
	Price              float64
	Currency           string
	CommissionRate     float64
	CommissionAmount   float64
	CommissionCurrency string
	ShopName           string
	Category           string
	ProductURL         string
	ReviewStatus       string
}

// FlowRadarVideo contains source metadata verified against the durable Radar store.
type FlowRadarVideo struct {
	VideoID     string
	VideoURL    string
	Author      string
	SoundTitle  string
	CollectedAt string
}

// ExecutionServices connects domain data owned by the host application.
type ExecutionServices struct {
	ResolveProduct    func(ctx context.Context, id int64) (FlowProduct, error)
	ResolveRadarVideo func(ctx context.Context, sourceURL string, requireDance bool) (FlowRadarVideo, error)
}

// NodeExecutor là hàm thực thi một node dựa trên inputs đã tính toán từ các node trước.
type NodeExecutor func(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error)

// FlowRun ghi lại một lần thực thi của graph.
type FlowRun struct {
	ID          string               `json:"id"`
	GraphID     string               `json:"graph_id"`
	GraphName   string               `json:"graph_name"`
	Status      string               `json:"status"` // queued, running, completed, failed, cancelled
	Progress    int                  `json:"progress"`
	NodeStates  map[string]NodeState `json:"node_states"`
	Outputs     map[string]any       `json:"outputs"`
	FinalOutput string               `json:"final_output,omitempty"`
	Log         string               `json:"log"`
	CreatedAt   string               `json:"created_at"`
	FinishedAt  string               `json:"finished_at,omitempty"`
}

// FlowSummary tóm tắt nhanh để hiển thị danh sách graph.
type FlowSummary struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Category         string     `json:"category"`
	IsPreset         bool       `json:"is_preset"`
	NodeCount        int        `json:"node_count"`
	EdgeCount        int        `json:"edge_count"`
	WorkflowReusable bool       `json:"workflow_reusable"`
	WorkflowError    string     `json:"workflow_error,omitempty"`
	Inputs           []FlowPort `json:"inputs,omitempty"`
	Outputs          []FlowPort `json:"outputs,omitempty"`
	UpdatedAt        string     `json:"updated_at"`
}
