package flow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// GraphEventListener is notified whenever a node or graph status changes.
type GraphEventListener func(graph *Graph, node *Node)

// Executor manages execution of ComfyUI-like Graphs with dependency resolution.
type Executor struct {
	bridge    *AdaptersBridge
	mu        sync.RWMutex
	listeners []GraphEventListener
}

// NewExecutor creates a new Graph Executor backed by the AdaptersBridge.
func NewExecutor(bridge *AdaptersBridge) *Executor {
	return &Executor{
		bridge:    bridge,
		listeners: make([]GraphEventListener, 0),
	}
}

// AddListener registers an event listener for live execution updates.
func (e *Executor) AddListener(listener GraphEventListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, listener)
}

func (e *Executor) notify(g *Graph, n *Node) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, l := range e.listeners {
		l(g, n)
	}
}

// Execute runs the full DAG graph with graceful fallback and real-time state tracking.
func (e *Executor) Execute(ctx context.Context, g *Graph) error {
	g.Status = GraphRunning
	g.CreatedAt = time.Now()
	g.AppendLog(fmt.Sprintf("[%s] Bắt đầu thực thi Graph: %s (%s)", time.Now().Format("15:04:05"), g.Title, g.ID))
	e.notify(g, nil)

	// 1. Build adjacency and dependency maps
	nodeMap := make(map[string]*Node)
	for _, n := range g.Nodes {
		nodeMap[n.ID] = n
	}

	// In-degree: how many upstream nodes each node depends on
	dependencies := make(map[string]map[string]bool)
	dependents := make(map[string][]string) // node -> list of nodes waiting on it

	for _, n := range g.Nodes {
		dependencies[n.ID] = make(map[string]bool)
	}

	for _, wire := range g.Wires {
		if _, ok := nodeMap[wire.FromNode]; ok {
			if _, ok2 := nodeMap[wire.ToNode]; ok2 {
				dependencies[wire.ToNode][wire.FromNode] = true
				dependents[wire.FromNode] = append(dependents[wire.FromNode], wire.ToNode)
			}
		}
	}

	// 2. Validate DAG (Cycle Detection)
	if hasCycle(dependencies) {
		g.Status = GraphFailed
		err := fmt.Errorf("đồ thị chứa chu trình (cycle detected), không phải là DAG hợp lệ")
		g.AppendLog(fmt.Sprintf("[%s] Lỗi: %v", time.Now().Format("15:04:05"), err))
		e.notify(g, nil)
		return err
	}

	// 3. Execution Loop
	completed := make(map[string]bool)
	wireData := make(map[string]interface{}) // key: "fromNode:fromPin" -> value

	for len(completed) < len(g.Nodes) {
		select {
		case <-ctx.Done():
			g.Status = GraphFailed
			return ctx.Err()
		default:
		}

		// Find all ready nodes whose dependencies are all satisfied
		var readyNodes []*Node
		for _, n := range g.Nodes {
			if completed[n.ID] || n.Status == StatusRunning {
				continue
			}

			isReady := true
			for depID := range dependencies[n.ID] {
				if !completed[depID] {
					isReady = false
					break
				}
			}

			if isReady {
				readyNodes = append(readyNodes, n)
			}
		}

		if len(readyNodes) == 0 {
			// Deadlock check
			g.Status = GraphFailed
			err := fmt.Errorf("không thể tiếp tục thực thi: các node bị bế tắc do thiếu phụ thuộc")
			g.AppendLog(fmt.Sprintf("[%s] Lỗi: %v", time.Now().Format("15:04:05"), err))
			e.notify(g, nil)
			return err
		}

		// Execute ready nodes
		for _, node := range readyNodes {
			node.Status = StatusRunning
			g.AppendLog(fmt.Sprintf("[%s] Đang chạy Node [%s]: %s...", time.Now().Format("15:04:05"), node.Category, node.Label))
			e.notify(g, node)

			startTime := time.Now()

			// Gather inputs from incoming wires
			inputs := make(map[string]interface{})
			for _, wire := range g.Wires {
				if wire.ToNode == node.ID {
					key := fmt.Sprintf("%s:%s", wire.FromNode, wire.FromPin)
					if val, ok := wireData[key]; ok {
						inputs[wire.ToPin] = val
					}
				}
			}

			// Add node params into inputs
			for k, v := range node.Params {
				if _, exists := inputs[k]; !exists {
					inputs[k] = v
				}
			}

			// Execute through bridge
			outputs, err := e.bridge.ExecuteNode(ctx, node, inputs)
			duration := time.Since(startTime)
			node.DurationMs = duration.Milliseconds()

			if err != nil {
				// Graceful Fallback Strategy
				node.Status = StatusFallback
				node.Warning = fmt.Sprintf("Lỗi thực thi: %v (đã kích hoạt Fallback an toàn)", err)
				g.AppendLog(fmt.Sprintf("[%s] Cảnh báo Node %s: %s", time.Now().Format("15:04:05"), node.Label, node.Warning))
				outputs = map[string]interface{}{"status": "fallback"}
			} else {
				if node.Status != StatusFallback {
					node.Status = StatusCompleted
				}
			}

			node.OutputValues = outputs

			// Store outputs in wireData
			for outPin, outVal := range outputs {
				key := fmt.Sprintf("%s:%s", node.ID, outPin)
				wireData[key] = outVal
			}

			// Check for special outputs (QC score, export path)
			if score, ok := outputs["score"].(int); ok {
				g.QCScore = score
			}
			if verdict, ok := outputs["verdict"].(string); ok {
				g.QCVerdict = verdict
			}
			if outPath, ok := outputs["output_path"].(string); ok {
				g.FinalOutputURL = outPath
			}
			if finalVid, ok := outputs["final_video"].(string); ok {
				if g.FinalOutputURL == "" {
					g.FinalOutputURL = finalVid
				}
			}

			completed[node.ID] = true
			g.AppendLog(fmt.Sprintf("[%s] Hoàn thành Node %s (%dms).", time.Now().Format("15:04:05"), node.Label, node.DurationMs))
			e.notify(g, node)
		}
	}

	g.Status = GraphCompleted
	g.FinishedAt = time.Now()
	g.AppendLog(fmt.Sprintf("[%s] Toàn bộ đồ thị đã hoàn thành xuất sắc! QC: %d/100 (%s)",
		time.Now().Format("15:04:05"), g.QCScore, g.QCVerdict))
	e.notify(g, nil)

	return nil
}

func hasCycle(dependencies map[string]map[string]bool) bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var isCyclic func(node string) bool
	isCyclic = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for dep := range dependencies[node] {
			if !visited[dep] {
				if isCyclic(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for node := range dependencies {
		if !visited[node] {
			if isCyclic(node) {
				return true
			}
		}
	}
	return false
}
