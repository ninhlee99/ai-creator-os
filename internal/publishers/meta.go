package publishers

// Meta Graph API (Facebook Page · Instagram Reels · Threads) — gọi API
// chính thức bằng net/http, không cần CLI ngoài.
//
// Cấu hình (per-account qua hậu tố _<USERNAME>, giống FB_PAGE_ID):
//   - META_PAGE_TOKEN: Page access token (quyền pages_manage_posts,
//     instagram_content_publish). Dùng chung cho Facebook Page và
//     Instagram Business/Creator nối với Page đó.
//   - FB_PAGE_ID: ID Trang Facebook.
//   - IG_USER_ID: ID tài khoản Instagram Business/Creator (nối với Page).
//   - THREADS_USER_ID + THREADS_TOKEN: Threads API (graph.threads.net).
//
// Giới hạn trung thực của từng nền tảng (không code nào vượt qua được):
//   - Instagram không có "nháp" qua API: Reels đăng là công khai. Vì vậy
//     IG chỉ đăng khi IG_AUTO_PUBLISH=1; mặc định chỉ tải lên + tạo
//     container (kiểm tra được) rồi dừng, báo Draft=true.
//   - Threads API chỉ nhận video qua URL công khai — app chạy local nên
//     Threads đăng bài CHỮ kèm link (tiêu đề + mô tả có link affiliate /
//     link video YouTube) để kéo traffic chéo, không tải video.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Các base URL là biến để test trỏ về httptest.
var (
	metaGraphURL   = "https://graph.facebook.com/v21.0"
	metaVideoURL   = "https://graph-video.facebook.com/v21.0"
	metaRuploadURL = "https://rupload.facebook.com/ig-api-upload/v21.0"
	threadsURL     = "https://graph.threads.net/v1.0"
	metaHTTP       = &http.Client{Timeout: 10 * time.Minute}
	// igPollEvery/igPollMax: chờ Instagram xử lý video container.
	igPollEvery = 5 * time.Second
	igPollMax   = 60
)

// metaPageToken trả Page access token của account ("" = chưa cấu hình).
func metaPageToken(username string) string { return envFor(username, "META_PAGE_TOKEN") }

// FacebookPageID trả ID Trang của account (FB_PAGE_ID_<USER> rồi FB_PAGE_ID).
func FacebookPageID(username string) string { return envFor(username, "FB_PAGE_ID") }

// FacebookPageConfigured requires both page identity and a usable page token.
func FacebookPageConfigured(username string) bool {
	return FacebookPageID(username) != "" && metaPageToken(username) != ""
}

// metaDo gửi request và giải mã JSON; lỗi Graph ({"error":{...}}) → error.
func metaDo(req *http.Request) (map[string]any, error) {
	resp, err := metaHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var m map[string]any
	if jerr := json.Unmarshal(body, &m); jerr != nil {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if e, ok := m["error"].(map[string]any); ok {
		msg, _ := e["message"].(string)
		return m, fmt.Errorf("meta API: %s", truncate(msg, 200))
	}
	if resp.StatusCode >= 300 {
		return m, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return m, nil
}

// metaPostForm POST form-urlencoded tới endpoint.
func metaPostForm(ctx context.Context, endpoint string, form url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return metaDo(req)
}

func strField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// postText ghép tiêu đề + mô tả thành caption (cắt theo giới hạn n rune).
func postText(title, description string, n int) string {
	return truncate(strings.TrimSpace(title+"\n\n"+description), n)
}

// ---------------------------------------------------------------- Facebook

// publishFacebookGraph tải video lên Trang qua graph-video (multipart,
// file local). draftOnly → published=false (video chưa công khai, duyệt
// trong Meta Business Suite).
func publishFacebookGraph(ctx context.Context, pageID, token, videoPath, title, description string, draftOnly bool) PublishResult {
	res := PublishResult{Platform: "facebook", Draft: draftOnly}
	f, err := os.Open(videoPath)
	if err != nil {
		res.Error = "mở video: " + err.Error()
		return res
	}
	defer f.Close()
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		fields := map[string]string{
			"access_token": token,
			"title":        truncate(title, 255),
			"description":  postText(title, description, 5000),
			"published":    fmt.Sprint(!draftOnly),
		}
		for k, v := range fields {
			if werr := mw.WriteField(k, v); werr != nil {
				pw.CloseWithError(werr)
				return
			}
		}
		part, werr := mw.CreateFormFile("source", filepath.Base(videoPath))
		if werr == nil {
			_, werr = io.Copy(part, f)
		}
		if werr == nil {
			werr = mw.Close()
		}
		pw.CloseWithError(werr)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		metaVideoURL+"/"+url.PathEscape(pageID)+"/videos", pr)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	m, err := metaDo(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.RemoteID = strField(m, "id")
	if res.RemoteID == "" {
		res.Error = "facebook: không nhận được video id"
		return res
	}
	res.Ok = true
	res.URL = "https://www.facebook.com/" + res.RemoteID
	return res
}

// --------------------------------------------------------------- Instagram

// instagramKinds: Reels nhận video dọc ngắn.
var instagramKinds = []string{"short_video", "short_film"}

// InstagramPublisher đăng Reels qua Instagram Graph API (resumable upload
// từ file local — không cần URL công khai).
type InstagramPublisher struct {
	Username    string
	igUserID    string
	token       string
	autoPublish bool
}

// NewInstagramPublisher đọc IG_USER_ID, META_PAGE_TOKEN, IG_AUTO_PUBLISH.
func NewInstagramPublisher(username string) *InstagramPublisher {
	return &InstagramPublisher{
		Username:    username,
		igUserID:    envFor(username, "IG_USER_ID"),
		token:       metaPageToken(username),
		autoPublish: envFor(username, "IG_AUTO_PUBLISH") == "1",
	}
}

func (p *InstagramPublisher) Name() string             { return "instagram" }
func (p *InstagramPublisher) IsConfigured() bool       { return p.igUserID != "" && p.token != "" }
func (p *InstagramPublisher) Handles(kind string) bool { return handlesKind(kind, instagramKinds) }

// Publish: tạo container REELS (resumable) → upload byte → chờ FINISHED
// → media_publish (chỉ khi IG_AUTO_PUBLISH=1).
func (p *InstagramPublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	res := PublishResult{Platform: p.Name()}
	if !p.IsConfigured() {
		res.Error = "instagram chưa cấu hình (IG_USER_ID + META_PAGE_TOKEN)"
		return res
	}
	st, err := os.Stat(videoPath)
	if err != nil {
		res.Error = "mở video: " + err.Error()
		return res
	}
	// 1. Container.
	m, err := metaPostForm(ctx, metaGraphURL+"/"+url.PathEscape(p.igUserID)+"/media", url.Values{
		"media_type":    {"REELS"},
		"upload_type":   {"resumable"},
		"caption":       {postText(title, description, 2200)},
		"share_to_feed": {"true"},
		"access_token":  {p.token},
	})
	if err != nil {
		res.Error = "tạo container: " + err.Error()
		return res
	}
	cid := strField(m, "id")
	if cid == "" {
		res.Error = "instagram: không nhận được container id"
		return res
	}
	res.RemoteID = cid
	// 2. Upload byte (rupload).
	f, err := os.Open(videoPath)
	if err != nil {
		res.Error = "mở video: " + err.Error()
		return res
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metaRuploadURL+"/"+url.PathEscape(cid), f)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.ContentLength = st.Size()
	req.Header.Set("Authorization", "OAuth "+p.token)
	req.Header.Set("offset", "0")
	req.Header.Set("file_size", fmt.Sprint(st.Size()))
	if _, err := metaDo(req); err != nil {
		res.Error = "upload video: " + err.Error()
		return res
	}
	// 3. Chờ xử lý.
	if err := p.waitFinished(ctx, cid); err != nil {
		res.Error = err.Error()
		return res
	}
	if !p.autoPublish {
		// IG không có nháp — dừng ở container đã xử lý xong.
		res.Ok, res.Draft = true, true
		return res
	}
	// 4. Publish.
	m, err = metaPostForm(ctx, metaGraphURL+"/"+url.PathEscape(p.igUserID)+"/media_publish", url.Values{
		"creation_id":  {cid},
		"access_token": {p.token},
	})
	if err != nil {
		res.Error = "media_publish: " + err.Error()
		return res
	}
	res.Ok = true
	res.RemoteID = strField(m, "id")
	return res
}

func (p *InstagramPublisher) waitFinished(ctx context.Context, cid string) error {
	for i := 0; i < igPollMax; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			metaGraphURL+"/"+url.PathEscape(cid)+"?fields=status_code&access_token="+url.QueryEscape(p.token), nil)
		if err != nil {
			return err
		}
		m, err := metaDo(req)
		if err != nil {
			return fmt.Errorf("kiểm tra container: %w", err)
		}
		switch strField(m, "status_code") {
		case "FINISHED":
			return nil
		case "ERROR", "EXPIRED":
			return fmt.Errorf("instagram xử lý video lỗi (%s)", strField(m, "status_code"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(igPollEvery):
		}
	}
	return fmt.Errorf("instagram xử lý video quá lâu")
}

// ----------------------------------------------------------------- Threads

// threadsKinds: Threads nhận bài chữ kèm link cho mọi video ngắn/dài.
var threadsKinds = []string{"short_video", "short_film"}

// ThreadsPublisher đăng bài chữ (tiêu đề + mô tả + link) lên Threads.
type ThreadsPublisher struct {
	Username string
	userID   string
	token    string
}

// NewThreadsPublisher đọc THREADS_USER_ID + THREADS_TOKEN.
func NewThreadsPublisher(username string) *ThreadsPublisher {
	return &ThreadsPublisher{
		Username: username,
		userID:   envFor(username, "THREADS_USER_ID"),
		token:    envFor(username, "THREADS_TOKEN"),
	}
}

func (p *ThreadsPublisher) Name() string             { return "threads" }
func (p *ThreadsPublisher) IsConfigured() bool       { return p.userID != "" && p.token != "" }
func (p *ThreadsPublisher) Handles(kind string) bool { return handlesKind(kind, threadsKinds) }

// Publish tạo container TEXT rồi threads_publish. videoPath không dùng
// (Threads API chỉ nhận video qua URL công khai).
func (p *ThreadsPublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	res := PublishResult{Platform: p.Name()}
	if !p.IsConfigured() {
		res.Error = "threads chưa cấu hình (THREADS_USER_ID + THREADS_TOKEN)"
		return res
	}
	text := postText(title, description, 500)
	if text == "" {
		res.Error = "threads: bài rỗng"
		return res
	}
	base := threadsURL + "/" + url.PathEscape(p.userID)
	m, err := metaPostForm(ctx, base+"/threads", url.Values{
		"media_type": {"TEXT"}, "text": {text}, "access_token": {p.token},
	})
	if err != nil {
		res.Error = "tạo bài: " + err.Error()
		return res
	}
	cid := strField(m, "id")
	if cid == "" {
		res.Error = "threads: không nhận được container id"
		return res
	}
	m, err = metaPostForm(ctx, base+"/threads_publish", url.Values{
		"creation_id": {cid}, "access_token": {p.token},
	})
	if err != nil {
		res.Error = "đăng bài: " + err.Error()
		return res
	}
	res.Ok = true
	res.RemoteID = strField(m, "id")
	return res
}
