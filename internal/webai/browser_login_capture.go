package webai

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const browserStderrTailLimit = 16 * 1024

type browserStderrTail struct {
	mu   sync.Mutex
	data []byte
}

func (b *browserStderrTail) Write(p []byte) (int, error) {
	n := len(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if n >= browserStderrTailLimit {
		b.data = append(b.data[:0], p[n-browserStderrTailLimit:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - browserStderrTailLimit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (b *browserStderrTail) hasMacOSBootstrapFailure() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	stderr := strings.ToLower(string(b.data))
	return strings.Contains(stderr, "bootstrap_check_in") && strings.Contains(stderr, "141")
}

func (b *browserStderrTail) hasMacOSMissingExecutable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	stderr := strings.ToLower(string(b.data))
	return strings.Contains(stderr, "klsnoexecutableerr") || strings.Contains(stderr, "code=-10827")
}

func browserProcessExitedError(binary string, processErr error, stderr *browserStderrTail) error {
	if runtime.GOOS == "darwin" && stderr.hasMacOSBootstrapFailure() {
		return fmt.Errorf("trình duyệt %s bị macOS chặn tại Crashpad/Mach bootstrap (lỗi 141): hãy mở MiuNova từ Terminal/iTerm trong phiên desktop rồi thử lại", filepath.Base(binary))
	}
	if runtime.GOOS == "darwin" && stderr.hasMacOSMissingExecutable() {
		return fmt.Errorf("macOS LaunchServices từ chối khởi chạy %s (-10827: kLSNoExecutableErr); nếu MiuNova đang chạy trong Codex/IDE, hãy mở lại từ Terminal/iTerm thường trong desktop session rồi thử lại. Nếu lỗi còn, sửa hoặc cài lại trình duyệt", filepath.Base(binary))
	}
	if runtime.GOOS == "darwin" && processErr != nil && strings.Contains(strings.ToLower(processErr.Error()), "signal: aborted") {
		return fmt.Errorf("macOS dừng trình duyệt %s trước khi CDP sẵn sàng; nếu MiuNova chạy trong Codex/IDE, hãy khởi chạy MiuNova từ Terminal/iTerm thường rồi thử lại", filepath.Base(binary))
	}
	if runtime.GOOS == "darwin" && processErr != nil {
		return fmt.Errorf("macOS không khởi chạy được trình duyệt %s trước khi CDP sẵn sàng; nếu MiuNova đang chạy trong Codex/IDE, hãy mở lại từ Terminal/iTerm thường rồi thử lại. Chi tiết: %w", filepath.Base(binary), processErr)
	}
	if processErr == nil {
		if runtime.GOOS == "darwin" {
			return fmt.Errorf("trình duyệt %s đã thoát trước khi CDP sẵn sàng; nếu MiuNova đang chạy trong Codex/IDE, hãy mở app từ Terminal/iTerm thường rồi thử lại", filepath.Base(binary))
		}
		return fmt.Errorf("trình duyệt %s đã thoát trước khi CDP sẵn sàng; kiểm tra cài đặt hoặc đặt AICOS_BROWSER_BINARY tới Chrome/Chromium", filepath.Base(binary))
	}
	return fmt.Errorf("trình duyệt %s thoát trước khi CDP sẵn sàng: %w; kiểm tra cài đặt hoặc đặt AICOS_BROWSER_BINARY tới Chrome/Chromium", filepath.Base(binary), processErr)
}

type loginBrowserSession struct {
	port   int
	wsURL  string
	binary string
}

func loginBrowserCommand(ctx context.Context, binary string, args []string) (*exec.Cmd, error) {
	if runtime.GOOS == "darwin" {
		if isExplicitBrowserOverride(binary) || isPlaywrightCachedBrowser(binary) {
			return exec.CommandContext(ctx, binary, args...), nil
		}
		if app := macOSApplicationBundle(binary); app != "" {
			openArgs := []string{"-n", "-W", "-a", app, "--args"}
			openArgs = append(openArgs, args...)
			return exec.CommandContext(ctx, "/usr/bin/open", openArgs...), nil
		}
	}
	return exec.CommandContext(ctx, binary, args...), nil
}

func isExplicitBrowserOverride(binary string) bool {
	env := strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY"))
	if env == "" {
		return false
	}
	resolved, err := exec.LookPath(env)
	if err == nil {
		return filepath.Clean(resolved) == filepath.Clean(binary)
	}
	return filepath.Clean(env) == filepath.Clean(binary)
}

func isPlaywrightCachedBrowser(binary string) bool {
	path := filepath.ToSlash(filepath.Clean(binary))
	return strings.Contains(path, "/Library/Caches/ms-playwright/")
}

func loginBrowserCandidates() ([]string, error) {
	// Prefer Playwright's Chromium on macOS; installed browser apps can exit
	// before their CDP endpoint opens.
	if strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY")) == "" && runtime.GOOS == "darwin" {
		if cached := playwrightChromiumBinary(); cached != "" {
			candidates := []string{cached}
			if system, err := findChromiumBinary(); err == nil && filepath.Clean(system) != filepath.Clean(cached) {
				candidates = append(candidates, system)
			}
			return candidates, nil
		}
	}

	binary, err := findChromiumBinary()
	if err != nil {
		if strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY")) != "" {
			return nil, err
		}
		if cached := playwrightChromiumBinary(); cached != "" {
			return []string{cached}, nil
		}
		return nil, err
	}
	candidates := []string{binary}
	if strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY")) == "" {
		if cached := playwrightChromiumBinary(); cached != "" && filepath.Clean(cached) != filepath.Clean(binary) {
			candidates = append(candidates, cached)
		}
	}
	return candidates, nil
}

type loginBrowserLaunch struct {
	cmd         *exec.Cmd
	processDone chan error
	wsURL       string
}

func launchLoginBrowser(ctx context.Context, binary string, args []string, port int) (loginBrowserLaunch, bool, error) {
	command, err := loginBrowserCommand(ctx, binary, args)
	if err != nil {
		return loginBrowserLaunch{}, true, err
	}
	command.Stdout = io.Discard
	stderr := &browserStderrTail{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return loginBrowserLaunch{}, true, fmt.Errorf("không thể khởi chạy %s: %w", filepath.Base(binary), err)
	}
	processDone := make(chan error, 1)
	go func() {
		processDone <- command.Wait()
		close(processDone)
	}()

	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()
	probeTicker := time.NewTicker(350 * time.Millisecond)
	defer probeTicker.Stop()
	for {
		wsURL, probeErr := debuggerWSURL(port)
		if probeErr == nil && wsURL != "" {
			return loginBrowserLaunch{cmd: command, processDone: processDone, wsURL: wsURL}, false, nil
		}
		select {
		case processErr := <-processDone:
			return loginBrowserLaunch{}, true, browserProcessExitedError(binary, processErr, stderr)
		case <-ctx.Done():
			terminateBrowserAfterWait(command, processDone)
			return loginBrowserLaunch{}, false, ctx.Err()
		case <-startupTimer.C:
			if runtime.GOOS == "darwin" && stderr.hasMacOSBootstrapFailure() {
				terminateBrowserAfterWait(command, processDone)
				return loginBrowserLaunch{}, false, browserProcessExitedError(binary, nil, stderr)
			}
			terminateBrowserAfterWait(command, processDone)
			return loginBrowserLaunch{}, false, fmt.Errorf("trình duyệt %s không mở CDP sau 30 giây; kiểm tra cài đặt hoặc đặt AICOS_BROWSER_BINARY tới Chrome/Chromium", filepath.Base(binary))
		case <-probeTicker.C:
		}
	}
}

func macOSApplicationBundle(binary string) string {
	path := filepath.Clean(binary)
	for {
		if strings.HasSuffix(strings.ToLower(path), ".app") {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				return path
			}
			return ""
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

// RunCaptureViaCDPWithBrowserReady also reports the loopback CDP port once the
// browser is ready, allowing the authenticated dashboard to show and control it.
func RunCaptureViaCDPWithBrowserReady(ctx context.Context, target WebLoginTarget, timeout time.Duration, updateStatus func(status, msg string), onBrowserReady func(port int)) (*CapturedWebAuth, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	binaries, err := loginBrowserCandidates()
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", fmt.Sprintf("aicos-login-%s-*", target.Platform))
	if err != nil {
		return nil, fmt.Errorf("không thể tạo profile đăng nhập tạm thời: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	clearChromiumSingletonLocks(dir)
	port, portHold, err := pickFreePort()
	if err != nil {
		return nil, err
	}
	_ = portHold.Close()

	windowSize := "1280,900"
	if target.Platform == PlatformGoogleFlow {
		// Keep the Google Account login surface modest while preserving a crisp
		// screenshot in the 680px viewer.
		windowSize = "1024,768"
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + dir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--window-size=" + windowSize,
		"--force-device-scale-factor=1",
		"--new-window",
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		args = append(args, "--headless=new", "--disable-gpu")
	}
	args = append(args, target.StartURL)
	var bin string
	var launch loginBrowserLaunch
	var launchFailures []string
	for index, candidate := range binaries {
		if updateStatus != nil {
			message := fmt.Sprintf("Đang mở trình duyệt đăng nhập (%s)...", filepath.Base(candidate))
			if index > 0 {
				message = fmt.Sprintf("Trình duyệt trước không mở được; đang thử %s...", filepath.Base(candidate))
			}
			updateStatus("opening", message)
		}
		var retry bool
		launch, retry, err = launchLoginBrowser(ctx, candidate, args, port)
		if err == nil {
			bin = candidate
			break
		}
		launchFailures = append(launchFailures, err.Error())
		if !retry {
			return nil, err
		}
	}
	if launch.cmd == nil {
		return nil, fmt.Errorf("không khởi chạy được trình duyệt đăng nhập: %s", strings.Join(launchFailures, "; "))
	}
	defer terminateBrowserAfterWait(launch.cmd, launch.processDone)
	wsURL := launch.wsURL
	if onBrowserReady != nil {
		onBrowserReady(port)
	}
	defer closeLoginBrowser(wsURL)

	if updateStatus != nil {
		updateStatus("waiting_for_user", fmt.Sprintf("Trình duyệt %s đã sẵn sàng. Hoàn tất đăng nhập trong khung bên dưới.", target.Name))
	}
	browser := loginBrowserSession{port: port, wsURL: wsURL, binary: bin}
	if adapter, ok := webLoginAdapterFor(target.Platform); ok {
		return adapter.capture(ctx, browser, target, updateStatus)
	}
	return captureCookieLogin(ctx, browser, target, updateStatus)
}
