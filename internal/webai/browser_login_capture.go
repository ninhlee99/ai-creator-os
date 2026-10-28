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
	// Invoke the browser executable directly. On macOS, LaunchServices can
	// forward flags to an existing interactive Edge instance and return early,
	// leaving automation attached to or terminating the user's full browser.
	return exec.CommandContext(ctx, binary, args...), nil
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

func launchLoginBrowser(ctx context.Context, binary string, args []string, profile string) (loginBrowserLaunch, bool, error) {
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
		wsURL, probeErr := debuggerWSURLFromProfile(profile)
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
	clearChromiumDevToolsPort(dir)
	windowSize := "1280,900"
	args := []string{
		"--remote-debugging-port=0",
		"--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + dir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--window-size=" + windowSize,
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
		clearChromiumDevToolsPort(dir)
		launch, retry, err = launchLoginBrowser(ctx, candidate, args, dir)
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
	port := debuggerPortFromProfile(dir)
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
