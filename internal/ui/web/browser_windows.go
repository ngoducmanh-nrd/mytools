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

// openDir mở thư mục trong Windows Explorer. Là biến để test thay thế được.
var openDir = func(path string) error {
	if path == "" {
		return fmt.Errorf("đường dẫn rỗng")
	}
	return exec.Command("explorer", path).Start()
}
