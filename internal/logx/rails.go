package logx

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level represents Rails severity log level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "INFO"
	}
}

// Letter returns the single letter severity code (Rails convention: D, I, W, E, F).
func (l Level) Letter() string {
	switch l {
	case LevelDebug:
		return "D"
	case LevelInfo:
		return "I"
	case LevelWarn:
		return "W"
	case LevelError:
		return "E"
	case LevelFatal:
		return "F"
	default:
		return "I"
	}
}

// ParseLevel parses a log level string (e.g. "debug", "info", "warn", "error").
func ParseLevel(lvl string) Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelDebug
	}
}

// ANSI color escape codes for Rails debug console styling.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// RailsLogger formats logs in Ruby on Rails debug logger format:
// [2026-10-26T18:15:30.123456 #12345]  INFO -- : message
type RailsLogger struct {
	mu      sync.Mutex
	level   Level
	console io.Writer
	file    io.Writer
	pid     int
}

var (
	globalMu     sync.RWMutex
	globalLogger *RailsLogger
)

func init() {
	lvl := LevelDebug
	if envLvl := os.Getenv("RAILS_LOG_LEVEL"); envLvl != "" {
		lvl = ParseLevel(envLvl)
	} else if envLvl := os.Getenv("LOG_LEVEL"); envLvl != "" {
		lvl = ParseLevel(envLvl)
	}
	globalLogger = NewRailsLogger(os.Stderr, nil, lvl)
}

// NewRailsLogger creates a new Rails-styled logger.
func NewRailsLogger(console io.Writer, file io.Writer, lvl Level) *RailsLogger {
	return &RailsLogger{
		level:   lvl,
		console: console,
		file:    file,
		pid:     os.Getpid(),
	}
}

// SetupRailsLogging initializes the global Rails logger and bridges Go's standard log package.
func SetupRailsLogging(console io.Writer, file io.Writer, lvl Level) *RailsLogger {
	globalMu.Lock()
	defer globalMu.Unlock()

	globalLogger = NewRailsLogger(console, file, lvl)

	// Route standard Go log package through Rails formatter
	log.SetFlags(0)
	log.SetOutput(&stdLogWriter{logger: globalLogger})

	return globalLogger
}

func (l *RailsLogger) SetLevel(lvl Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = lvl
}

func (l *RailsLogger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// formatRailsLine formats a log line in Rails format:
// Colorized for console:
// I, [2026-10-26T18:15:30.123456 #12345]  INFO -- : message
func (l *RailsLogger) formatLine(now time.Time, lvl Level, tag string, msg string, color bool) string {
	timestamp := now.Format("2006-01-02T15:04:05.000000")
	letter := lvl.Letter()
	name := fmt.Sprintf("%-5s", lvl.String())

	var tagPart string
	if tag != "" {
		tagPart = fmt.Sprintf("[%s] ", tag)
	}

	if !color {
		return fmt.Sprintf("%s, [%s #%d] %s -- : %s%s\n", letter, timestamp, l.pid, name, tagPart, msg)
	}

	var colorCode string
	switch lvl {
	case LevelDebug:
		colorCode = ansiCyan
	case LevelInfo:
		colorCode = ansiGreen
	case LevelWarn:
		colorCode = ansiYellow
	case LevelError:
		colorCode = ansiRed
	case LevelFatal:
		colorCode = ansiBold + ansiRed
	}

	// Rails color styling: dimmed timestamp & PID, colored severity badge
	return fmt.Sprintf("%s, %s[%s #%d]%s %s%s%s -- : %s%s\n",
		letter,
		ansiDim, timestamp, l.pid, ansiReset,
		colorCode, name, ansiReset,
		tagPart, msg)
}

func (l *RailsLogger) log(lvl Level, tag string, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if lvl < l.level {
		return
	}

	now := time.Now()
	if l.console != nil {
		colored := l.formatLine(now, lvl, tag, msg, true)
		_, _ = l.console.Write([]byte(colored))
	}
	if l.file != nil {
		plain := l.formatLine(now, lvl, tag, msg, false)
		_, _ = l.file.Write([]byte(plain))
	}
}

func (l *RailsLogger) Debug(args ...any) {
	l.log(LevelDebug, "", fmt.Sprint(args...))
}

func (l *RailsLogger) Debugf(format string, args ...any) {
	l.log(LevelDebug, "", fmt.Sprintf(format, args...))
}

func (l *RailsLogger) Info(args ...any) {
	l.log(LevelInfo, "", fmt.Sprint(args...))
}

func (l *RailsLogger) Infof(format string, args ...any) {
	l.log(LevelInfo, "", fmt.Sprintf(format, args...))
}

func (l *RailsLogger) Warn(args ...any) {
	l.log(LevelWarn, "", fmt.Sprint(args...))
}

func (l *RailsLogger) Warnf(format string, args ...any) {
	l.log(LevelWarn, "", fmt.Sprintf(format, args...))
}

func (l *RailsLogger) Error(args ...any) {
	l.log(LevelError, "", fmt.Sprint(args...))
}

func (l *RailsLogger) Errorf(format string, args ...any) {
	l.log(LevelError, "", fmt.Sprintf(format, args...))
}

func (l *RailsLogger) Fatal(args ...any) {
	l.log(LevelFatal, "", fmt.Sprint(args...))
	os.Exit(1)
}

func (l *RailsLogger) Fatalf(format string, args ...any) {
	l.log(LevelFatal, "", fmt.Sprintf(format, args...))
	os.Exit(1)
}

// LogRequest logs an HTTP request in classic Rails style:
// Started GET "/studio" for 127.0.0.1 at 2026-10-26 18:15:30 +0700
// Processing by WebServer#GET /studio as HTML
//   Parameters: {"topic"=>"dance"}
// Completed 200 OK in 14.5ms
func (l *RailsLogger) LogRequest(r *http.Request, status int, duration time.Duration, location string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Assets and health checks quieted down if not debug level
	isAsset := strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz"
	if isAsset && l.level > LevelDebug {
		return
	}

	now := time.Now()
	startTime := now.Add(-duration)
	clientIP := clientIPFromRequest(r)

	// Format Started line
	timeStr := startTime.Format("2006-01-02 15:04:05 -0700")
	startedPlain := fmt.Sprintf("Started %s \"%s\" for %s at %s\n", r.Method, r.URL.RequestURI(), clientIP, timeStr)
	startedColored := fmt.Sprintf("%sStarted %s \"%s\" for %s at %s%s\n", ansiBold, r.Method, r.URL.RequestURI(), clientIP, timeStr, ansiReset)

	// Processing by line
	contentType := "HTML"
	if accept := r.Header.Get("Accept"); strings.Contains(accept, "application/json") {
		contentType = "JSON"
	}
	processingPlain := fmt.Sprintf("Processing by WebServer#%s %s as %s\n", r.Method, r.URL.Path, contentType)

	// Parameters line (if any query parameters)
	var paramsPlain string
	if len(r.URL.Query()) > 0 {
		var pairs []string
		for k, v := range r.URL.Query() {
			val := strings.Join(v, ", ")
			pairs = append(pairs, fmt.Sprintf("%q=>%q", k, val))
		}
		paramsPlain = fmt.Sprintf("  Parameters: {%s}\n", strings.Join(pairs, ", "))
	}

	// Completed line with status code and duration
	statusText := http.StatusText(status)
	durationMs := float64(duration.Microseconds()) / 1000.0

	var redirectPlain, redirectColored string
	if location != "" {
		redirectPlain = fmt.Sprintf("Redirected to %s\n", location)
		redirectColored = fmt.Sprintf("%sRedirected to %s%s\n", ansiCyan, location, ansiReset)
	}

	completedPlain := fmt.Sprintf("Completed %d %s in %.1fms\n", status, statusText, durationMs)

	// Rails status color
	var statusColor string
	switch {
	case status >= 200 && status < 300:
		statusColor = ansiGreen
	case status >= 300 && status < 400:
		statusColor = ansiCyan
	case status >= 400 && status < 500:
		statusColor = ansiYellow
	default:
		statusColor = ansiRed
	}

	completedColored := fmt.Sprintf("Completed %s%d %s%s in %s%.1fms%s\n",
		statusColor, status, statusText, ansiReset,
		ansiBold, durationMs, ansiReset)

	// Write to console
	if l.console != nil {
		buf := startedColored + processingPlain
		if paramsPlain != "" {
			buf += paramsPlain
		}
		if redirectColored != "" {
			buf += redirectColored
		}
		buf += completedColored
		_, _ = l.console.Write([]byte(buf))
	}

	// Write to file (clean plain text)
	if l.file != nil {
		buf := startedPlain + processingPlain
		if paramsPlain != "" {
			buf += paramsPlain
		}
		if redirectPlain != "" {
			buf += redirectPlain
		}
		buf += completedPlain
		_, _ = l.file.Write([]byte(buf))
	}
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if host := r.RemoteAddr; host != "" {
		if u, err := url.Parse("//" + host); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
		return host
	}
	return "127.0.0.1"
}

// stdLogWriter captures Go's standard library log.Print/Printf calls and
// converts them into Rails debug log entries.
type stdLogWriter struct {
	logger *RailsLogger
}

var goStdDateRegex = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? `)

func (w *stdLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	// Strip standard Go logger timestamp if it was injected
	msg = goStdDateRegex.ReplaceAllString(msg, "")

	// Infer severity level
	lower := strings.ToLower(msg)
	lvl := LevelInfo
	if strings.Contains(lower, "[debug]") || strings.HasPrefix(lower, "debug:") {
		lvl = LevelDebug
	} else if strings.Contains(lower, "[warn]") || strings.Contains(lower, "warning:") || strings.Contains(lower, "cảnh báo") {
		lvl = LevelWarn
	} else if strings.Contains(lower, "[error]") || strings.Contains(lower, "lỗi:") || strings.Contains(lower, "failed:") || strings.Contains(lower, "panic:") || strings.Contains(lower, "hỏng:") {
		lvl = LevelError
	}

	// Parse optional tag like [gmodels] or studio:
	var tag string
	if strings.HasPrefix(msg, "[") {
		if end := strings.Index(msg, "]"); end > 0 {
			tag = msg[1:end]
			msg = strings.TrimSpace(msg[end+1:])
		}
	}

	w.logger.log(lvl, tag, msg)
	return len(p), nil
}

// Global helper functions
func Debug(args ...any)                 { globalLogger.Debug(args...) }
func Debugf(format string, args ...any) { globalLogger.Debugf(format, args...) }
func Info(args ...any)                  { globalLogger.Info(args...) }
func Infof(format string, args ...any)  { globalLogger.Infof(format, args...) }
func Warn(args ...any)                  { globalLogger.Warn(args...) }
func Warnf(format string, args ...any)  { globalLogger.Warnf(format, args...) }
func Error(args ...any)                 { globalLogger.Error(args...) }
func Errorf(format string, args ...any) { globalLogger.Errorf(format, args...) }
func Fatal(args ...any)                 { globalLogger.Fatal(args...) }
func Fatalf(format string, args ...any) { globalLogger.Fatalf(format, args...) }
func SetLevel(lvl Level)                { globalLogger.SetLevel(lvl) }
func GetLevel() Level                   { return globalLogger.GetLevel() }
func LogRequest(r *http.Request, status int, duration time.Duration, location string) {
	globalLogger.LogRequest(r, status, duration, location)
}
