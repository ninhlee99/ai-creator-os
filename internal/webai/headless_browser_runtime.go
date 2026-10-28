package webai

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var profileLocks sync.Map

func clearChromiumSingletonLocks(dir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie", "DevToolsActivePort"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// cleanChromiumVolatileCaches strips heavy transient shader/code caches on
// profile release while keeping cookies, localStorage, and IndexedDB intact.
func cleanChromiumVolatileCaches(dir string) {
	clearChromiumSingletonLocks(dir)
	for _, rel := range []string{
		"Crashpad",
		"BrowserMetrics",
		"GrShaderCache",
		"ShaderCache",
		"GraphiteDawnCache",
		filepath.Join("Default", "Cache"),
		filepath.Join("Default", "Code Cache"),
		filepath.Join("Default", "GPUCache"),
		filepath.Join("Default", "Service Worker", "CacheStorage"),
	} {
		_ = os.RemoveAll(filepath.Join(dir, rel))
	}
}

// acquireAccountBrowserProfile returns a persistent per-account profile
// directory (keeping localStorage, IndexedDB, and anti-bot tokens across runs)
// when no other goroutine is currently using that profile, or falls back to a
// clean temporary directory when concurrent runs share the same account.
func acquireAccountBrowserProfile(platform, cookieHeader string, cookies []BrowserCookie) (string, func(), error) {
	if os.Getenv("AICOS_EPHEMERAL_BROWSER_PROFILE") != "1" {
		fingerprint := strings.TrimSpace(cookieHeader)
		if fingerprint == "" && len(cookies) > 0 {
			var parts []string
			for _, c := range cookies {
				if c.Name != "" && c.Value != "" {
					parts = append(parts, c.Domain+":"+c.Name+"="+c.Value)
				}
			}
			sort.Strings(parts)
			fingerprint = strings.Join(parts, ";")
		}
		if fingerprint != "" {
			sum := sha256.Sum256([]byte(platform + "|" + fingerprint))
			key := platform + "-" + hex.EncodeToString(sum[:8])
			if _, loaded := profileLocks.LoadOrStore(key, struct{}{}); !loaded {
				baseDir := filepath.Join(os.TempDir(), "aicos-browser-profiles", key)
				if err := os.MkdirAll(baseDir, 0o700); err == nil {
					clearChromiumSingletonLocks(baseDir)
					return baseDir, func() {
						cleanChromiumVolatileCaches(baseDir)
						profileLocks.Delete(key)
					}, nil
				}
				profileLocks.Delete(key)
			}
		}
	}
	profile, err := os.MkdirTemp("", "aicos-"+platform+"-headless-*")
	if err != nil {
		return "", func() {}, err
	}
	clearChromiumSingletonLocks(profile)
	return profile, func() { _ = os.RemoveAll(profile) }, nil
}

// optimizedHeadlessBrowserArgs builds low-RAM, low-CPU, low-SSD-wear flags for
// headless Chromium automation.
func optimizedHeadlessBrowserArgs(port int, profileDir, windowSize string) []string {
	if windowSize == "" {
		windowSize = "1440,1100"
	}
	return []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-software-rasterizer",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-extensions",
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-ipc-flooding-protection",
		"--disable-features=Translate,OptimizationHints,MediaRouter,DialMediaRouteProvider,CalculateNativeWinOcclusion,AudioServiceOutOfProcess",
		"--blink-settings=primaryHoverType=2,availableHoverTypes=2,primaryPointerType=4,availablePointerTypes=4",
		"--disk-cache-size=10485760",
		"--media-cache-size=10485760",
		"--js-flags=--max-old-space-size=512",
		"--hide-scrollbars",
		"--mute-audio",
		"--window-size=" + windowSize,
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profileDir,
		"about:blank",
	}
}

// ConfigureHeadlessBrowserCommand exports process-group isolation for callers
// outside package webai (such as Studio's TikTok crawler).
func ConfigureHeadlessBrowserCommand(cmd *exec.Cmd) {
	configureHeadlessBrowserCommand(cmd)
}

// TerminateBrowser exports graceful process-group termination for callers
// outside package webai.
func TerminateBrowser(cmd *exec.Cmd) {
	terminateBrowser(cmd)
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
		terminateBrowser(cmd)
		<-processDone
	}
}

// FindChromiumBinary exports the binary resolution for headless automation.
func FindChromiumBinary() (string, error) {
	return findChromiumBinary()
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
