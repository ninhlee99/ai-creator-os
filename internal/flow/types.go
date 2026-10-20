package flow

import (
	"context"
	"sync"
	"time"
)

// DataType defines the type of data flowing through pins between nodes.
type DataType string

const (
	TypeAny    DataType = "any"
	TypeString DataType = "string"
	TypeNumber DataType = "number"
	TypeBool   DataType = "bool"
	TypeImage  DataType = "image"
	TypeVideo  DataType = "video"
	TypeAudio  DataType = "audio"
	TypeFace   DataType = "face_embedding"
	TypePose   DataType = "pose_motion"
	TypeJSON   DataType = "json"
	TypeList   DataType = "list"
)

// Pin represents an input or output connection point on a Node.
type Pin struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        DataType `json:"type"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
}

// NodeStatus tracks the lifecycle state of a Node during Graph execution.
type NodeStatus string

const (
	StatusPending   NodeStatus = "pending"
	StatusRunning   NodeStatus = "running"
	StatusCompleted NodeStatus = "completed"
	StatusFailed    NodeStatus = "failed"
	StatusFallback  NodeStatus = "fallback"
	StatusSkipped   NodeStatus = "skipped"
)

// Node represents a discrete execution block in the ComfyUI-like Graph.
type Node struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Label        string                 `json:"label"`
	Category     string                 `json:"category"` // input | adapter | local | post | qc
	Inputs       []Pin                  `json:"inputs"`
	Outputs      []Pin                  `json:"outputs"`
	Params       map[string]interface{} `json:"params"`
	Status       NodeStatus             `json:"status"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Warning      string                 `json:"warning,omitempty"`
	DurationMs   int64                  `json:"duration_ms,omitempty"`
	OutputValues map[string]interface{} `json:"output_values,omitempty"`
	PreviewURL   string                 `json:"preview_url,omitempty"`
	PreviewType  string                 `json:"preview_type,omitempty"` // image | video | audio | text | json
	PositionX    float64                `json:"pos_x,omitempty"`
	PositionY    float64                `json:"pos_y,omitempty"`
}

// Wire defines a directed data connection from an output pin to an input pin.
type Wire struct {
	FromNode string `json:"from_node"`
	FromPin  string `json:"from_pin"`
	ToNode   string `json:"to_node"`
	ToPin    string `json:"to_pin"`
}

// GraphStatus tracks the state of the entire DAG.
type GraphStatus string

const (
	GraphPending   GraphStatus = "pending"
	GraphRunning   GraphStatus = "running"
	GraphCompleted GraphStatus = "completed"
	GraphFailed    GraphStatus = "failed"
)

// Graph represents a full ComfyUI-like workflow diagram.
type Graph struct {
	ID             string                 `json:"id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	RecipeKey      string                 `json:"recipe_key,omitempty"`
	Prompt         string                 `json:"prompt,omitempty"`
	Nodes          []*Node                `json:"nodes"`
	Wires          []Wire                 `json:"wires"`
	Status         GraphStatus            `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	FinishedAt     time.Time              `json:"finished_at,omitempty"`
	FinalOutputURL string                 `json:"final_output_url,omitempty"`
	QCScore        int                    `json:"qc_score,omitempty"`
	QCVerdict      string                 `json:"qc_verdict,omitempty"`
	Log            []string               `json:"log"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	mu             sync.RWMutex
}

// AppendLog adds a log line thread-safely.
func (g *Graph) AppendLog(line string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Log = append(g.Log, line)
}

// GetNode finds a node by its ID.
func (g *Graph) GetNode(id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// NodeExecutor defines the execution contract for any node implementation.
type NodeExecutor interface {
	Execute(ctx context.Context, node *Node, inputs map[string]interface{}) (map[string]interface{}, error)
}

// NodeDef defines a registered node type with its metadata and factory/executor.
type NodeDef struct {
	Type        string
	Label       string
	Category    string
	Description string
	Inputs      []Pin
	Outputs     []Pin
	DefaultData map[string]interface{}
	Executor    NodeExecutor
}
