//go:build windows

package runner

import "syscall"

const detachedProcess = 0x00000008

// DetachedSysProcAttr giúp tiến trình con chạy tách khỏi hub: không cửa sổ console, nhóm tiến trình riêng.
func DetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
