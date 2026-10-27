//go:build !windows

package appmgr

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func ProcessStartTime(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid %d không hợp lệ", pid)
	}

	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("đọc thời điểm bắt đầu của tiến trình %d: %w", pid, err)
	}

	text := strings.Join(strings.Fields(string(output)), " ")
	if text == "" {
		return "", fmt.Errorf("không có thông tin thời điểm bắt đầu của tiến trình %d", pid)
	}
	started, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", text, time.UTC)
	if err != nil {
		return "", fmt.Errorf("không đọc được thời điểm bắt đầu %q của tiến trình %d: %w", text, pid, err)
	}
	return started.UTC().Format(time.RFC3339), nil
}

func killProcessTree(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("pid %d không hợp lệ", pid)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill nhóm tiến trình %d: %w", pid, err)
	}
	return nil
}
