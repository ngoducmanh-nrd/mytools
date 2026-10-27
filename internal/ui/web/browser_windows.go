//go:build windows

package web

import (
	"fmt"
	"os/exec"
)

// OpenBrowser mở URL trong trình duyệt mặc định trên Windows.
func OpenBrowser(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL rỗng")
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	return cmd.Start()
}
