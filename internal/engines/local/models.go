package local

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Registry tracks downloaded assets and third-party repos under a data directory:
//
//	<data>/models               downloaded model assets
//	<data>/third_party/<name>   sidecar repos
type Registry struct {
	dataDir string
	http    *http.Client
}

// NewRegistry creates a registry rooted at dataDir.
func NewRegistry(dataDir string) *Registry {
	return &Registry{dataDir: dataDir, http: &http.Client{}}
}

// ModelsDir is <data>/models.
func (r *Registry) ModelsDir() string { return filepath.Join(r.dataDir, "models") }

// ThirdPartyDir is <data>/third_party.
func (r *Registry) ThirdPartyDir() string { return filepath.Join(r.dataDir, "third_party") }

// ModelPath returns the local path of a model file.
func (r *Registry) ModelPath(name string) string { return filepath.Join(r.ModelsDir(), name) }

// Has reports whether a non-empty model file exists.
func (r *Registry) Has(name string) bool {
	st, err := os.Stat(r.ModelPath(name))
	return err == nil && st.Size() > 0
}

// Download fetches url into <data>/models/name with resume support.
// A partial ".part" file is resumed via Range when the server allows it.
// onProgress may be nil. Cancelling ctx aborts with an error; the partial
// file is kept so a later call can resume.
func (r *Registry) Download(ctx context.Context, name, url string, minSize int64, onProgress func(downloaded, total int64)) error {
	if err := os.MkdirAll(r.ModelsDir(), 0o755); err != nil {
		return err
	}
	dest := r.ModelPath(name)
	if minSize > 0 {
		if st, err := os.Stat(dest); err == nil && st.Size() >= minSize {
			return nil // already have it
		}
	} else if _, err := os.Stat(dest); err == nil {
		return nil
	}
	tmp := dest + ".part"
	if _, err := r.fetch(ctx, url, tmp, onProgress); err != nil {
		return err
	}
	if minSize > 0 {
		if st, err := os.Stat(tmp); err != nil || st.Size() < minSize {
			size := int64(-1)
			if err == nil {
				size = st.Size()
			}
			return fmt.Errorf("download %s: got %d bytes, need at least %d", name, size, minSize)
		}
	}
	return os.Rename(tmp, dest)
}

// fetch downloads url to tmpPath, resuming a partial file when possible.
// Returns bytes written.
func (r *Registry) fetch(ctx context.Context, url, tmpPath string, onProgress func(downloaded, total int64)) (int64, error) {
	var start int64
	if st, err := os.Stat(tmpPath); err == nil {
		start = st.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if start > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(start, 10)+"-")
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if start > 0 && resp.StatusCode != http.StatusPartialContent {
		// Server ignored Range; restart from scratch.
		start = 0
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	total := resp.ContentLength
	if total >= 0 {
		total += start
	}
	flag := os.O_CREATE | os.O_WRONLY
	if start > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(tmpPath, flag, 0o644)
	if err != nil {
		return 0, err
	}
	downloaded := start
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return downloaded, werr
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if rerr != nil {
			f.Close()
			if rerr == io.EOF {
				return downloaded, nil
			}
			return downloaded, fmt.Errorf("download %s: %w", url, rerr)
		}
	}
}
