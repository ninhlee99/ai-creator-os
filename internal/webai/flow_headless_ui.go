package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func flowUpdatedCookieHeader(ctx context.Context, client *browserCDPClient) string {
	response, err := client.call(ctx, "Network.getAllCookies", map[string]any{}, client.sessionID)
	if err != nil {
		return ""
	}
	var cookies struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if json.Unmarshal(response, &cookies) != nil {
		return ""
	}
	header := browserCookieHeaderAllPaths(cookies.Cookies, BrowserCookieScope{Platform: PlatformGoogleFlow, CookieHost: "flow.google.com"})
	if !googleFlowSessionCookiePresent(cookies.Cookies) {
		return ""
	}
	return header
}

func flowSelectModelAndMode(ctx context.Context, client *browserCDPClient, request flowHeadlessRequest) error {
	if request.Mode == "text" {
		return nil
	}
	modeLabels := []string{"ingredients to video", "ingredients", "thành phần thành video", "nguyên liệu thành video", "thành phần", "nguyên liệu"}
	if request.Mode == "video_edit" {
		modeLabels = []string{"video to video", "video-to-video", "edit video", "video editing", "ingredients to video", "ingredients", "thành phần thành video", "nguyên liệu thành video", "thành phần", "nguyên liệu"}
	}
	if request.Mode != "ingredients" && request.Mode != "video_edit" {
		return fmt.Errorf("unsupported Google Flow mode %q", request.Mode)
	}

	if strings.TrimSpace(request.ModelLabel) != "" {
		modelLabels := []string{strings.ToLower(strings.TrimSpace(request.ModelLabel))}
		if !flowHasSelectedLabel(ctx, client, modelLabels) && !flowTriggerContains(ctx, client, modelLabels) {
			opened, err := flowWaitAndClick(ctx, client, `(() => {const e=document.querySelector('button.settings-trigger-button,.settings-trigger-button,button[aria-label*="settings" i]');if(!e)return null;const r=e.getBoundingClientRect();return r.width>0&&r.height>0?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`, 5*time.Second)
			if err != nil || !opened {
				return errors.New("Google Flow model settings control was not found")
			}
			clicked, err := flowClickNamed(ctx, client, modelLabels, 4*time.Second)
			if err != nil {
				return err
			}
			if !clicked {
				return fmt.Errorf("Google Flow model %q is unavailable in current account or UI", request.ModelLabel)
			}
		}
	}

	videoLabels := []string{"video", "videocam", "video generation"}
	if !flowHasSelectedLabel(ctx, client, videoLabels) {
		clicked, err := flowClickNamed(ctx, client, videoLabels, 6*time.Second)
		if err != nil {
			return err
		}
		if !clicked && !flowTriggerContains(ctx, client, videoLabels) {
			return errors.New("Google Flow Video mode could not be selected")
		}
	}
	if !flowHasSelectedLabel(ctx, client, modeLabels) {
		clicked, err := flowClickNamed(ctx, client, modeLabels, 8*time.Second)
		if err != nil {
			return err
		}
		if !clicked {
			return errors.New("Google Flow Ingredients mode could not be selected")
		}
	}
	if !flowHasSelectedLabel(ctx, client, modeLabels) && !flowTriggerContains(ctx, client, modeLabels) {
		return errors.New("Google Flow did not confirm the requested Ingredients mode")
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "rawKeyDown", "windowsVirtualKeyCode": 27}, client.sessionID)
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "windowsVirtualKeyCode": 27}, client.sessionID)
	return nil
}

func flowClickNamed(ctx context.Context, client *browserCDPClient, labels []string, timeout time.Duration) (bool, error) {
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};const xs=[...document.querySelectorAll('button,[role="button"],[role="menuitem"],[role="option"],[role="combobox"],mat-button-toggle')].filter(visible);const text=e=>((e.innerText||'')+' '+(e.getAttribute('aria-label')||'')+' '+(e.getAttribute('title')||'')).toLowerCase().replace(/\s+/g,' ').trim();const enabled=e=>!e.disabled&&e.getAttribute('aria-disabled')!=='true';const exact=xs.filter(x=>enabled(x)&&labels.some(s=>text(x)===s));let e=exact.find(x=>x.matches('[role="option"],[role="menuitem"],mat-button-toggle')||x.closest('.cdk-overlay-pane'));if(!e)e=exact.find(x=>!x.matches('.settings-trigger-button'));if(!e)e=exact[0];if(!e)e=xs.find(x=>enabled(x)&&(x.matches('[role="option"],[role="menuitem"],mat-button-toggle')||x.closest('.cdk-overlay-pane'))&&labels.some(s=>text(x).includes(s)));if(!e)return false;e.click();return true})()`
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		value, err := client.evaluate(ctx, expression)
		if err == nil && value == true {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return false, nil
}

func flowHasSelectedLabel(ctx context.Context, client *browserCDPClient, labels []string) bool {
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};return [...document.querySelectorAll('[aria-selected="true"],[aria-pressed="true"],[data-state="checked"],mat-button-toggle-checked,.mat-button-toggle-checked,.selected,.active')].some(e=>{if(!visible(e))return false;const t=((e.innerText||'')+' '+(e.getAttribute('aria-label')||'')+' '+(e.getAttribute('title')||'')).toLowerCase();return labels.some(s=>t.includes(s))})})()`
	value, err := client.evaluate(ctx, expression)
	return err == nil && value == true
}

func flowTriggerContains(ctx context.Context, client *browserCDPClient, labels []string) bool {
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const e=document.querySelector('button.settings-trigger-button,.settings-trigger-button,button[aria-label*="settings" i],button[aria-label*="aspect" i]');if(!e)return false;const t=((e.innerText||'')+' '+(e.getAttribute('aria-label')||'')).toLowerCase();return labels.some(s=>t.includes(s))})()`
	value, err := client.evaluate(ctx, expression)
	return err == nil && value == true
}

type flowFileInput struct {
	NodeID   int
	Accept   string
	Multiple bool
}

func flowFileInputs(ctx context.Context, client *browserCDPClient) ([]flowFileInput, error) {
	doc, err := client.call(ctx, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, client.sessionID)
	if err != nil {
		return nil, err
	}
	var document struct {
		Root struct {
			NodeID int `json:"nodeId"`
		} `json:"root"`
	}
	if json.Unmarshal(doc, &document) != nil || document.Root.NodeID == 0 {
		return nil, errors.New("Flow document has no root node")
	}
	query, err := client.call(ctx, "DOM.querySelectorAll", map[string]any{"nodeId": document.Root.NodeID, "selector": "input[type=file]"}, client.sessionID)
	if err != nil {
		return nil, err
	}
	var nodes struct {
		NodeIDs []int `json:"nodeIds"`
	}
	if json.Unmarshal(query, &nodes) != nil {
		return nil, errors.New("Flow file input query failed")
	}
	inputs := make([]flowFileInput, 0, len(nodes.NodeIDs))
	for _, nodeID := range nodes.NodeIDs {
		description, err := client.call(ctx, "DOM.describeNode", map[string]any{"nodeId": nodeID}, client.sessionID)
		if err != nil {
			continue
		}
		var node struct {
			Node struct {
				Attributes []string `json:"attributes"`
			} `json:"node"`
		}
		if json.Unmarshal(description, &node) != nil {
			continue
		}
		attrs := map[string]string{}
		for i := 0; i+1 < len(node.Node.Attributes); i += 2 {
			attrs[strings.ToLower(node.Node.Attributes[i])] = node.Node.Attributes[i+1]
		}
		_, multiple := attrs["multiple"]
		inputs = append(inputs, flowFileInput{NodeID: nodeID, Accept: strings.ToLower(attrs["accept"]), Multiple: multiple})
	}
	return inputs, nil
}

func flowInputMatches(input flowFileInput, kind string) bool {
	if input.Accept == "" || strings.Contains(input.Accept, "*/*") {
		return true
	}
	if kind == "image" {
		return strings.Contains(input.Accept, "image/")
	}
	return strings.Contains(input.Accept, "video/")
}

func flowFindFileInput(ctx context.Context, client *browserCDPClient, kind string) (flowFileInput, bool, error) {
	inputs, err := flowFileInputs(ctx, client)
	if err != nil {
		return flowFileInput{}, false, err
	}
	for _, input := range inputs {
		if flowInputMatches(input, kind) {
			return input, true, nil
		}
	}
	return flowFileInput{}, false, nil
}

func flowOpenUpload(ctx context.Context, client *browserCDPClient, kind string) error {
	// 1. Click Add menu trigger button in composer
	addTrigger := `(() => {
		const e = document.querySelector('button.add-menu-trigger, button[aria-label*="Thêm thành phần" i], button[aria-label*="Add ingredient" i]');
		if (!e) return null;
		const r = e.getBoundingClientRect();
		return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
	})()`
	clickedAdd, err := flowWaitAndClick(ctx, client, addTrigger, 10*time.Second)
	if err != nil || !clickedAdd {
		return errors.New("Flow add ingredient button was not found in composer")
	}
	time.Sleep(1 * time.Second)

	// 2. Click Upload button inside the popover
	uploadBtn := `(() => {
		const xs = [...document.querySelectorAll('button.sidebar-upload-btn, button[aria-label*="Tải nội dung" i], button[aria-label*="Upload" i], .add-menu-popover-container button')];
		const e = xs.find(x => /upload|tải nội dung/i.test((x.innerText||'') + ' ' + (x.getAttribute('aria-label')||'')));
		if (!e) return null;
		const r = e.getBoundingClientRect();
		return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
	})()`
	clickedUpload, err := flowWaitAndClick(ctx, client, uploadBtn, 10*time.Second)
	if err != nil || !clickedUpload {
		return errors.New("Flow upload media button was not found in add menu")
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

func flowAttachmentCount(ctx context.Context, client *browserCDPClient) (int, error) {
	value, err := client.evaluate(ctx, `(() => {
		const chips = [...document.querySelectorAll('flow-prompt-box flow-ingredient-chip, flow-composer flow-ingredient-chip, .base-prompt-box flow-ingredient-chip, .prompt-ingredient-bar flow-ingredient-chip, flow-image-ingredient-chip, flow-video-ingredient-chip, img.chip-image, .prompt-ingredient-bar img, [data-testid*="attachment" i], [data-testid*="ingredient" i]')];
		return chips.filter(e => {
			const r = e.getBoundingClientRect();
			return r.width > 0 && r.height > 0;
		}).length;
	})()`)
	if err != nil {
		return 0, err
	}
	n, ok := value.(float64)
	if !ok {
		return 0, errors.New("Flow attachment state is unreadable")
	}
	return int(n), nil
}

func flowAttachOneFile(ctx context.Context, client *browserCDPClient, path, kind string) error {
	cleanPath, err := filepath.Abs(path)
	if err == nil {
		path = filepath.Clean(cleanPath)
	}

	before, err := flowAttachmentCount(ctx, client)
	if err != nil {
		before = 0
	}

	// Clear previous injected file input if any
	_, _ = client.evaluate(ctx, `(() => {
		delete window.__lastFileInput;
		return true;
	})()`)

	if err := flowOpenUpload(ctx, client, kind); err != nil {
		return err
	}

	// Wait up to 8s for the hook to capture the Angular-created input
	inputReady, err := flowWaitForValue(ctx, client, `!!window.__lastFileInput`, 8*time.Second)
	if err != nil || inputReady != true {
		return fmt.Errorf("Flow %s upload control did not expose a file input", kind)
	}

	// Get input objectId via Runtime.evaluate
	evalResp, err := client.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    "window.__lastFileInput",
		"returnByValue": false,
	}, client.sessionID)
	if err != nil {
		return fmt.Errorf("Flow could not access %s file input: %w", kind, err)
	}
	var evalResult struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if json.Unmarshal(evalResp, &evalResult) != nil || evalResult.Result.ObjectID == "" {
		return fmt.Errorf("Flow %s file input object is invalid", kind)
	}

	// Set file via CDP
	if _, err := client.call(ctx, "DOM.setFileInputFiles", map[string]any{
		"files":    []string{path},
		"objectId": evalResult.Result.ObjectID,
	}, client.sessionID); err != nil {
		return fmt.Errorf("Flow could not attach %s reference: %w", kind, err)
	}

	// Dispatch change and input events to trigger Angular upload pipeline
	_, _ = client.evaluate(ctx, `(() => {
		const inp = window.__lastFileInput;
		if (!inp) return false;
		inp.dispatchEvent(new Event('change', { bubbles: true }));
		inp.dispatchEvent(new Event('input', { bubbles: true }));
		return true;
	})()`)

	// Handle consent dialog if it appears ("Quyền sử dụng hình ảnh này" / "Tôi đồng ý")
	time.Sleep(1 * time.Second)
	_, _ = flowWaitAndClick(ctx, client, `(() => {
		const dlg = document.querySelector('flow-upload-consent-dialog');
		if (!dlg) return null;
		const btns = [...dlg.querySelectorAll('button')];
		const e = btns.find(b => /đồng ý|agree|accept|tiếp tục|continue/i.test((b.innerText||'') + ' ' + (b.getAttribute('aria-label')||'')));
		if (!e) return null;
		const r = e.getBoundingClientRect();
		return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
	})()`, 5*time.Second)

	// Wait up to 50s for the upload to complete and detail-add-to-prompt-btn ("Thêm vào câu lệnh") to become ready.
	clickedAddToPrompt, err := flowWaitAndClick(ctx, client, `(() => {
		const btn = document.querySelector('button.detail-add-to-prompt-btn, button[aria-label*="Thêm vào câu lệnh" i]');
		if (!btn || btn.disabled || btn.getAttribute('aria-disabled') === 'true') return null;
		const r = btn.getBoundingClientRect();
		return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
	})()`, 50*time.Second)

	// Fallback: if detail button is not yet visible, check if an asset item is present in the list and click it to reveal detail
	if err != nil || !clickedAddToPrompt {
		_, _ = flowWaitAndClick(ctx, client, `(() => {
			const item = document.querySelector('flow-add-menu-asset-list button.asset-item');
			if (!item) return null;
			const r = item.getBoundingClientRect();
			return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
		})()`, 5*time.Second)

		clickedAddToPrompt, _ = flowWaitAndClick(ctx, client, `(() => {
			const btn = document.querySelector('button.detail-add-to-prompt-btn, button[aria-label*="Thêm vào câu lệnh" i]');
			if (!btn || btn.disabled || btn.getAttribute('aria-disabled') === 'true') return null;
			const r = btn.getBoundingClientRect();
			return (r.width > 0 && r.height > 0) ? {x: r.x + r.width/2, y: r.y + r.height/2} : null;
		})()`, 15*time.Second)
	}

	if !clickedAddToPrompt {
		return fmt.Errorf("Flow %s upload did not produce 'Add to prompt' action", kind)
	}
	time.Sleep(1 * time.Second)

	// Close add menu popover with Escape
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "rawKeyDown", "windowsVirtualKeyCode": 27}, client.sessionID)
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "windowsVirtualKeyCode": 27}, client.sessionID)

	// Confirm attachment count increased or ingredient chip is present in composer
	expression := fmt.Sprintf(`(() => {
		const chips = [...document.querySelectorAll('flow-prompt-box flow-ingredient-chip, flow-composer flow-ingredient-chip, .base-prompt-box flow-ingredient-chip, .prompt-ingredient-bar flow-ingredient-chip, flow-image-ingredient-chip, flow-video-ingredient-chip, img.chip-image, .prompt-ingredient-bar img, [data-testid*="attachment" i], [data-testid*="ingredient" i]')];
		const count = chips.filter(e => {
			const r = e.getBoundingClientRect();
			return r.width > 0 && r.height > 0;
		}).length;
		if (count > %d) return true;
		const bar = document.querySelector('flow-ingredient-bar, .prompt-ingredient-bar, .has-ingredient-bar');
		return !!(bar && bar.querySelector('img, button, flow-ingredient-chip, flow-image-ingredient-chip'));
	})()`, before)
	if _, err := flowWaitForValue(ctx, client, expression, 25*time.Second); err != nil {
		count, _ := flowAttachmentCount(ctx, client)
		if count <= before && count == 0 {
			return fmt.Errorf("Flow did not confirm the %s reference attachment", kind)
		}
	}
	return nil
}

func flowAttachRequestFiles(ctx context.Context, client *browserCDPClient, request flowHeadlessRequest) error {
	if request.Mode == "video_edit" {
		if err := flowAttachOneFile(ctx, client, request.VideoPath, "video"); err != nil {
			return fmt.Errorf("Flow source video: %w", err)
		}
	}
	for i, path := range request.ImagePaths {
		if err := flowAttachOneFile(ctx, client, path, "image"); err != nil {
			return fmt.Errorf("Flow reference image %d: %w", i+1, err)
		}
	}
	minimum := len(request.ImagePaths)
	if request.Mode == "video_edit" {
		minimum++
	}
	count, err := flowAttachmentCount(ctx, client)
	if err != nil || count < minimum {
		return errors.New("Flow did not confirm all source and character references")
	}
	return nil
}
