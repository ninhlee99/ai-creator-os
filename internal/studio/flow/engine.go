package flow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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
		if isUnsupportedExperimental3DNode(n.Type) {
			return nil, fmt.Errorf("node %q (%s) đã bị gỡ vì chưa có bộ xử lý 3D thật; hãy thay bằng node được hỗ trợ", n.Title, n.Type)
		}
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

		fromPort, fromPortFound := nodeOutputPort(fromNode, edge.FromPort)
		toPort, toPortFound := nodeInputPort(toNode, edge.ToPort)

		if !fromPortFound {
			return nil, fmt.Errorf("node %s không có cổng xuất '%s'", edge.FromNode, edge.FromPort)
		}
		if !toPortFound {
			return nil, fmt.Errorf("node %s không có cổng nhận '%s'", edge.ToNode, edge.ToPort)
		}

		// Kiểm tra tính tương thích của kiểu dữ liệu
		if !isPortTypeCompatible(fromPort.Type, toPort.Type) {
			return nil, fmt.Errorf("kiểu dữ liệu cổng không khớp: node %s cổng '%s' (%s) -> node %s cổng '%s' (%s)",
				edge.FromNode, edge.FromPort, fromPort.Type, edge.ToNode, edge.ToPort, toPort.Type)
		}
	}

	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		inputs := nodeInputPorts(node)
		for _, port := range inputs {
			if !port.Required || hasIncomingPort(graph.Edges, node.ID, port.ID) || nodeHasInputValue(node, port.ID) {
				continue
			}
			return nil, fmt.Errorf("node %s thiếu input bắt buộc '%s'", node.Title, port.Name)
		}
	}
	// Sắp xếp topo (Topological Sort) và phát hiện chu trình (Kahn's Algorithm)
	order, err := topologicalSort(graph)
	if err != nil {
		return nil, err
	}

	if err := validateGraphTerminals(graph); err != nil {
		return nil, err
	}

	return order, nil
}

func validateGraphTerminals(graph *FlowGraph) error {
	terminals := map[string]bool{}
	reverseEdges := make(map[string][]string, len(graph.Nodes))
	for _, edge := range graph.Edges {
		reverseEdges[edge.ToNode] = append(reverseEdges[edge.ToNode], edge.FromNode)
	}
	for _, node := range graph.Nodes {
		if isWorkflowTerminal(node.Type) {
			terminals[node.ID] = true
		}
	}
	if len(terminals) == 0 {
		return nil
	}
	reachesTerminal := make(map[string]bool, len(graph.Nodes))
	queue := make([]string, 0, len(terminals))
	for id := range terminals {
		reachesTerminal[id] = true
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, parent := range reverseEdges[current] {
			if !reachesTerminal[parent] {
				reachesTerminal[parent] = true
				queue = append(queue, parent)
			}
		}
	}
	for _, node := range graph.Nodes {
		if !reachesTerminal[node.ID] {
			return fmt.Errorf("node %q không dẫn tới node kết thúc", node.Title)
		}
	}
	return nil
}

func isWorkflowTerminal(nodeType string) bool {
	switch nodeType {
	case "video_output", "image_output", "video_master_output", "custom_export_publish",
		"preview_media", "preview_image", "preview_video", "workflow_output", "custom_api_webhook":
		return true
	default:
		return false
	}
}

func nodeInputPort(node *FlowNode, portID string) (FlowPort, bool) {
	for _, port := range nodeInputPorts(node) {
		if port.ID == portID {
			return port, true
		}
	}
	return FlowPort{}, false
}

func nodeOutputPort(node *FlowNode, portID string) (FlowPort, bool) {
	for _, port := range nodeOutputPorts(node) {
		if port.ID == portID {
			return port, true
		}
	}
	return FlowPort{}, false
}

func nodeInputPorts(node *FlowNode) []FlowPort {
	if node == nil {
		return nil
	}
	if node.Type == "workflow_output" {
		return []FlowPort{{ID: "value", Name: "Giá trị", Type: workflowNodeValueType(node), Required: true}}
	}
	if def, ok := GetNodeDefinition(node.Type); ok && node.Type != "workflow_call" && !strings.HasPrefix(node.Type, "workflow_call_") {
		return def.Inputs
	}
	if len(node.Inputs) > 0 {
		return node.Inputs
	}
	return nil
}

func nodeOutputPorts(node *FlowNode) []FlowPort {
	if node == nil {
		return nil
	}
	switch node.Type {
	case "workflow_input":
		return []FlowPort{{ID: "value", Name: "Giá trị", Type: workflowNodeValueType(node)}}
	case "workflow_output":
		return []FlowPort{{ID: "value", Name: "Giá trị", Type: workflowNodeValueType(node)}}
	}
	if def, ok := GetNodeDefinition(node.Type); ok && node.Type != "workflow_call" && !strings.HasPrefix(node.Type, "workflow_call_") {
		return def.Outputs
	}
	if len(node.Outputs) > 0 {
		return node.Outputs
	}
	return nil
}

func workflowNodeValueType(node *FlowNode) PortType {
	if node == nil {
		return PortTypeAny
	}
	typeName := nodeParamString(node.Params, "value_type", string(PortTypeAny))
	portType := PortType(typeName)
	if !validWorkflowPortType(portType) {
		return PortTypeAny
	}
	return portType
}

func hasIncomingPort(edges []FlowEdge, nodeID, portID string) bool {
	for _, edge := range edges {
		if edge.ToNode == nodeID && edge.ToPort == portID {
			return true
		}
	}
	return false
}

func nodeHasInputValue(node *FlowNode, portID string) bool {
	if node == nil || node.Params == nil {
		return false
	}
	keys := []string{portID}
	switch portID {
	case "prompt", "prompt_in":
		keys = append(keys, "prompt", "template")
	case "character_image":
		keys = append(keys, "model_path", "character_image")
	case "product_image":
		keys = append(keys, "product_image")
	case "video_in", "video_path":
		keys = append(keys, "video_path", "video_url")
	case "image_in", "source_image":
		keys = append(keys, "image_path", "model_path", "product_image", "source_image")
	}
	for _, key := range keys {
		if value, ok := node.Params[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
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
	if ctx == nil {
		return nil, errors.New("execution context is nil")
	}
	if graph == nil {
		return nil, errors.New("workflow graph is nil")
	}
	if ctx.Context == nil {
		ctx.Context = context.Background()
	}
	for _, activeID := range ctx.WorkflowStack {
		if activeID == graph.ID {
			return nil, fmt.Errorf("phát hiện workflow gọi lặp: %s", graph.ID)
		}
	}
	if len(ctx.WorkflowStack) >= 8 {
		return nil, errors.New("workflow lồng nhau vượt quá 8 cấp")
	}
	ctx.WorkflowStack = append(ctx.WorkflowStack, graph.ID)
	defer func() { ctx.WorkflowStack = ctx.WorkflowStack[:len(ctx.WorkflowStack)-1] }()

	order, err := ValidateGraph(graph)
	if err != nil {
		return nil, fmt.Errorf("xác thực đồ thị thất bại: %w", err)
	}

	now := time.Now().Format(time.RFC3339)

	ctx.MuseThreadStarted = false

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

		// Share descriptive values from earlier nodes where explicit edges are not defined.
		for _, prevOutputs := range portData {
			for k, v := range prevOutputs {
				if strings.HasPrefix(k, "character_") || strings.HasPrefix(k, "garment_") || k == "face_id" {
					if nodeInputs[k] == nil {
						nodeInputs[k] = v
					}
				}
			}
		}

		for _, edge := range incomingEdges[nodeID] {
			if sourceOutputs, ok := portData[edge.FromNode]; ok {
				if val, exists := sourceOutputs[edge.FromPort]; exists {
					if source := nodeMap[edge.FromNode]; source.Type == "workflow_input" &&
						!nodeParamBool(source.Params, "required", true) && isEmptyFlowValue(val) {
						continue
					}
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
		for _, edge := range graph.Edges {
			if edge.FromNode != nodeID {
				continue
			}
			value, exists := outputs[edge.FromPort]
			if node.Type == "workflow_input" && !nodeParamBool(node.Params, "required", true) && (!exists || isEmptyFlowValue(value)) {
				continue
			}
			if !exists || isEmptyFlowValue(value) {
				execErr = fmt.Errorf("node %q không trả dữ liệu cho cổng đã nối %q", node.Title, edge.FromPort)
				nodeState.Status = "failed"
				nodeState.Error = execErr.Error()
				nodeState.FinishedAt = time.Now().Format(time.RFC3339)
				run.NodeStates[nodeID] = nodeState
				run.Status = "failed"
				run.Log = logBuilder.String()
				appendRunLog("LỖI contract output: " + execErr.Error())
				return run, execErr
			}
		}

		// Lưu kết quả của node vào bảng trung gian
		if outputs == nil {
			outputs = make(map[string]any)
		}
		portData[nodeID] = outputs
		if node.Type == "workflow_output" {
			key := nodeParamString(node.Params, "key", "output")
			if value, exists := nodeInputs["value"]; exists {
				run.Outputs[key] = value
			}
		}

		// Xác định output preview (ảnh hoặc video) cho các node visual media và node preview
		var previewURL string
		if isVisualNodeType(node.Type) {
			// Ưu tiên video trước nếu node có sinh video
			for _, key := range []string{"master_video", "rendered_video", "video_out", "graded_video", "final_video", "video", "image_out", "final_image", "character_ref", "sample_frame", "product_cutout", "warped_garment"} {
				if val, exists := outputs[key]; exists {
					if s, ok := val.(string); ok && s != "" && fileExists(s) {
						previewURL = s
						break
					}
				}
			}
			if previewURL == "" {
				outputKeys := make([]string, 0, len(outputs))
				for key := range outputs {
					outputKeys = append(outputKeys, key)
				}
				sort.Strings(outputKeys)
				for _, key := range outputKeys {
					val := outputs[key]
					if s, ok := val.(string); ok && s != "" {
						ext := strings.ToLower(filepath.Ext(s))
						if ext == ".mp4" || ext == ".webm" || ext == ".mov" || ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
							previewURL = s
							break
						}
					}
				}
			}
			if previewURL == "" {
				previewURL = firstMediaPath(outputs)
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
		nodeState.Outputs = outputs

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
		} else if node.Type == "video_master_output" {
			if finalVid, ok := outputs["master_video"].(string); ok {
				run.FinalOutput = finalVid
			}
		} else if node.Type == "custom_export_publish" {
			if finalFile, ok := outputs["final_file"].(string); ok {
				run.FinalOutput = finalFile
			}
		} else if node.Type == "preview_media" || node.Type == "preview_image" || node.Type == "preview_video" {
			if preview, ok := outputs["media_out"].(string); ok {
				run.FinalOutput = preview
			} else if preview, ok := outputs["images"].(string); ok {
				run.FinalOutput = preview
			}
		} else if node.Type == "workflow_output" {
			if finalMedia := firstMediaPath(nodeInputs["value"]); finalMedia != "" {
				run.FinalOutput = finalMedia
			}
		} else if node.Type == "workflow_call" || strings.HasPrefix(node.Type, "workflow_call_") {
			if finalMedia := firstMediaPath(outputs); finalMedia != "" {
				run.FinalOutput = finalMedia
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

func isEmptyFlowValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []string:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func isVisualNodeType(nodeType string) bool {
	switch nodeType {
	case "character_input", "product_input", "product_extractor", "image_composite",
		"virtual_tryon", "google_flow_video", "muse_ai_video", "video_source",
		"video_upscale", "concat_scenes", "video_output", "image_output",
		"muse_ai_neural_render", "cinematic_color_grading", "video_master_output",
		"preview_media", "preview_image", "preview_video", "workflow_call", "workflow_map", "workflow_output",
		"text_to_speech", "audio_mix",
		"scene_environment", "lighting_atmosphere", "dance_motion_extractor", "dance_retarget",
		"custom_universal_ai", "custom_media_input", "custom_vision_analyzer", "custom_media_processor", "custom_export_publish":
		return true
	default:
		return false
	}
}

func firstMediaPath(value any) string {
	mediaExt := map[string]bool{".mp4": true, ".webm": true, ".mov": true, ".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".mp3": true, ".aac": true, ".wav": true, ".ogg": true, ".m4a": true, ".flac": true}
	var find func(any) string
	find = func(item any) string {
		switch value := item.(type) {
		case string:
			if value != "" && mediaExt[strings.ToLower(filepath.Ext(value))] && fileExists(value) {
				return value
			}
		case []any:
			for _, child := range value {
				if path := find(child); path != "" {
					return path
				}
			}
		case []string:
			for _, child := range value {
				if path := find(child); path != "" {
					return path
				}
			}
		case map[string]any:
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if path := find(value[key]); path != "" {
					return path
				}
			}
		}
		return ""
	}
	return find(value)
}
