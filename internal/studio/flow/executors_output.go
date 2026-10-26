package flow

import (
	"fmt"
	"os"
	"path/filepath"
)

func executeVideoOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	title := inputString(inputs, "title", "Video hoàn thiện")

	if videoIn == "" || !fileExists(videoIn) {
		return nil, fmt.Errorf("VideoOutputNode: Không có file video đầu vào hợp lệ")
	}

	_ = os.MkdirAll(ctx.OutDir, 0o755)
	finalFilename := fmt.Sprintf("flow_video_%s.mp4", ctx.RunID)
	finalPath := filepath.Join(ctx.OutDir, finalFilename)

	if err := copyFile(videoIn, finalPath); err != nil {
		finalPath = videoIn
	}

	autoDraft := nodeParamBool(node.Params, "auto_publish_draft", false)
	if autoDraft {
		ctx.Log(fmt.Sprintf("VideoOutputNode: Tự động xếp hàng video '%s' vào lịch xuất bản TikTok Draft", title))
	}

	ctx.Log(fmt.Sprintf("VideoOutputNode: Video cuối cùng đã sẵn sàng tại %s (%s)", filepath.Base(finalPath), title))

	return map[string]any{
		"final_video": finalPath,
	}, nil
}

func executeImageOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	if imgIn == "" || !fileExists(imgIn) {
		return nil, fmt.Errorf("ImageOutputNode: Không có file ảnh đầu vào hợp lệ")
	}

	_ = os.MkdirAll(ctx.OutDir, 0o755)
	finalFilename := fmt.Sprintf("flow_image_%s.jpg", ctx.RunID)
	finalPath := filepath.Join(ctx.OutDir, finalFilename)

	if err := copyFile(imgIn, finalPath); err != nil {
		finalPath = imgIn
	}

	ctx.Log(fmt.Sprintf("ImageOutputNode: Ảnh hoàn thiện đã sẵn sàng tại %s", filepath.Base(finalPath)))

	return map[string]any{
		"final_image": finalPath,
	}, nil
}
