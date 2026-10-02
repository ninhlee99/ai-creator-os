//go:build parked

// Package affiliatehunter điều khiển điện thoại Android qua ADB để tự quét
// Product Marketplace trong app TikTok (Affiliate Center) mà không cần
// TikTok Shop API: không app review, không business/company setup.
//
// Luồng dự kiến: adb kiểm tra máy → mở TikTok → vào marketplace →
// uiautomator dump cây UI → trích tên/giá/% hoa hồng → lưu vào kho sản phẩm
// cho autopilot dùng. Fail-closed: thiếu adb, mất kết nối máy, hay TikTok
// đổi giao diện đều dừng lại và báo rõ trong UI chứ không đoán bừa.
package affiliatehunter

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// TikTokPackages thử lần lượt khi mở app (VN dùng bản global).
var TikTokPackages = []string{
	"com.zhiliaoapp.musically",
	"com.ss.android.ugc.trill",
}

// LookADB tìm binary adb, báo rõ cách cài nếu thiếu.
func LookADB() (string, error) {
	p, err := exec.LookPath("adb")
	if err != nil {
		return "", fmt.Errorf("chưa cài adb (Android platform-tools): chạy `brew install android-platform-tools` trên Mac rồi thử lại")
	}
	return p, nil
}

// runADB chạy `adb [-s serial] ...`, trả stdout đã trim.
func runADB(serial string, args ...string) (string, error) {
	adb, err := LookADB()
	if err != nil {
		return "", err
	}
	full := []string{}
	if serial != "" {
		full = append(full, "-s", serial)
	}
	full = append(full, args...)
	cmd := exec.Command(adb, full...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("adb %s: %v: %s", strings.Join(full, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Devices liệt kê serial các máy đang ở trạng thái "device".
func Devices() ([]string, error) {
	out, err := runADB("", "devices", "-l")
	if err != nil {
		return nil, err
	}
	var devs []string
	for _, line := range strings.Split(out, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "device" {
			devs = append(devs, f[0])
		}
	}
	if len(devs) == 0 {
		return nil, fmt.Errorf("không thấy điện thoại nào: cắm cáp USB, bật USB debugging và bấm \"Cho phép\" trên điện thoại")
	}
	return devs, nil
}

// FirstDevice trả serial máy đầu tiên (thường chỉ có 1 máy của Ninh).
func FirstDevice() (string, error) {
	devs, err := Devices()
	if err != nil {
		return "", err
	}
	return devs[0], nil
}

// Shell chạy lệnh shell trên điện thoại.
func Shell(serial string, args ...string) (string, error) {
	return runADB(serial, append([]string{"shell"}, args...)...)
}

// Tap bấm vào tọa độ màn hình.
func Tap(serial string, x, y int) error {
	_, err := Shell(serial, "input", "tap", itoa(x), itoa(y))
	return err
}

// Swipe vuốt từ (x1,y1) tới (x2,y2) trong ms mili-giây.
func Swipe(serial string, x1, y1, x2, y2, ms int) error {
	_, err := Shell(serial, "input", "swipe", itoa(x1), itoa(y1), itoa(x2), itoa(y2), itoa(ms))
	return err
}

// KeyEvent gửi phím Android (4 = back, 3 = home, 26 = nguồn).
func KeyEvent(serial string, code int) error {
	_, err := Shell(serial, "input", "keyevent", itoa(code))
	return err
}

// StartApp mở app theo package, trả package mở thành công.
func StartApp(serial, pkg string) error {
	_, err := Shell(serial, "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	return err
}

// StartTikTok mở app TikTok (thử các package đã biết).
func StartTikTok(serial string) error {
	var last error
	for _, pkg := range TikTokPackages {
		if err := StartApp(serial, pkg); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return fmt.Errorf("không mở được app TikTok trên điện thoại: %v", last)
}

// UIAutomatorDump dump cây UI màn hình hiện tại → XML (dùng cho discovery
// và trích dữ liệu marketplace).
func UIAutomatorDump(serial string) (string, error) {
	if _, err := Shell(serial, "uiautomator", "dump", "/sdcard/ui.xml"); err != nil {
		return "", err
	}
	out, err := Shell(serial, "cat", "/sdcard/ui.xml")
	if err != nil {
		return "", err
	}
	if !strings.Contains(out, "<hierarchy") {
		return "", fmt.Errorf("dump UI thất bại: output không phải XML hierarchy")
	}
	return out, nil
}

// Screencap chụp màn hình → PNG bytes (đối chiếu khi discovery).
func Screencap(serial string) ([]byte, error) {
	adb, err := LookADB()
	if err != nil {
		return nil, err
	}
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "exec-out", "screencap", "-p")
	cmd := exec.Command(adb, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("screencap: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	if out.Len() < 100 {
		return nil, fmt.Errorf("screencap trả về dữ liệu rỗng")
	}
	return out.Bytes(), nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
