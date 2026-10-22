package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	header := cookieHeaderForHost(cookies.Cookies, "google.com")
	if !strings.Contains(header, "__Secure-1PSID=") {
		return ""
	}
	return header
}

func flowSelectModelAndMode(ctx context.Context, client *browserCDPClient, request flowHeadlessRequest) error {
	modeLabels := []string{"ingredients to video", "ingredients"}
	if request.Mode == "video_edit" {
		modeLabels = []string{"video to video", "video-to-video", "edit video", "video editing"}
	}
	if !flowHasSelectedLabel(ctx, client, modeLabels) {
		_, _ = flowClickNamed(ctx, client, []string{"text to video", "ingredients", "video to video", "video-to-video", "edit video"}, 8*time.Second)
		clicked, err := flowClickNamed(ctx, client, modeLabels, 8*time.Second)
		if err != nil || !clicked {
			return fmt.Errorf("Flow %s mode is not available", request.Mode)
		}
		if !flowHasSelectedLabel(ctx, client, modeLabels) {
			return fmt.Errorf("Flow did not confirm %s mode", request.Mode)
		}
	}

	modelLabels := []string{strings.ToLower(request.ModelLabel)}
	if !flowHasSelectedLabel(ctx, client, modelLabels) {
		_, _ = flowClickNamed(ctx, client, []string{"model", "veo", "gemini omni"}, 8*time.Second)
		clicked, err := flowClickNamed(ctx, client, modelLabels, 8*time.Second)
		if err != nil || !clicked {
			return fmt.Errorf("Flow model %q is not available", request.ModelLabel)
		}
		if !flowHasSelectedLabel(ctx, client, modelLabels) {
			return fmt.Errorf("Flow did not confirm model %q", request.ModelLabel)
		}
	}
	return nil
}

func flowClickNamed(ctx context.Context, client *browserCDPClient, labels []string, timeout time.Duration) (bool, error) {
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};const xs=[...document.querySelectorAll('button,[role="button"],[role="menuitem"],[role="option"],[role="combobox"]')].filter(visible);const e=xs.find(x=>{const t=((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('title')||'')).toLowerCase();return labels.some(s=>t.includes(s))&&!x.disabled&&x.getAttribute('aria-disabled')!=='true'});if(!e)return false;e.click();return true})()`
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
	expression := `(() => {const labels=` + string(encoded) + `;const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};return [...document.querySelectorAll('button,[role="button"],[role="combobox"],[aria-selected="true"],[aria-pressed="true"]')].some(e=>{if(!visible(e)||e.disabled||e.getAttribute('aria-expanded')==='true')return false;const t=((e.innerText||'')+' '+(e.getAttribute('aria-label')||'')+' '+(e.getAttribute('title')||'')).toLowerCase();if(!labels.some(s=>t.includes(s)))return false;return e.getAttribute('aria-selected')==='true'||e.getAttribute('aria-pressed')==='true'||e.getAttribute('data-state')==='checked'||/selected|active|checked/.test(e.className||'')||e.matches('button,[role="combobox"]')})})()`
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
		inputs = append(inputs, flowFileInput{NodeID: nodeID, Accept: strings.ToLower(attrs["accept"]), Multiple: attrs["multiple"] != ""})
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
	labels := []string{"upload", "attach", "add reference", "add ingredient", "add image"}
	if kind == "video" {
		labels = []string{"upload", "attach", "add video", "add source video"}
	}
	if _, err := flowClickNamed(ctx, client, labels, 10*time.Second); err != nil {
		return err
	}
	if input, found, err := flowFindFileInput(ctx, client, kind); err != nil {
		return err
	} else if found && input.NodeID != 0 {
		return nil
	}
	return fmt.Errorf("Flow %s upload control did not expose a file input", kind)
}

func flowAttachmentCount(ctx context.Context, client *browserCDPClient) (int, error) {
	value, err := client.evaluate(ctx, `(() => {const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0};const xs=[...document.querySelectorAll('img,video,[style*="blob:"],[data-testid*="attachment" i],[data-testid*="ingredient" i]')];return xs.filter(e=>visible(e)&&((e.getAttribute('src')||'').startsWith('blob:')||(e.getAttribute('src')||'').startsWith('data:image/')||(e.getAttribute('style')||'').includes('blob:')||/attachment|ingredient/i.test(e.getAttribute('data-testid')||''))).length})()`)
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
	input, found, err := flowFindFileInput(ctx, client, kind)
	if err != nil {
		return err
	}
	if !found {
		if err := flowOpenUpload(ctx, client, kind); err != nil {
			return err
		}
		input, found, err = flowFindFileInput(ctx, client, kind)
		if err != nil || !found {
			return fmt.Errorf("Flow %s file input is unavailable", kind)
		}
	}
	before, err := flowAttachmentCount(ctx, client)
	if err != nil {
		return err
	}
	if _, err := client.call(ctx, "DOM.setFileInputFiles", map[string]any{"files": []string{path}, "nodeId": input.NodeID}, client.sessionID); err != nil {
		return fmt.Errorf("Flow could not attach %s reference", kind)
	}
	expression := fmt.Sprintf(`(() => {const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0};return [...document.querySelectorAll('img,video,[style*="blob:"],[data-testid*="attachment" i],[data-testid*="ingredient" i]')].filter(e=>visible(e)&&((e.getAttribute('src')||'').startsWith('blob:')||(e.getAttribute('src')||'').startsWith('data:image/')||(e.getAttribute('style')||'').includes('blob:')||/attachment|ingredient/i.test(e.getAttribute('data-testid')||''))).length>%d})()`, before)
	if _, err := flowWaitForValue(ctx, client, expression, 30*time.Second); err != nil {
		return fmt.Errorf("Flow did not confirm the %s reference attachment", kind)
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

func flowBrowserCookies(header string) []map[string]any {
	var out []map[string]any
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		for _, cookieURL := range []string{flowAppURL, "https://labs.google/"} {
			out = append(out, map[string]any{"name": name, "value": value, "url": cookieURL, "path": "/", "secure": true, "httpOnly": true})
		}
	}
	return out
}
