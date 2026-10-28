package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var workflowPortKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

func WorkflowReusableError(graph *FlowGraph) string {
	inputs, outputs, err := WorkflowContract(graph)
	if err != nil {
		return err.Error()
	}
	if _, err := ValidateGraph(graph); err != nil {
		return err.Error()
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		return "workflow cần ít nhất một node Đầu vào và một node Đầu ra"
	}
	return ""
}

func WorkflowContract(graph *FlowGraph) (inputs, outputs []FlowPort, err error) {
	if graph == nil {
		return nil, nil, fmt.Errorf("workflow không tồn tại")
	}
	inputs = make([]FlowPort, 0)
	outputs = make([]FlowPort, 0)
	seenInputs := make(map[string]bool)
	seenOutputs := make(map[string]bool)
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if node.Type != "workflow_input" && node.Type != "workflow_output" {
			continue
		}
		key := nodeParamString(node.Params, "key", "")
		if !workflowPortKeyPattern.MatchString(key) {
			return nil, nil, fmt.Errorf("node %q cần key chữ thường, số và gạch dưới; bắt đầu bằng chữ cái", node.Title)
		}
		label := nodeParamString(node.Params, "label", key)
		portType := PortType(nodeParamString(node.Params, "value_type", string(PortTypeAny)))
		if !validWorkflowPortType(portType) {
			return nil, nil, fmt.Errorf("node %q có kiểu dữ liệu không hỗ trợ: %s", node.Title, portType)
		}
		port := FlowPort{ID: key, Name: label, Type: portType}
		if node.Type == "workflow_input" {
			if seenInputs[key] {
				return nil, nil, fmt.Errorf("trùng input workflow: %s", key)
			}
			seenInputs[key] = true
			port.Required = nodeParamBool(node.Params, "required", true)
			inputs = append(inputs, port)
		} else {
			if seenOutputs[key] {
				return nil, nil, fmt.Errorf("trùng output workflow: %s", key)
			}
			seenOutputs[key] = true
			outputs = append(outputs, port)
		}
	}
	return inputs, outputs, nil
}

// ValidateWorkflowRunInputs checks per-run values against declared workflow inputs.
func ValidateWorkflowRunInputs(graph *FlowGraph, values map[string]any) error {
	inputs, _, err := WorkflowContract(graph)
	if err != nil {
		return err
	}
	ports := make(map[string]FlowPort, len(inputs))
	for _, port := range inputs {
		ports[port.ID] = port
	}
	for key := range values {
		if _, ok := ports[key]; !ok {
			return fmt.Errorf("input workflow không được khai báo: %s", key)
		}
	}
	for _, port := range inputs {
		value, provided := values[port.ID]
		if !provided {
			value, provided = workflowInputDefault(graph, port.ID)
		}
		if port.Required && isEmptyWorkflowValue(value, provided) {
			return fmt.Errorf("thiếu input workflow bắt buộc %q", port.Name)
		}
	}
	return nil
}

func workflowInputDefault(graph *FlowGraph, key string) (any, bool) {
	for _, node := range graph.Nodes {
		if node.Type == "workflow_input" && nodeParamString(node.Params, "key", "") == key {
			value, exists := node.Params["value"]
			return value, exists
		}
	}
	return nil, false
}

func validWorkflowPortType(portType PortType) bool {
	switch portType {
	case PortTypeAny, PortTypeImage, PortTypeImageList, PortTypeTextList, PortTypeVideo, PortTypeAudio,
		PortTypeText, PortTypeProduct, PortTypeMotion, PortTypeNumber, PortTypeConfig:
		return true
	default:
		return false
	}
}

func executeWorkflowInput(ctx *ExecutionContext, node *FlowNode, _ map[string]any) (map[string]any, error) {
	key := nodeParamString(node.Params, "key", "input")
	value, exists := ctx.WorkflowInputs[key]
	if !exists {
		value, exists = node.Params["value"]
	}
	if isEmptyWorkflowValue(value, exists) && nodeParamBool(node.Params, "required", true) {
		return nil, fmt.Errorf("thiếu input workflow %q", nodeParamString(node.Params, "label", key))
	}
	return map[string]any{"value": value}, nil
}

func executeWorkflowOutput(_ *ExecutionContext, _ *FlowNode, inputs map[string]any) (map[string]any, error) {
	value, exists := inputs["value"]
	if isEmptyWorkflowValue(value, exists) {
		return nil, fmt.Errorf("workflow output chưa được nối hoặc không có dữ liệu")
	}
	return map[string]any{"value": value}, nil
}

func isEmptyWorkflowValue(value any, exists bool) bool {
	if !exists || value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func executeWorkflowCall(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	workflowID := nodeParamString(node.Params, "workflow_id", "")
	if workflowID == "" {
		return nil, fmt.Errorf("chọn workflow con trước khi chạy")
	}
	if ctx.ResolveWorkflow == nil {
		return nil, fmt.Errorf("không có bộ phân giải workflow đã lưu")
	}
	childGraph, err := ctx.ResolveWorkflow(workflowID)
	if err != nil {
		return nil, fmt.Errorf("không tải được workflow con %q: %w", workflowID, err)
	}
	childInputs, childOutputs, err := WorkflowContract(childGraph)
	if err != nil {
		return nil, fmt.Errorf("workflow con %q có contract không hợp lệ: %w", childGraph.Name, err)
	}
	if expectedCategory := nodeParamString(node.Params, "workflow_category", ""); expectedCategory != "" && childGraph.Category != expectedCategory {
		return nil, fmt.Errorf("workflow con %q thuộc loại %q, node này chỉ nhận loại %q", childGraph.Name, childGraph.Category, expectedCategory)
	}
	if len(childInputs) == 0 || len(childOutputs) == 0 {
		return nil, fmt.Errorf("workflow %q cần ít nhất một node Đầu vào và một node Đầu ra", childGraph.Name)
	}
	if err := matchWorkflowPorts(node.Inputs, childInputs, "input"); err != nil {
		return nil, err
	}
	if err := matchWorkflowPorts(node.Outputs, childOutputs, "output"); err != nil {
		return nil, err
	}

	childValues := make(map[string]any, len(childInputs))
	for _, port := range childInputs {
		value, ok := inputs[port.ID]
		if !ok && port.Required {
			return nil, fmt.Errorf("thiếu input %q cho workflow %q", port.Name, childGraph.Name)
		}
		if ok {
			childValues[port.ID] = value
		}
	}

	childCtx := *ctx
	childCtx.WorkflowInputs = childValues
	childCtx.NodeStatus = nil
	childCtx.RunID = ctx.RunID + "__" + node.ID
	childCtx.WorkDir = filepath.Join(ctx.WorkDir, "subflows", safeWorkflowPathPart(node.ID))
	if err := os.MkdirAll(childCtx.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục workflow con: %w", err)
	}
	childGraph = cloneWorkflowWithPrefix(childGraph, safeWorkflowPathPart(node.ID)+"_")
	run, err := ExecuteGraph(&childCtx, childGraph)
	if err != nil {
		return nil, fmt.Errorf("workflow con %q thất bại: %w", childGraph.Name, err)
	}

	result := make(map[string]any, len(childOutputs))
	for _, port := range childOutputs {
		value, ok := run.Outputs[port.ID]
		if !ok {
			return nil, fmt.Errorf("workflow %q không tạo output %q", childGraph.Name, port.Name)
		}
		result[port.ID] = value
	}
	return result, nil
}

func executeWorkflowMap(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	workflowID := nodeParamString(node.Params, "workflow_id", "")
	if workflowID == "" {
		return nil, fmt.Errorf("chọn workflow con để xử lý từng phần tử")
	}
	if ctx.ResolveWorkflow == nil {
		return nil, fmt.Errorf("không có bộ phân giải workflow đã lưu")
	}
	items, ok := workflowMapItems(inputs["items"])
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("Workflow Map cần một danh sách không rỗng")
	}
	maxItems := nodeParamInt(node.Params, "max_items", 20)
	if maxItems < 1 || maxItems > 100 {
		return nil, fmt.Errorf("giới hạn phần tử phải từ 1 đến 100")
	}
	if len(items) > maxItems {
		return nil, fmt.Errorf("danh sách có %d phần tử, vượt giới hạn %d; tăng giới hạn hoặc chia danh sách", len(items), maxItems)
	}

	childGraph, err := ctx.ResolveWorkflow(workflowID)
	if err != nil {
		return nil, fmt.Errorf("không tải được workflow con %q: %w", workflowID, err)
	}
	if problem := WorkflowReusableError(childGraph); problem != "" {
		return nil, fmt.Errorf("workflow con chưa sẵn sàng: %s", problem)
	}
	childInputs, childOutputs, err := WorkflowContract(childGraph)
	if err != nil {
		return nil, err
	}
	itemInputKey := nodeParamString(node.Params, "item_input_key", "")
	resultOutputKey := nodeParamString(node.Params, "result_output_key", childOutputs[0].ID)
	indexInputKey := nodeParamString(node.Params, "index_input_key", "")
	itemInputMap, err := workflowMapInputMap(node.Params["item_input_map"])
	if err != nil {
		return nil, err
	}
	if itemInputKey == "" && len(itemInputMap) == 0 {
		itemInputKey = childInputs[0].ID
	}
	var itemPort, resultPort FlowPort
	for _, port := range childInputs {
		if port.ID == itemInputKey {
			itemPort = port
		}
		if _, mapped := itemInputMap[port.ID]; mapped && port.ID == itemInputKey {
			return nil, fmt.Errorf("input phần tử %q không thể đồng thời nhận cả item và item map", port.Name)
		}
		if indexInputKey != "" && port.ID == indexInputKey && port.Type != PortTypeNumber {
			return nil, fmt.Errorf("input thứ tự %q của workflow con phải có kiểu number", port.Name)
		}
	}
	for targetKey := range itemInputMap {
		found := false
		for _, port := range childInputs {
			if port.ID == targetKey {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("workflow con không có input map %q", targetKey)
		}
	}
	for _, port := range childOutputs {
		if port.ID == resultOutputKey {
			resultPort = port
		}
	}
	if itemInputKey != "" && itemPort.ID == "" {
		return nil, fmt.Errorf("workflow con không có input phần tử %q", itemInputKey)
	}
	if indexInputKey != "" && indexInputKey == itemInputKey {
		return nil, fmt.Errorf("input phần tử và input thứ tự phải khác nhau")
	}
	if indexInputKey != "" {
		found := false
		for _, port := range childInputs {
			if port.ID == indexInputKey {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("workflow con không có input thứ tự %q", indexInputKey)
		}
	}
	if resultPort.ID == "" {
		return nil, fmt.Errorf("workflow con không có output kết quả %q", resultOutputKey)
	}
	previousVideoFrameInputKey := nodeParamString(node.Params, "previous_video_frame_input_key", "")
	previousVideoFrameVariablesKey := nodeParamString(node.Params, "previous_video_frame_variables_key", "variables")
	previousVideoFrameInsertAt := nodeParamInt(node.Params, "previous_video_frame_insert_at", -1)
	insertAfterReferencePrefix := nodeParamBool(node.Params, "previous_video_frame_insert_after_reference_prefix", false)
	if previousVideoFrameInsertAt < -1 {
		return nil, fmt.Errorf("previous_video_frame_insert_at phải là -1 hoặc vị trí không âm")
	}
	referencePrefixCount := -1
	if insertAfterReferencePrefix {
		prefixCount := flowInputFloat(inputs, "reference_prefix_count", -1)
		if prefixCount < 0 || prefixCount > 100 || float64(int(prefixCount)) != prefixCount {
			return nil, fmt.Errorf("reference_prefix_count phải là số nguyên từ 0 đến 100")
		}
		referencePrefixCount = int(prefixCount)
	}
	if previousVideoFrameInputKey != "" {
		if resultPort.Type != PortTypeVideo {
			return nil, fmt.Errorf("frame liên tục chỉ hỗ trợ output video")
		}
	}
	if _, ok := inputs["items"]; !ok {
		return nil, fmt.Errorf("thiếu danh sách cần xử lý")
	}
	contextValues, err := workflowMapContext(inputs["context"])
	if err != nil {
		return nil, err
	}
	mergeContextIntoItem := nodeParamBool(node.Params, "merge_context_into_item", false)
	childInputSet := make(map[string]bool, len(childInputs))
	childInputTypes := make(map[string]PortType, len(childInputs))
	childInputRequired := make(map[string]bool, len(childInputs))
	for _, port := range childInputs {
		childInputSet[port.ID] = true
		childInputTypes[port.ID] = port.Type
		childInputRequired[port.ID] = port.Required
	}
	if previousVideoFrameInputKey != "" {
		portType, found := childInputTypes[previousVideoFrameInputKey]
		if !found || (portType != PortTypeImageList && portType != PortTypeAny) {
			return nil, fmt.Errorf("workflow con cần input ảnh dạng image_list %q để nhận frame shot trước", previousVideoFrameInputKey)
		}
	}
	sharedImageInputKey := nodeParamString(node.Params, "shared_image_input_key", "")
	sharedImages := flowStringList(inputs["reference_images"])
	if sharedImageInputKey != "" {
		portType, found := childInputTypes[sharedImageInputKey]
		if !found || (portType != PortTypeImageList && portType != PortTypeImage && portType != PortTypeAny) {
			return nil, fmt.Errorf("workflow con cần input ảnh %q kiểu image hoặc image_list", sharedImageInputKey)
		}
		if len(sharedImages) == 0 {
			return nil, fmt.Errorf("thiếu ảnh tham chiếu dùng chung cho input workflow con %q", sharedImageInputKey)
		}
		for _, imagePath := range sharedImages {
			if !fileExists(imagePath) || !isImageFile(imagePath) {
				return nil, fmt.Errorf("ảnh tham chiếu dùng chung không hợp lệ: %s", filepath.Base(imagePath))
			}
		}
	}

	results := make([]any, 0, len(items))
	mapID := safeWorkflowPathPart(node.ID)
	for i, item := range items {
		childValues := make(map[string]any, len(contextValues)+len(itemInputMap)+2)
		for key, value := range contextValues {
			if childInputSet[key] {
				childValues[key] = value
			}
		}
		if sharedImageInputKey != "" {
			if _, mapped := itemInputMap[sharedImageInputKey]; mapped || sharedImageInputKey == itemInputKey {
				return nil, fmt.Errorf("input ảnh dùng chung %q bị map trùng với item", sharedImageInputKey)
			}
			childValues[sharedImageInputKey] = sharedImages
		}
		if itemInputKey != "" {
			itemValue := item
			if mergeContextIntoItem {
				itemValues, ok := workflowMapObject(item)
				if !ok {
					return nil, fmt.Errorf("phần tử %d cần là object khi merge_context_into_item bật", i+1)
				}
				merged := make(map[string]any, len(contextValues)+len(itemValues))
				for key, value := range contextValues {
					merged[key] = value
				}
				for key, value := range itemValues {
					merged[key] = value
				}
				itemValue = merged
			}
			childValues[itemInputKey] = itemValue
		}
		if mergeContextIntoItem {
			itemValues, ok := workflowMapObject(item)
			if !ok {
				return nil, fmt.Errorf("phần tử %d cần là object khi merge_context_into_item bật", i+1)
			}
			for key, itemValue := range itemValues {
				if !childInputSet[key] {
					continue
				}
				if contextValue, exists := childValues[key]; exists {
					if contextObject, ok := workflowMapObject(contextValue); ok {
						if itemObject, ok := workflowMapObject(itemValue); ok {
							merged := make(map[string]any, len(contextObject)+len(itemObject))
							for field, value := range contextObject {
								merged[field] = value
							}
							for field, value := range itemObject {
								merged[field] = value
							}
							childValues[key] = merged
							continue
						}
					}
				}
				childValues[key] = itemValue
			}
		}
		if len(itemInputMap) > 0 {
			itemValues, ok := workflowMapObject(item)
			if !ok {
				return nil, fmt.Errorf("phần tử %d cần là object để map input workflow con", i+1)
			}
			for targetKey, sourceKey := range itemInputMap {
				value, exists := itemValues[sourceKey]
				if !exists || isEmptyWorkflowValue(value, true) {
					if !childInputRequired[targetKey] {
						continue
					}
					return nil, fmt.Errorf("phần tử %d thiếu trường %q để map vào input %q", i+1, sourceKey, targetKey)
				}
				childValues[targetKey] = value
			}
		}
		if indexInputKey != "" {
			childValues[indexInputKey] = i + 1
		}
		if previousVideoFrameInputKey != "" && childInputSet[previousVideoFrameVariablesKey] && childInputTypes[previousVideoFrameVariablesKey] == PortTypeConfig {
			variables, _ := workflowMapObject(childValues[previousVideoFrameVariablesKey])
			variablesWithContinuity := make(map[string]any, len(variables)+1)
			for key, value := range variables {
				variablesWithContinuity[key] = value
			}
			if insertAfterReferencePrefix {
				variablesWithContinuity["character_reference_count"] = referencePrefixCount
			}
			if i > 0 {
				variablesWithContinuity["previous_shot_reference_count"] = 1
			} else {
				variablesWithContinuity["previous_shot_reference_count"] = 0
			}
			childValues[previousVideoFrameVariablesKey] = variablesWithContinuity
		}
		if previousVideoFrameInputKey != "" && i > 0 {
			previousVideo, ok := results[i-1].(string)
			if !ok || !fileExists(previousVideo) || !isVideoFile(previousVideo) {
				return nil, fmt.Errorf("output phần tử %d không phải video hợp lệ để giữ continuity", i)
			}
			previousDuration, err := probeFlowDuration(previousVideo)
			if err != nil {
				return nil, fmt.Errorf("không đọc được thời lượng video phần tử %d: %w", i, err)
			}
			framePath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_previous_frame_%03d.jpg", mapID, i+1))
			if err := ExtractFrameFromVideo(ctx.Context, previousVideo, framePath, previousDuration/2); err != nil {
				return nil, fmt.Errorf("không trích xuất được frame liên tục từ phần tử %d: %w", i, err)
			}
			if !fileExists(framePath) || !isImageFile(framePath) {
				return nil, fmt.Errorf("frame liên tục của phần tử %d không hợp lệ", i)
			}
			references := append([]string(nil), flowStringList(childValues[previousVideoFrameInputKey])...)
			if len(references) == 0 {
				return nil, fmt.Errorf("thiếu ảnh tham chiếu hiện có để chèn frame liên tục vào %q", previousVideoFrameInputKey)
			}
			insertAt := previousVideoFrameInsertAt
			if insertAfterReferencePrefix {
				if referencePrefixCount > len(references) {
					return nil, fmt.Errorf("reference_prefix_count %d vượt quá %d ảnh tham chiếu", referencePrefixCount, len(references))
				}
				insertAt = referencePrefixCount
			}
			if insertAt < 0 || insertAt > len(references) {
				insertAt = len(references)
			}
			references = append(references, "")
			copy(references[insertAt+1:], references[insertAt:])
			references[insertAt] = framePath
			childValues[previousVideoFrameInputKey] = references
		}
		if err := ValidateWorkflowRunInputs(childGraph, childValues); err != nil {
			return nil, fmt.Errorf("input workflow con ở phần tử %d không hợp lệ: %w", i+1, err)
		}

		childCtx := *ctx
		if !nodeParamBool(node.Params, "share_muse_thread", true) {
			childCtx.MuseThread = &MuseThreadState{}
		}
		childCtx.WorkflowInputs = childValues
		childCtx.NodeStatus = nil
		childCtx.RunID = fmt.Sprintf("%s__%s_item_%03d", ctx.RunID, mapID, i+1)
		childCtx.WorkDir = filepath.Join(ctx.WorkDir, "iterations", mapID, fmt.Sprintf("item_%03d", i+1))
		if err := os.MkdirAll(childCtx.WorkDir, 0o755); err != nil {
			return nil, fmt.Errorf("không tạo được thư mục phần tử %d: %w", i+1, err)
		}
		if ctx.NodeStatus != nil {
			ctx.NodeStatus(node.ID, NodeState{Status: "running", Progress: int(float64(i) / float64(len(items)) * 100)})
		}
		iterationGraph := cloneWorkflowWithPrefix(childGraph, fmt.Sprintf("%s_item_%03d_", mapID, i+1))
		run, err := ExecuteGraph(&childCtx, iterationGraph)
		if err != nil {
			return nil, fmt.Errorf("workflow con thất bại ở phần tử %d/%d: %w", i+1, len(items), err)
		}
		result, exists := run.Outputs[resultOutputKey]
		if !exists || isEmptyFlowValue(result) {
			return nil, fmt.Errorf("workflow con không tạo output %q ở phần tử %d", resultPort.Name, i+1)
		}
		results = append(results, result)
		if ctx.NodeStatus != nil {
			ctx.NodeStatus(node.ID, NodeState{Status: "running", Progress: int(float64(i+1) / float64(len(items)) * 100)})
		}
		ctx.Log(fmt.Sprintf("Workflow Map: hoàn tất phần tử %d/%d", i+1, len(items)))
	}
	return map[string]any{"results": results}, nil
}

func workflowMapContext(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	contextValues, ok := workflowMapObject(value)
	if !ok {
		return nil, fmt.Errorf("context cần là object các giá trị dùng chung")
	}
	return contextValues, nil
}

func workflowMapObject(value any) (map[string]any, bool) {
	switch object := value.(type) {
	case map[string]any:
		return object, true
	case map[string]string:
		values := make(map[string]any, len(object))
		for key, value := range object {
			values[key] = value
		}
		return values, true
	default:
		return nil, false
	}
}

func workflowMapInputMap(value any) (map[string]string, error) {
	if value == nil {
		return map[string]string{}, nil
	}
	result := make(map[string]string)
	switch fields := value.(type) {
	case map[string]any:
		for target, rawSource := range fields {
			source, ok := rawSource.(string)
			if !ok || strings.TrimSpace(target) == "" || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("item_input_map cần ánh xạ tên input sang tên trường object")
			}
			result[target] = strings.TrimSpace(source)
		}
	case map[string]string:
		for target, source := range fields {
			if strings.TrimSpace(target) == "" || strings.TrimSpace(source) == "" {
				return nil, fmt.Errorf("item_input_map cần ánh xạ tên input sang tên trường object")
			}
			result[target] = strings.TrimSpace(source)
		}
	default:
		return nil, fmt.Errorf("item_input_map cần là object")
	}
	return result, nil
}

func workflowMapItems(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []string:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func matchWorkflowPorts(actual, expected []FlowPort, direction string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("contract %s của workflow thay đổi; tải lại node workflow", direction)
	}
	actualByID := make(map[string]FlowPort, len(actual))
	for _, port := range actual {
		actualByID[port.ID] = port
	}
	for _, port := range expected {
		candidate, ok := actualByID[port.ID]
		if !ok || candidate.Type != port.Type || (direction == "input" && candidate.Required != port.Required) {
			return fmt.Errorf("contract %s %q của workflow thay đổi; tải lại node workflow", direction, port.Name)
		}
	}
	return nil
}

func cloneWorkflowWithPrefix(graph *FlowGraph, prefix string) *FlowGraph {
	clone := *graph
	clone.Nodes = append([]FlowNode(nil), graph.Nodes...)
	clone.Edges = append([]FlowEdge(nil), graph.Edges...)
	ids := make(map[string]string, len(clone.Nodes))
	for i := range clone.Nodes {
		oldID := clone.Nodes[i].ID
		newID := prefix + oldID
		ids[oldID] = newID
		clone.Nodes[i].ID = newID
		clone.Nodes[i].Inputs = append([]FlowPort(nil), clone.Nodes[i].Inputs...)
		clone.Nodes[i].Outputs = append([]FlowPort(nil), clone.Nodes[i].Outputs...)
	}
	for i := range clone.Edges {
		clone.Edges[i].FromNode = ids[clone.Edges[i].FromNode]
		clone.Edges[i].ToNode = ids[clone.Edges[i].ToNode]
	}
	return &clone
}

func safeWorkflowPathPart(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "workflow"
	}
	return builder.String()
}
