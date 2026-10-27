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
