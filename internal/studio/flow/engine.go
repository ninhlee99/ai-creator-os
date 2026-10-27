package flow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ValidateGraph kiểm tra tính hợp lệ của graph: loại node, kết nối cổng và tính chu trình.
func ValidateGraph(graph *FlowGraph) ([]string, error) {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil, errors.New("đồ thị rỗng, cần ít nhất một node")
	}

	nodeMap := make(map[string]*FlowNode, len(graph.Nodes))
	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		if _, exists := nodeMap[n.ID]; exists {
			return nil, fmt.Errorf("trùng lặp ID node: %s", n.ID)
		}
		if _, ok := GetNodeDefinition(n.Type); !ok {
			return nil, fmt.Errorf("loại node chưa được đăng ký: %s (id=%s)", n.Type, n.ID)
		}
		nodeMap[n.ID] = n
	}

	// Kiểm tra tính hợp lệ của các cạnh kết nối
	for _, edge := range graph.Edges {
		fromNode, okFrom := nodeMap[edge.FromNode]
		toNode, okTo := nodeMap[edge.ToNode]
		if !okFrom {
			return nil, fmt.Errorf("kết nối từ node không tồn tại: %s", edge.FromNode)
		}
		if !okTo {
			return nil, fmt.Errorf("kết nối tới node không tồn tại: %s", edge.ToNode)
		}

		// Kiểm tra cổng nguồn
		fromPortFound := false
		var fromPortType PortType
		for _, p := range fromNode.Outputs {
			if p.ID == edge.FromPort {
				fromPortFound = true
				fromPortType = p.Type
				break
			}
		}
		if !fromPortFound {
			// Tra cứu từ definition
			if def, ok := GetNodeDefinition(fromNode.Type); ok {
				for _, p := range def.Outputs {
					if p.ID == edge.FromPort {
						fromPortFound = true
						fromPortType = p.Type
						break
					}
				}
			}
		}

		// Kiểm tra cổng đích
		toPortFound := false
		var toPortType PortType
		for _, p := range toNode.Inputs {
			if p.ID == edge.ToPort {
				toPortFound = true
				toPortType = p.Type
				break
			}
		}
		if !toPortFound {
			if def, ok := GetNodeDefinition(toNode.Type); ok {
				for _, p := range def.Inputs {
					if p.ID == edge.ToPort {
						toPortFound = true
						toPortType = p.Type
						break
					}
				}
			}
		}

		if !fromPortFound {
			return nil, fmt.Errorf("node %s không có cổng xuất '%s'", edge.FromNode, edge.FromPort)
		}
		if !toPortFound {
			return nil, fmt.Errorf("node %s không có cổng nhận '%s'", edge.ToNode, edge.ToPort)
		}

		// Kiểm tra tính tương thích của kiểu dữ liệu
		if !isPortTypeCompatible(fromPortType, toPortType) {
			return nil, fmt.Errorf("kiểu dữ liệu cổng không khớp: node %s cổng '%s' (%s) -> node %s cổng '%s' (%s)",
				edge.FromNode, edge.FromPort, fromPortType, edge.ToNode, edge.ToPort, toPortType)
		}
	}

	// Sắp xếp topo (Topological Sort) và phát hiện chu trình (Kahn's Algorithm)
	order, err := topologicalSort(graph)
	if err != nil {
		return nil, err
	}

	return order, nil
}

func isPortTypeCompatible(from, to PortType) bool {
	if to == PortTypeAny || from == PortTypeAny || to == "" || from == "" {
		return true
	}
	if from == to {
		return true
	}
	// Image có thể truyền vào ImageList (tự động wrap thành 1 phần tử)
	if from == PortTypeImage && to == PortTypeImageList {
		return true
	}
	return false
}

func topologicalSort(graph *FlowGraph) ([]string, error) {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, n := range graph.Nodes {
		inDegree[n.ID] = 0
		adj[n.ID] = []string{}
	}

	for _, edge := range graph.Edges {
		inDegree[edge.ToNode]++
		adj[edge.FromNode] = append(adj[edge.FromNode], edge.ToNode)
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]string, 0, len(graph.Nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, neighbor := range adj[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(order) < len(graph.Nodes) {
		return nil, errors.New("đồ thị bị lặp chu trình (circular dependency detected), không thể xác định thứ tự thực thi")
	}

	return order, nil
}

// ExecuteGraph thực thi toàn bộ luồng đồ thị theo thứ tự topo.
func ExecuteGraph(ctx *ExecutionContext, graph *FlowGraph) (*FlowRun, error) {
	order, err := ValidateGraph(graph)
	if err != nil {
		return nil, fmt.Errorf("xác thực đồ thị thất bại: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	run := &FlowRun{
		ID:         ctx.RunID,
		GraphID:    graph.ID,
		GraphName:  graph.Name,
		Status:     "running",
		Progress:   0,
		NodeStates: make(map[string]NodeState),
		Outputs:    make(map[string]any),
		CreatedAt:  now,
	}

	// Khởi tạo trạng thái ban đầu cho toàn bộ node
	for _, n := range graph.Nodes {
		run.NodeStates[n.ID] = NodeState{Status: "queued", Progress: 0}
	}

	nodeMap := make(map[string]*FlowNode, len(graph.Nodes))
	for i := range graph.Nodes {
		nodeMap[graph.Nodes[i].ID] = &graph.Nodes[i]
	}

	// Lập bản đồ incoming edges cho từng node
	incomingEdges := make(map[string][]FlowEdge)
	for _, edge := range graph.Edges {
		incomingEdges[edge.ToNode] = append(incomingEdges[edge.ToNode], edge)
	}

	// Bộ lưu trữ dữ liệu truyền giữa các cổng của node: nodeID -> portID -> value
	portData := make(map[string]map[string]any)

	var logBuilder strings.Builder
	appendRunLog := func(msg string) {
		line := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
		logBuilder.WriteString(line)
		if ctx.Log != nil {
			ctx.Log(msg)
		}
	}

	appendRunLog(fmt.Sprintf("Khởi chạy đồ thị '%s' (gồm %d nodes, %d kết nối)...", graph.Name, len(graph.Nodes), len(graph.Edges)))

	totalNodes := len(order)
	for idx, nodeID := range order {
		// Kiểm tra nếu context bị hủy giữa chừng
		if err := ctx.Context.Err(); err != nil {
			run.Status = "cancelled"
			run.Log = logBuilder.String()
			appendRunLog("Luồng thực thi đã bị hủy bởi người dùng")
			return run, err
		}

		node := nodeMap[nodeID]
		def, _ := GetNodeDefinition(node.Type)

		// 1. Kiểm tra nghiêm ngặt: Mọi node tiên quyết (dependencies) phải hoàn tất 100%
		var pendingParents []string
		for _, edge := range incomingEdges[nodeID] {
			parentState, exists := run.NodeStates[edge.FromNode]
			if !exists || parentState.Status != "completed" {
				pTitle := edge.FromNode
				if pNode, ok := nodeMap[edge.FromNode]; ok {
					pTitle = pNode.Title
				}
				pendingParents = append(pendingParents, pTitle)
			}
		}
		if len(pendingParents) > 0 {
			nodeState := NodeState{
				Status:     "failed",
				Error:      fmt.Sprintf("Chưa thể thực thi vì các node tiên quyết chưa hoàn thành: %s", strings.Join(pendingParents, ", ")),
				FinishedAt: time.Now().Format(time.RFC3339),
			}
			run.NodeStates[nodeID] = nodeState
			run.Status = "failed"
			run.Log = logBuilder.String()
			appendRunLog(fmt.Sprintf("LỖI PHỤ THUỘC tại node '%s': Cần chờ các node [%s] hoàn tất trước!", node.Title, strings.Join(pendingParents, ", ")))
			return run, fmt.Errorf("node '%s' cần chờ các node hoàn tất: %s", node.Title, strings.Join(pendingParents, ", "))
		}

		if len(incomingEdges[nodeID]) > 0 {
			parentTitles := make([]string, 0, len(incomingEdges[nodeID]))
			for _, edge := range incomingEdges[nodeID] {
				pTitle := edge.FromNode
				if pNode, ok := nodeMap[edge.FromNode]; ok {
					pTitle = pNode.Title
				}
				parentTitles = append(parentTitles, pTitle)
			}
			appendRunLog(fmt.Sprintf("Node [%d/%d] '%s': Đã xác nhận hoàn tất đầy đủ %d node nguồn: [%s]", idx+1, totalNodes, node.Title, len(parentTitles), strings.Join(parentTitles, ", ")))
		}

		// 2. Thu thập inputs từ các node đi trước
		nodeInputs := make(map[string]any)
		for _, edge := range incomingEdges[nodeID] {
			if sourceOutputs, ok := portData[edge.FromNode]; ok {
				if val, exists := sourceOutputs[edge.FromPort]; exists {
					nodeInputs[edge.ToPort] = val
				}
			}
		}

		// Đánh dấu node đang chạy
		nodeState := NodeState{
			Status:    "running",
			Progress:  20,
			StartedAt: time.Now().Format(time.RFC3339),
		}
		run.NodeStates[nodeID] = nodeState
		if ctx.NodeStatus != nil {
			ctx.NodeStatus(nodeID, nodeState)
		}

		appendRunLog(fmt.Sprintf("Node [%d/%d] '%s' (%s) bắt đầu xử lý...", idx+1, totalNodes, node.Title, node.Type))

		// Thực thi executor của node
		executor := def.Executor
		if executor == nil {
			nodeState.Status = "failed"
			nodeState.Error = "Node không có hàm thực thi"
			run.NodeStates[nodeID] = nodeState
			run.Status = "failed"
			run.Log = logBuilder.String()
			return run, fmt.Errorf("node %s thiếu executor", node.Title)
		}

		outputs, execErr := executor(ctx, node, nodeInputs)
		if execErr != nil {
			nodeState.Status = "failed"
			nodeState.Error = execErr.Error()
			nodeState.FinishedAt = time.Now().Format(time.RFC3339)
			run.NodeStates[nodeID] = nodeState
			run.Status = "failed"
			run.Log = logBuilder.String()
			appendRunLog(fmt.Sprintf("LỖI tại node '%s': %s", node.Title, execErr.Error()))
			return run, execErr
		}

		// Lưu kết quả của node vào bảng trung gian
		if outputs == nil {
			outputs = make(map[string]any)
		}
		portData[nodeID] = outputs

		// Xác định output preview (ảnh hoặc video) CHỈ CHO CÁC NODE VISUAL MEDIA THỰC TẾ
		var previewURL string
		if isVisualNodeType(node.Type) {
			for _, val := range outputs {
				if s, ok := val.(string); ok && s != "" {
					ext := strings.ToLower(filepath.Ext(s))
					if (ext == ".mp4" || ext == ".webm" || ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp") && !strings.Contains(s, "_mask.") {
						previewURL = s
						break
					}
				}
			}
		}

		// Nếu có file preview và ctx.OutDir được chỉ định, đảm bảo copy về OutDir để web load /media/ không 404
		if previewURL != "" && ctx.OutDir != "" && fileExists(previewURL) {
			destMedia := filepath.Join(ctx.OutDir, filepath.Base(previewURL))
			if !fileExists(destMedia) {
				_ = copyFile(previewURL, destMedia)
			}
		}

		nodeState.Status = "completed"
		nodeState.Progress = 100
		nodeState.OutputPreview = previewURL
		nodeState.FinishedAt = time.Now().Format(time.RFC3339)
		run.NodeStates[nodeID] = nodeState

		if ctx.NodeStatus != nil {
			ctx.NodeStatus(nodeID, nodeState)
		}

		progressPercent := int(float64(idx+1) / float64(totalNodes) * 100)
		run.Progress = progressPercent

		// Ghi nhận final output nếu là node output
		if node.Type == "video_output" {
			if finalVid, ok := outputs["final_video"].(string); ok {
				run.FinalOutput = finalVid
			}
		} else if node.Type == "image_output" {
			if finalImg, ok := outputs["final_image"].(string); ok {
				run.FinalOutput = finalImg
			}
		}

		appendRunLog(fmt.Sprintf("Node [%d/%d] '%s' hoàn tất thành công (100%%)", idx+1, totalNodes, node.Title))
	}

	run.Status = "completed"
	run.Progress = 100
	run.FinishedAt = time.Now().Format(time.RFC3339)
	run.Log = logBuilder.String()
	appendRunLog(fmt.Sprintf("Toàn bộ đồ thị đã thực thi thành công 100%% (Final Output: %s)", filepath.Base(run.FinalOutput)))

	return run, nil
}

func isVisualNodeType(nodeType string) bool {
	switch nodeType {
	case "character_input", "product_input", "product_extractor", "image_composite",
		"virtual_tryon", "google_flow_video", "muse_ai_video", "video_source",
		"video_upscale", "concat_scenes", "video_output", "image_output":
		return true
	default:
		return false
	}
}
