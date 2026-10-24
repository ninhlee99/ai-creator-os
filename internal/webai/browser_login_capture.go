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
		return fmt.Errorf("macOS không mở được ứng dụng %s (-10827: kLSNoExecutableErr); cài lại trình duyệt hoặc đặt AICOS_BROWSER_BINARY tới Chrome/Chromium còn nguyên vẹn", filepath.Base(binary))
	}
	if processErr == nil {
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
		if app := macOSApplicationBundle(binary); app != "" {
			openArgs := []string{"-n", "-W", "-a", app, "--args"}
			openArgs = append(openArgs, args...)
			return exec.CommandContext(ctx, "/usr/bin/open", openArgs...), nil
		}
	}
	return exec.CommandContext(ctx, binary, args...), nil
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

	bin, err := findChromiumBinary()
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

	if updateStatus != nil {
		updateStatus("opening", fmt.Sprintf("Đang mở trình duyệt đăng nhập (%s)...", filepath.Base(bin)))
	}

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
	command, launchErr := loginBrowserCommand(ctx, bin, args)
	if launchErr != nil {
		return nil, launchErr
	}
	cmd := command
	cmd.Stdout = io.Discard
	stderr := &browserStderrTail{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không thể khởi chạy trình duyệt: %w", err)
	}
	processDone := make(chan error, 1)
	go func() {
		processDone <- cmd.Wait()
		close(processDone)
	}()
	defer terminateBrowserAfterWait(cmd, processDone)

	var wsURL string
	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()
	probeTicker := time.NewTicker(350 * time.Millisecond)
	defer probeTicker.Stop()
	for {
		wsURL, err = debuggerWSURL(port)
		if err == nil && wsURL != "" {
			break
		}
		select {
		case processErr := <-processDone:
			return nil, browserProcessExitedError(bin, processErr, stderr)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-startupTimer.C:
			if runtime.GOOS == "darwin" && stderr.hasMacOSBootstrapFailure() {
				return nil, browserProcessExitedError(bin, nil, stderr)
			}
			return nil, fmt.Errorf("trình duyệt %s không mở CDP sau 30 giây; kiểm tra cài đặt hoặc đặt AICOS_BROWSER_BINARY tới Chrome/Chromium", filepath.Base(bin))
		case <-probeTicker.C:
		}
	}
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
