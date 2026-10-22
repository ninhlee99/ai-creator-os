package webai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func DownloadFlowVideo(ctx context.Context, mediaURL, outPath string) error {
	if !isGoogleMediaURL(mediaURL) {
		return errors.New("Google Flow media URL không được cho phép")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return errors.New("invalid Google Flow media URL")
	}
	req.Header.Set("User-Agent", museUserAgent)
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Google Flow media download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Google Flow media HTTP %d", resp.StatusCode)
	}
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".aicos-flow-*.mp4")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	written, err := io.Copy(f, io.LimitReader(resp.Body, 250<<20+1))
	if err != nil {
		_ = f.Close()
		return err
	}
	if written < 12 || written > 250<<20 {
		_ = f.Close()
		return errors.New("Google Flow video size is invalid or exceeds 250 MB")
	}
	if _, err := f.Seek(4, io.SeekStart); err != nil {
		_ = f.Close()
		return err
	}
	var signature [4]byte
	if _, err := io.ReadFull(f, signature[:]); err != nil || string(signature[:]) != "ftyp" {
		_ = f.Close()
		return errors.New("Google Flow media is not an MP4 file")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return err
	}
	return nil
}
