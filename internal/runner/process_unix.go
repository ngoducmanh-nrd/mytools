//go:build !windows

package runner

import "syscall"

// DetachedSysProcAttr giúp tiến trình con chạy tách khỏi hub bằng session riêng.
func DetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// HiddenSysProcAttr trên Unix không cần ẩn cửa sổ; chạy trong session riêng
// như tiến trình tách rời để sống tiếp sau khi CLI thoát.
func HiddenSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
