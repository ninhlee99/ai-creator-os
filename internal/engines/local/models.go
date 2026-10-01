package local

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ModelInfo describes one downloadable model asset.
type ModelInfo struct {
	Name    string // file name under <data>/models
	URL     string // download URL (HuggingFace resolve link)
	MinSize int64  // minimum acceptable bytes; 0 = skip verification
}

// DefaultModels is the built-in catalog. Sizes are conservative minimums.
var DefaultModels = []ModelInfo{
	{
		Name:    "qwen2.5-7b-instruct-q4_k_m.gguf",
		URL:     "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct-GGUF/resolve/main/qwen2.5-7b-instruct-q4_k_m.gguf",
		MinSize: 4_400_000_000, // ~4.7 GB actual
	},
}

// VieNeuRepoURL is the third-party TTS sidecar repo (Apache-2.0).
const VieNeuRepoURL = "https://github.com/pnnbao97/VieNeu-TTS.git"

// VieNeuZipURL is the fallback when git is unavailable.
const VieNeuZipURL = "https://github.com/pnnbao97/VieNeu-TTS/archive/refs/heads/main.zip"

// Registry tracks model files and third-party repos under a data directory:
//
//	<data>/models               GGUF and other model files
//	<data>/third_party/<name>   sidecar repos (vieneu-tts)
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

// VieNeuDir is <data>/third_party/vieneu-tts.
func (r *Registry) VieNeuDir() string { return filepath.Join(r.ThirdPartyDir(), "vieneu-tts") }

// ModelPath returns the local path of a model file.
func (r *Registry) ModelPath(name string) string { return filepath.Join(r.ModelsDir(), name) }

// Has reports whether a non-empty model file exists.
func (r *Registry) Has(name string) bool {
	st, err := os.Stat(r.ModelPath(name))
	return err == nil && st.Size() > 0
}

// VieNeuPresent reports whether the sidecar repo looks usable.
func (r *Registry) VieNeuPresent() bool {
	_, err := os.Stat(filepath.Join(r.VieNeuDir(), "apps", "openai_speech.py"))
	return err == nil
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

// EnsureVieNeuRepo makes sure <data>/third_party/vieneu-tts exists and looks
// usable: git clone --depth 1 when git is available, otherwise a zip download
// from GitHub. onProgress may be nil (only used for the zip path).
func (r *Registry) EnsureVieNeuRepo(ctx context.Context, onProgress func(downloaded, total int64)) error {
	if r.VieNeuPresent() {
		return nil
	}
	if err := os.MkdirAll(r.ThirdPartyDir(), 0o755); err != nil {
		return err
	}
	if _, err := exec.LookPath("git"); err == nil {
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", VieNeuRepoURL, r.VieNeuDir())
		var out strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone VieNeu-TTS: %w: %s", err, truncate(out.String(), 300))
		}
		if !r.VieNeuPresent() {
			return fmt.Errorf("git clone succeeded but apps/openai_speech.py is missing")
		}
		return nil
	}
	// No git: download the repo zip and extract it.
	tmp, err := os.CreateTemp("", "vieneu-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	if _, err := r.fetch(ctx, VieNeuZipURL, tmpPath, onProgress); err != nil {
		return fmt.Errorf("download VieNeu-TTS zip: %w", err)
	}
	if err := unzipStripTop(tmpPath, r.VieNeuDir()); err != nil {
		return fmt.Errorf("extract VieNeu-TTS zip: %w", err)
	}
	if !r.VieNeuPresent() {
		return fmt.Errorf("zip extracted but apps/openai_speech.py is missing")
	}
	return nil
}

// unzipStripTop extracts a zip, dropping the first path component
// (GitHub zips nest everything under <repo>-<branch>/).
func unzipStripTop(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	cleanDest := filepath.Clean(destDir)
	for _, f := range zr.File {
		rel := f.Name
		if i := strings.Index(rel, "/"); i >= 0 {
			rel = rel[i+1:]
		} else {
			continue // top-level file without directory; skip
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(target), cleanDest+string(os.PathSeparator)) &&
			filepath.Clean(target) != cleanDest {
			return fmt.Errorf("zip slip: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
