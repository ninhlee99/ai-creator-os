package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type imageAnalysisPage struct {
	url             string
	composer        string
	responses       string
	busy            string
	imageURL        string
	videoURL        string
	imageModeLabels []string
	videoModeLabels []string
}

var imageAnalysisPages = map[string]imageAnalysisPage{
	PlatformChatGPT: {
		url: "https://chatgpt.com/", composer: "#prompt-textarea, [data-testid='prompt-textarea'], textarea, [contenteditable='true']",
		responses: "[data-message-author-role='assistant']", busy: "[data-testid*='stop'], button[aria-label*='Stop generating' i]",
	},
	PlatformClaude: {
		url: "https://claude.ai/new", composer: "[contenteditable='true'], [data-testid*='chat-input'] textarea, textarea",
		responses: ".font-claude-response", busy: "button[aria-label*='Stop' i], [data-testid*='stop']",
	},
	PlatformGemini: {
		url: "https://gemini.google.com/app", composer: "rich-textarea [contenteditable='true'], .ql-editor, textarea, [contenteditable='true']",
		responses: "model-response, .response-container", busy: "button[aria-label*='Stop' i], mat-spinner",
		imageModeLabels: []string{"create image", "create images"}, videoModeLabels: []string{"create video"},
	},
	PlatformGrok: {
		url: "https://grok.com/", composer: "textarea, [contenteditable='true']",
		responses: "main [data-message-author-role='assistant'], main [data-role='assistant'], main [data-testid*='assistant' i], [data-testid*='conversation-turn'], main article", busy: "button[aria-label*='Stop' i], [data-testid*='stop']",
		imageURL: "https://grok.com/imagine", videoURL: "https://grok.com/imagine",
		imageModeLabels: []string{"image"}, videoModeLabels: []string{"video"},
	},
}

// AnalyzeImagesInWebUI submits local image files and prompt through the
// provider's signed-in website. It uses no provider chat API or undocumented
// web endpoint.
func AnalyzeImagesInWebUI(ctx context.Context, platform, cookie string, savedCookies []BrowserCookie, prompt string, imagePaths []string, updater CookieSnapshotUpdater) (answer string, retErr error) {
	if len(imagePaths) == 0 {
		return "", errors.New("Web AI image analysis needs 1-10 images")
	}
	result, err := UseWebAIInWebUI(ctx, platform, cookie, savedCookies, WebAIOperationRequest{
		Prompt: prompt, Files: imagePaths, OutputType: "text",
		requireRadarMarkers: strings.Contains(strings.ToLower(prompt), "video_index") || strings.Contains(strings.ToLower(prompt), "dance_detected"),
	}, updater)
	return result.Text, err
}

func imageAnalysisCookieHeader(platform string, cookies []BrowserCookie) string {
	hosts := map[string]string{
		PlatformChatGPT: "chatgpt.com", PlatformClaude: "claude.ai",
		PlatformGemini: "gemini.google.com", PlatformGrok: "grok.com",
	}
	return browserCookieHeaderAllPaths(cookies, BrowserCookieScope{Platform: platform, CookieHost: hosts[platform]})
}

func waitForWebAIComposer(ctx context.Context, client *browserCDPClient, selector string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		value, err := client.evaluate(ctx, `(() => [...document.querySelectorAll(`+mustJSON(selector)+`)].some(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&!e.disabled&&getComputedStyle(e).visibility!=='hidden'}))()`)
		if err == nil && value == true {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return errors.New("composer wait timed out")
}

func webAIResponseExpression(selector, prompt string, includeBusy string, requireRadarMarkers ...bool) string {
	requireRadar := len(requireRadarMarkers) > 0 && requireRadarMarkers[0]
	return `(() => {
		const normalize=s=>(s||'').replace(/\s+/g,' ').trim();
		const promptText=normalize(` + mustJSON(prompt) + `);
		const requireRadarMarkers=` + strconv.FormatBool(requireRadar) + `;
		const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'};
		const assistant=e=>e.querySelector("[data-message-author-role='assistant'],[data-role='assistant'],[data-testid*='assistant' i]")||e;
		const media=e=>({
			images:[...e.querySelectorAll('img')].filter(x=>x.complete&&x.naturalWidth>=128&&x.naturalHeight>=128).map(x=>x.currentSrc||x.src).filter(Boolean),
			videos:[...e.querySelectorAll('video,video source,a[href]')].map(x=>x.currentSrc||x.src||x.href||'').filter(u=>/^(blob:|data:video\/|https?:)/i.test(u)&&(/\.(mp4|webm|mov)(?:[?#]|$)/i.test(u)||/^blob:|^data:video\//i.test(u)))
		});
		const xs=[...new Set([...document.querySelectorAll(` + mustJSON(selector) + `)].map(assistant))].filter(e=>{
			if(!visible(e))return false;
			for(let n=e;n&&n!==document.body;n=n.parentElement){
				const role=((n.getAttribute('data-message-author-role')||n.getAttribute('data-role')||'').toLowerCase());
				const id=(n.getAttribute('data-testid')||'').toLowerCase();
				if(role==='user'||role==='human'||/(^|[-_ ])(?:user|human)[-_ ]?(?:message|turn)?(?:$|[-_ ])/i.test(id))return false;
			}
			const raw=normalize(e.innerText||e.textContent||'');
			const text=promptText?normalize(raw.split(promptText).join(' ')):raw;
			const artifacts=media(e);
		if(requireRadarMarkers)return text.length>0&&/dance_detected|video_index/i.test(text);
		return text.length>0||artifacts.images.length>0||artifacts.videos.length>0;
		});
		const busy=[...document.querySelectorAll(` + includeBusy + `)].some(visible);
		const latest=xs.length?xs[xs.length-1]:null;
		const raw=latest?(latest.innerText||latest.textContent||''):'';
		const text=promptText?normalize(raw).split(promptText).join(' ').trim():raw;
		const artifacts=latest?media(latest):{images:[],videos:[]};
		return {count:xs.length,text,busy,images:artifacts.images,videos:artifacts.videos};
	})()`
}

func webAIResponseCount(ctx context.Context, client *browserCDPClient, selector, prompt string, requireRadarMarkers ...bool) (int, error) {
	value, err := client.evaluate(ctx, webAIResponseExpression(selector, prompt, mustJSON(":root:disabled"), requireRadarMarkers...))
	if err != nil {
		return 0, err
	}
	var state struct {
		Count int `json:"count"`
	}
	raw, marshalErr := json.Marshal(value)
	if marshalErr != nil || json.Unmarshal(raw, &state) != nil {
		return 0, errors.New("response count is unreadable")
	}
	return state.Count, nil
}

func attachWebAIFiles(ctx context.Context, client *browserCDPClient, paths []string) error {
	inputs, err := flowFileInputs(ctx, client)
	if err != nil {
		return err
	}
	selected := selectWebAIFileInput(inputs, paths)
	if selected == nil {
		if err := openWebAIAttachmentMenu(ctx, client); err != nil {
			return errors.New("file input not found")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		inputs, err = flowFileInputs(ctx, client)
		if err != nil {
			return err
		}
		selected = selectWebAIFileInput(inputs, paths)
	}
	if selected == nil {
		return errors.New("provider has no file input accepting all selected files")
	}
	if _, err := client.call(ctx, "DOM.setFileInputFiles", map[string]any{"files": paths, "nodeId": selected.NodeID}, client.sessionID); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1500 * time.Millisecond):
	}
	return nil
}

func selectWebAIFileInput(inputs []flowFileInput, paths []string) *flowFileInput {
	var fallback *flowFileInput
	for i := range inputs {
		input := &inputs[i]
		if !webAIInputAcceptsPaths(*input, paths) {
			continue
		}
		if fallback == nil {
			fallback = input
		}
		if input.Multiple || len(paths) == 1 {
			return input
		}
	}
	if fallback != nil && len(paths) == 1 {
		return fallback
	}
	return nil
}

func webAIInputAcceptsPaths(input flowFileInput, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	if input.Accept == "" || strings.Contains(input.Accept, "*/*") {
		return true
	}
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		kind := ""
		if strings.Contains(" .png .jpg .jpeg .webp ", " "+ext+" ") {
			kind = "image"
		} else if strings.Contains(" .mp4 .mov .webm .m4v ", " "+ext+" ") {
			kind = "video"
		} else {
			return false
		}
		matched := false
		for _, accepted := range strings.Split(strings.ToLower(input.Accept), ",") {
			accepted = strings.TrimSpace(accepted)
			if accepted == "" || accepted == "*/*" || strings.HasPrefix(accepted, kind+"/") || accepted == ext {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func openWebAIAttachmentMenu(ctx context.Context, client *browserCDPClient) error {
	labels := []string{"attach", "add file", "add photo", "upload", "tải tệp", "đính kèm", "thêm ảnh"}
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const e=[...document.querySelectorAll('button,[role="button"]')].find(x=>{const r=x.getBoundingClientRect();const t=((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('title')||'')).toLowerCase();return r.width>0&&r.height>0&&!x.disabled&&labels.some(s=>t.includes(s))});if(!e)return false;e.click();return true})()`
	if value, err := client.evaluate(ctx, expression); err == nil && value == true {
		return nil
	}
	return errors.New("attachment menu not found")
}

func setWebAIPrompt(ctx context.Context, client *browserCDPClient, selector, prompt string) error {
	expression := `(() => {const e=[...document.querySelectorAll(` + mustJSON(selector) + `)].find(x=>{const r=x.getBoundingClientRect();return r.width>0&&r.height>0&&!x.disabled&&getComputedStyle(x).visibility!=='hidden'});if(!e)return false;const p=` + mustJSON(prompt) + `;e.focus();if(e instanceof HTMLTextAreaElement||e instanceof HTMLInputElement){const proto=e instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;Object.getOwnPropertyDescriptor(proto,'value').set.call(e,p);e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));e.dispatchEvent(new Event('change',{bubbles:true}));}else{e.textContent='';document.execCommand('insertText',false,p);e.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:p}));}return true})()`
	value, err := client.evaluate(ctx, expression)
	if err != nil || value != true {
		return errors.New("composer rejected prompt")
	}
	return nil
}

func submitWebAIPrompt(ctx context.Context, client *browserCDPClient, platform string) error {
	labels := []string{"send", "submit", "send message", "submit prompt", "gửi"}
	encoded, _ := json.Marshal(labels)
	expression := `(() => {const labels=` + string(encoded) + `;const e=[...document.querySelectorAll('button,[role="button"]')].find(x=>{const r=x.getBoundingClientRect();const t=((x.innerText||'')+' '+(x.getAttribute('aria-label')||'')+' '+(x.getAttribute('data-testid')||'')).toLowerCase();return r.width>0&&r.height>0&&!x.disabled&&x.getAttribute('aria-disabled')!=='true'&&labels.some(s=>t.includes(s))});if(!e)return false;e.click();return true})()`
	if value, err := client.evaluate(ctx, expression); err == nil && value == true {
		return nil
	}
	if _, err := client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID); err != nil {
		return fmt.Errorf("%s composer did not accept submit", platform)
	}
	_, _ = client.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, client.sessionID)
	return nil
}

func mustJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
