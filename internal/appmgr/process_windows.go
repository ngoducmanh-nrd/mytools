//go:build windows

package appmgr

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func ProcessStartTime(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid %d không hợp lệ", pid)
	}

	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("mở tiến trình %d: %w", pid, err)
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return "", fmt.Errorf("đọc trạng thái tiến trình %d: %w", pid, err)
	}
	if exitCode != stillActive {
		return "", fmt.Errorf("tiến trình %d đã thoát", pid)
	}

	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", fmt.Errorf("đọc thời điểm bắt đầu của tiến trình %d: %w", pid, err)
	}
	return time.Unix(0, creation.Nanoseconds()).UTC().Format(time.RFC3339Nano), nil
}

func killProcessTree(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("pid %d không hợp lệ", pid)
	}

	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill PID %d: %w%s", pid, err, tailText(string(output)))
	}
	return nil
}
