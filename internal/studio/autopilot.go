package studio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// Autopilot is the hands-off affiliate loop Ninh asked for (2026-10-01):
// theme (per account) -> find high-commission product -> real listing photos
// as product-lock ground truth -> model photo (uploaded by Ninh) as
// identity lock -> studio affiliate job -> QC. Ninh only uploads model
// photos, picks the theme, and watches.
type Autopilot struct {
	studio    *Studio
	accounts  *network.AccountManager
	store     *products.Store
	providers []products.Provider
	models    *ModelStore
	http      *http.Client
	// MusicPath is the trending-audio file to mix under autopilot videos.
	// "" = silent video (the job log says so explicitly).
	MusicPath string
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
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

// Result is what one autopilot run produced.
type Result struct {
	AccountID   int64
	AccountName string
	JobID       string
	Product     products.Product
}

// Run executes one hands-off cycle for an account.
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
	ok, err := a.models.HasPhotos(accountID)
	if err != nil {
		return r, err
	}
	if !ok {
		return r, fmt.Errorf("autopilot: account %s chưa có ảnh mẫu — Ninh upload ảnh mẫu trong UI rồi chạy lại",
			acct.Username)
	}

	// 1. Product: cached winners first, else live discovery.
	cands, err := a.store.TopByTheme(acct.Theme, acct.MinCommission, accountID, 5)
	if err != nil {
		return r, err
	}
	if len(cands) == 0 {
		q := products.QueryForTheme(acct.Theme, acct.MinCommission, 20)
		found, errs := products.Aggregate(ctx, a.providers, q)
		for _, p := range found {
			if id, serr := a.store.Save(p); serr == nil {
				p.ID = id
			}
		}
		cands, _ = a.store.TopByTheme(acct.Theme, acct.MinCommission, accountID, 5)
		if len(cands) == 0 {
			msg := "autopilot: không tìm được sản phẩm đủ hoa hồng cho theme " + acct.Theme
			if len(errs) > 0 {
				msg += fmt.Sprintf(" (nguồn lỗi: %v)", errs[0])
			}
			return r, fmt.Errorf("%s", msg)
		}
	}
	prod := cands[0]
	r.Product = prod

	// 2. Product photos: download real listing images (ground truth).
	work := filepath.Join(a.studio.workRoot, "autopilot", fmt.Sprint(accountID))
	if err := os.MkdirAll(work, 0o755); err != nil {
		return r, err
	}
	var prodPhotos []string
	for i, u := range prod.ImageURLs {
		if i >= 3 {
			break
		}
		dst := filepath.Join(work, fmt.Sprintf("prod-%d.jpg", i))
		if derr := a.downloadImage(ctx, u, dst); derr != nil {
			continue
		}
		prodPhotos = append(prodPhotos, dst)
	}
	if len(prodPhotos) == 0 {
		return r, fmt.Errorf("autopilot: không tải được ảnh sản phẩm %q", prod.Title)
	}

	// 3. Model photo: Ninh's upload = identity lock.
	mps, err := a.models.List(accountID)
	if err != nil || len(mps) == 0 {
		return r, fmt.Errorf("autopilot: mất ảnh mẫu của account %s", acct.Username)
	}

	// 4. Studio job (photo-list mode — Ninh's chosen fashion format).
	themeLabel := acct.Theme
	if t, ok := products.DescribeTheme(acct.Theme); ok {
		themeLabel = t.Label
	}
	jobID, err := a.studio.CreateAffiliateJob(AffiliateParams{
		Mode:         AffiliateModePhoto,
		Niche:        themeLabel,
		ProductName:  prod.Title,
		ModelPhoto:   mps[0].Path,
		ProductPhoto: prodPhotos[0],
		Seconds:      30,
		MusicPath:    a.MusicPath,
		MusicTitle:   "trending",
	})
	if err != nil {
		return r, err
	}
	r.JobID = jobID

	// 5. Don't promote the same product twice for this account.
	if prod.ID != 0 {
		_ = a.store.RecordUse(prod.ID, accountID, jobID)
	}
	return r, nil
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
	if url == "" {
		return fmt.Errorf("empty url")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	resp, err := a.http.Do(req)
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
	defer f.Close()
	// 15 MB cap — listing photos are small; this stops runaway downloads.
	if _, err := io.CopyN(f, resp.Body, 15<<20); err != nil && err != io.EOF {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
