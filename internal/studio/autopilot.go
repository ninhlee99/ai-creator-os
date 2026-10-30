package studio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// Autopilot holds affiliate automation wiring. Production stays paused until
// account-level authorization for automatic Muse AI credit use exists.
type Autopilot struct {
	studio    *Studio
	accounts  *network.AccountManager
	store     *products.Store
	providers []products.Provider
	models    *ModelStore
}

// NewAutopilot wires the collaborators. Any of them may be nil only in
// tests; Run fails loudly when a required one is missing.
func NewAutopilot(st *Studio, accounts *network.AccountManager,
	store *products.Store, providers []products.Provider,
	models *ModelStore) *Autopilot {
	return &Autopilot{
		studio:    st,
		accounts:  accounts,
		store:     store,
		providers: providers,
		models:    models,
	}
}

// Result is what one autopilot run produced.
type Result struct {
	AccountID   int64
	AccountName string
	JobID       string
	Product     products.Product
}

// Run exits before product discovery until automatic Muse AI credit use is
// explicitly authorized for the account.
func (a *Autopilot) Run(ctx context.Context, accountID int64) (Result, error) {
	var r Result
	if a.studio == nil || a.accounts == nil || a.store == nil || a.models == nil {
		return r, fmt.Errorf("autopilot: thiếu thành phần (studio/accounts/store/models)")
	}
	acct, err := a.accounts.GetWithAutopilot(accountID)
	if err != nil {
		return r, fmt.Errorf("autopilot: %w", err)
	}
	r.AccountID = acct.ID
	r.AccountName = acct.Username
	if !network.AutopilotReady(acct) {
		return r, fmt.Errorf("autopilot: account %s chưa sẵn sàng (bật autopilot + chọn theme trong UI)",
			acct.Username)
	}
	return r, fmt.Errorf("autopilot: tạm dừng; chưa có xác nhận dùng Muse AI credits tự động theo account")
}

// RunAll runs every account that is autopilot-ready. One account's failure
// never stops the others; results carry per-account errors.
func (a *Autopilot) RunAll(ctx context.Context) []Result {
	accts, err := a.accounts.ListWithAutopilot()
	if err != nil {
		return nil
	}
	var out []Result
	for _, acct := range accts {
		if !network.AutopilotReady(acct) {
			continue
		}
		r, err := a.Run(ctx, acct.ID)
		if err != nil {
			r.AccountName = acct.Username
			// surface the error through the product title slot is ugly;
			// instead log it on the studio side via a failed no-op job note.
			r.Product = products.Product{Title: "LỖI: " + err.Error()}
		}
		out = append(out, r)
	}
	return out
}

// downloadImage fetches a product listing photo with sanity checks.
func (a *Autopilot) downloadImage(ctx context.Context, url, dst string) error {
	return DownloadImage(ctx, url, dst)
}

// downloadHTTP là client tải ảnh listing (timeout 60s như Autopilot).
var downloadHTTP = &http.Client{Timeout: 60 * time.Second}

// DownloadImage tải ảnh listing sản phẩm về file local (kiểm tra
// content-type + giới hạn 15MB). Dùng chung cho autopilot theo account
// và hunter Accesstrade (không viết lại logic tải ở automation).
func DownloadImage(ctx context.Context, url, dst string) error {
	return DownloadImageWithCookie(ctx, url, dst, "")
}

// DownloadImageWithCookie tải ảnh listing với session cookie đã được caller
// giới hạn theo domain đích. Cookie không được ghi log hoặc trả trong lỗi.
func DownloadImageWithCookie(ctx context.Context, url, dst, cookie string) error {
	if url == "" {
		return fmt.Errorf("empty url")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := downloadHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" &&
		!strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("not an image: %s", ct)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	// Read one byte beyond the cap so oversized responses fail instead of
	// being accepted as silently truncated image files.
	const maxImageBytes = int64(15 << 20)
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxImageBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	if written > maxImageBytes {
		_ = os.Remove(dst)
		return fmt.Errorf("image exceeds %d-byte limit", maxImageBytes)
	}
	return nil
}
