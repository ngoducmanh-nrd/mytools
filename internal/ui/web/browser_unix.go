//go:build !windows

package web

import (
	"fmt"
	"os/exec"
)

// OpenBrowser mở URL trong trình duyệt mặc định trên Linux/macOS.
func OpenBrowser(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL rỗng")
	}
	return exec.Command("xdg-open", rawURL).Start()
}
