package webai

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func clearChromiumSingletonLocks(dir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
func terminateBrowser(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func terminateBrowserAfterWait(cmd *exec.Cmd, processDone <-chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	select {
	case <-processDone:
		return
	default:
	}
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-processDone:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-processDone
	}
}

func findChromiumBinary() (string, error) {
	if env := strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY")); env != "" {
		if binary, err := exec.LookPath(env); err == nil {
			return binary, nil
		}
		return "", fmt.Errorf("AICOS_BROWSER_BINARY không trỏ tới trình duyệt có thể chạy: %q", env)
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		candidates = append(candidates,
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Volumes/Macintosh HD/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, "Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
				filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
				filepath.Join(home, "Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
				filepath.Join(home, "Applications/Chromium.app/Contents/MacOS/Chromium"),
			)
		}
	case "windows":
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if root == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(root, "Microsoft/Edge/Application/msedge.exe"),
				filepath.Join(root, "Google/Chrome/Application/chrome.exe"),
				filepath.Join(root, "BraveSoftware/Brave-Browser/Application/brave.exe"),
			)
		}
	default:
		candidates = append(candidates,
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/usr/bin/brave-browser",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		)
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	for _, name := range []string{
		"msedge", "microsoft-edge", "microsoft-edge-stable",
		"google-chrome", "google-chrome-stable", "chrome",
		"brave", "brave-browser", "chromium", "chromium-browser",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("không tìm thấy Chrome, Edge, Brave hoặc Chromium cho phiên Web AI; có thể đặt AICOS_BROWSER_BINARY")
}

func playwrightChromiumBinary() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	root := filepath.Join(home, "Library", "Caches", "ms-playwright")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		left := chromiumCacheVersion(entries[i].Name())
		right := chromiumCacheVersion(entries[j].Name())
		if left == right {
			return entries[i].Name() > entries[j].Name()
		}
		return left > right
	})
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "chromium-") {
			continue
		}
		binary := filepath.Join(root, entry.Name(), "chrome-mac-"+arch,
			"Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
		if info, err := os.Stat(binary); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return binary
		}
	}
	return ""
}

func chromiumCacheVersion(name string) int {
	version, _ := strconv.Atoi(strings.TrimPrefix(name, "chromium-"))
	return version
}
