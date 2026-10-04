package reup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Downloader: yt-dlp trước, gãy → TikWM Lookup (CHỈ link wmplay có
// watermark).
//
// RANH GIỚI CỨNG (Đợt I): không bao giờ tải bản no-watermark của video
// người khác. yt-dlp giữ nguyên watermark gốc; TikWM fallback chỉ dùng
// link wmplay (giữ attribution tác giả) — thiếu wmplay thì fail-closed.
//
//   - Dedupe 2 lớp: douyin_id (trước tải — không tải lại) và sha256
//     (sau tải — URL khác nhưng cùng nội dung thì bỏ file trùng).
//   - QC sau tải, trung thực về phương pháp: có ffprobe → kiểm tra video
//     stream thật; không có ffprobe → chỉ kiểm tra size + phần mở rộng,
//     ghi qc_method="size-only" (không đoán là "đạt chuẩn").
//   - Fail-closed: QC rớt → status=failed + lý do, không đưa vào kho.
// ---------------------------------------------------------------------------

// Downloader tải video Douyin về outDir.
type Downloader struct {
	YtDlp *Manager
	TikWM *TikWM
	Store *Store
	// OutDir là thư mục chứa video đã tải.
	OutDir string
	http   *http.Client
}

// NewDownloader tạo downloader.
func NewDownloader(yt *Manager, tw *TikWM, store *Store, outDir string) *Downloader {
	return &Downloader{
		YtDlp:  yt,
		TikWM:  tw,
		Store:  store,
		OutDir: outDir,
		http:   &http.Client{Timeout: downloadTimeout},
	}
}

// DownloadVideo tải 1 video từ URL Douyin share (trích douyin_id từ URL
// khi được). Trả về Video đã lưu trong store.
func (d *Downloader) DownloadVideo(ctx context.Context, url string) (Video, error) {
	return d.download(ctx, ExtractDouyinID(url), url)
}

// DownloadCandidate tải 1 candidate từ discover: ưu tiên douyin_id của
// candidate để nối đúng bản ghi đã queue. Luôn tải qua URL share gốc
// (yt-dlp → TikWM wmplay), không tải link CDN trực tiếp.
func (d *Downloader) DownloadCandidate(ctx context.Context, c Candidate) (Video, error) {
	return d.download(ctx, c.DouyinID, c.URL)
}

// download là lõi chung của DownloadVideo/DownloadCandidate.
func (d *Downloader) download(ctx context.Context, douyinID, url string) (Video, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Video{}, fmt.Errorf("reup: URL trống")
	}
	if d.Store == nil {
		return Video{}, fmt.Errorf("reup: kho chưa sẵn sàng")
	}
	if err := os.MkdirAll(d.OutDir, 0o755); err != nil {
		return Video{}, fmt.Errorf("reup: tạo thư mục tải: %w", err)
	}

	// Dedupe lớp 1: douyin_id.
	//  - downloaded/downloading → trả về luôn, không tải lại.
	//  - queued/failed → dùng lại bản ghi, tiến hành tải.
	var rec Video
	if douyinID != "" {
		if ex, err := d.Store.GetByDouyinID(douyinID); err == nil {
			switch ex.Status {
			case StatusDownloaded, StatusDownloading:
				return ex, nil
			default:
				rec = ex
			}
		}
	}
	if rec.ID == 0 {
		q, qerr := d.Store.QueueVideo(Video{
			DouyinID: douyinID, URL: url, Status: StatusQueued,
		})
		if qerr != nil {
			return Video{}, qerr
		}
		rec = q
	}
	_ = d.Store.SetStatus(rec.ID, StatusDownloading, "")

	// Thử yt-dlp trước (auto-ensure binary theo mẫu sidecar).
	var (
		filePath    string
		dlDouyinID  string
		title       string
		duration    float64
		via         string
		wmFree      bool
		downloadErr error
	)
	ytOK := false
	if d.YtDlp != nil {
		if eerr := d.YtDlp.Ensure(ctx); eerr != nil {
			downloadErr = fmt.Errorf("yt-dlp chưa sẵn sàng (%v) — thử TikWM", eerr)
		} else {
			res, broken, derr := d.YtDlp.Download(ctx, url, d.OutDir)
			if derr == nil {
				filePath, dlDouyinID, title, duration = res.FilePath, res.VideoID, res.Title, res.Duration
				via, wmFree, ytOK = "ytdlp", false, true
			} else if broken {
				// Extractor gãy (Douyin đổi chữ ký): update 1 lần rồi thử lại.
				if uerr := d.YtDlp.Update(ctx); uerr == nil {
					if res2, _, derr2 := d.YtDlp.Download(ctx, url, d.OutDir); derr2 == nil {
						filePath, dlDouyinID, title, duration = res2.FilePath, res2.VideoID, res2.Title, res2.Duration
						via, wmFree, ytOK = "ytdlp", false, true
					} else {
						downloadErr = fmt.Errorf("yt-dlp vẫn lỗi sau update: %v — thử TikWM", derr2)
					}
				} else {
					downloadErr = fmt.Errorf("yt-dlp gãy + update thất bại (%v) — thử TikWM", uerr)
				}
			} else {
				downloadErr = fmt.Errorf("%v — thử TikWM", derr)
			}
		}
	}

	// Fallback TikWM — CHỈ link wmplay (có watermark, giữ attribution
	// tác giả gốc). Không bao giờ dùng bản no-watermark: Lookup thiếu
	// wmplay → fail-closed với lý do rõ ràng.
	if !ytOK {
		if d.TikWM == nil {
			ferr := joinErr(downloadErr, fmt.Errorf("reup: không có TikWM fallback"))
			d.fail(rec.ID, "", "tải thất bại: "+ferr.Error())
			return Video{}, ferr
		}
		v, lerr := d.TikWM.Lookup(ctx, url)
		if lerr != nil {
			ferr := joinErr(downloadErr, fmt.Errorf("tikwm lookup: %w", lerr))
			d.fail(rec.ID, "", "tải thất bại: "+ferr.Error())
			return Video{}, ferr
		}
		// Link wmplay có chữ ký, hết hạn sau vài phút → tải ngay.
		dst := filepath.Join(d.OutDir, "tikwm_"+v.ID+".mp4")
		if derr := d.fetchFile(ctx, v.WMPlayURL, dst); derr != nil {
			ferr := joinErr(downloadErr, fmt.Errorf("tikwm tải file: %w", derr))
			d.fail(rec.ID, "", "tải thất bại: "+ferr.Error())
			return Video{}, ferr
		}
		filePath, dlDouyinID, title, duration = dst, v.ID, v.Title, v.Duration
		via, wmFree = "tikwm", false // wmplay = có watermark
	}

	// Cập nhật douyin_id/title nếu lượt tải trích được id mà bản ghi chưa
	// có (bản ghi tạo từ URL trực tiếp không trích được id).
	if dlDouyinID != "" && rec.DouyinID == "" {
		_, _ = d.Store.db.Exec(`UPDATE reup_videos SET douyin_id=?, title=? WHERE id=?`,
			dlDouyinID, title, rec.ID)
	}

	// Dedupe lớp 2: sha256 sau tải.
	sha, serr := sha256File(filePath)
	if serr != nil {
		d.fail(rec.ID, filePath, "không đọc được file để tính sha256: "+serr.Error())
		return Video{}, fmt.Errorf("reup: %s", "không đọc được file để tính sha256: "+serr.Error())
	}
	if d.Store.HasSHA256(sha) {
		os.Remove(filePath)
		d.fail(rec.ID, "", "trùng nội dung với video đã có trong kho (sha256)")
		dup, _ := d.Store.GetVideo(rec.ID)
		return dup, nil
	}

	// QC: file + (ffprobe nếu có).
	qcMethod, resolution, qcErr := qcVideo(filePath)
	if qcErr != nil {
		d.fail(rec.ID, filePath, qcErr.Error())
		return Video{}, fmt.Errorf("reup: QC rớt: %w", qcErr)
	}
	if merr := d.Store.MarkDownloaded(rec.ID, filePath, sha, via, wmFree,
		duration, resolution, qcMethod); merr != nil {
		return Video{}, merr
	}
	return d.Store.GetVideo(rec.ID)
}

// fail đánh failed + xoá file rác (nếu còn).
func (d *Downloader) fail(id int64, filePath, reason string) {
	if filePath != "" {
		os.Remove(filePath)
	}
	_ = d.Store.SetStatus(id, StatusFailed, reason)
}

// Retry tải lại video đang failed (giữ nguyên bản ghi, không tạo mới).
func (d *Downloader) Retry(ctx context.Context, id int64) (Video, error) {
	v, err := d.Store.GetVideo(id)
	if err != nil {
		return Video{}, err
	}
	if v.Status != StatusFailed && v.Status != StatusQueued {
		return v, fmt.Errorf("reup: video #%d đang ở trạng thái %q — chỉ tải lại khi lỗi/chờ", id, v.StatusLabel())
	}
	_ = d.Store.SetStatus(id, StatusQueued, "")
	return d.download(ctx, v.DouyinID, v.URL)
}

// fetchFile tải URL vào dst (dùng cho link CDN TikWM hết hạn nhanh).
func (d *Downloader) fetchFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "aicos-reup/1.0")
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

// ----------------------------------------------------------------------- QC

// qcVideo kiểm tra file tải về. Trả về (phương pháp, độ phân giải, lỗi).
// Có ffprobe → kiểm tra có video stream thật (tin cậy).
// Không có ffprobe → chỉ kiểm tra size > 0 + đuôi mp4/mov/mkv/webm,
// ghi qc_method="size-only" trung thực (không khẳng định "đạt chuẩn").
func qcVideo(path string) (method, resolution string, err error) {
	st, serr := os.Stat(path)
	if serr != nil {
		return "", "", fmt.Errorf("file không tồn tại: %s", path)
	}
	if st.Size() == 0 {
		return "", "", fmt.Errorf("file rỗng (0 byte)")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".mov", ".mkv", ".webm":
	default:
		return "", "", fmt.Errorf("đuôi file lạ %q (không phải video)", ext)
	}
	if _, lookErr := exec.LookPath("ffprobe"); lookErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, rerr := exec.CommandContext(ctx, "ffprobe",
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=width,height,duration",
			"-of", "csv=p=0",
			path).Output()
		if rerr != nil {
			return "", "", fmt.Errorf("ffprobe không đọc được video stream: %v", rerr)
		}
		fields := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(fields) < 2 || fields[0] == "" || fields[1] == "" {
			return "", "", fmt.Errorf("ffprobe không thấy video stream")
		}
		return "ffprobe", fields[0] + "x" + fields[1], nil
	}
	return "size-only", "", nil
}

// sha256File tính sha256 của file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractDouyinID trích video id từ URL Douyin (dạng /video/<id>).
// Không trích được → "" (caller xử lý, không đoán).
func ExtractDouyinID(rawURL string) string {
	low := strings.ToLower(rawURL)
	idx := strings.Index(low, "/video/")
	if idx < 0 {
		return ""
	}
	rest := rawURL[idx+len("/video/"):]
	var b strings.Builder
	for _, r := range rest {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}

func joinErr(a, b error) error {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return fmt.Errorf("%v; %v", a, b)
}
