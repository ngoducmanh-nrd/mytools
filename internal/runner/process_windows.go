//go:build windows

package runner

import "syscall"

const detachedProcess = 0x00000008

// noWindowProcess cho con chạy với console riêng không bao giờ hiện cửa sổ.
const noWindowProcess = 0x08000000

// DetachedSysProcAttr giúp tiến trình con chạy tách khỏi hub: không cửa sổ console, nhóm tiến trình riêng.
func DetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}

// HiddenSysProcAttr chạy tiến trình con mà không cửa sổ nào hiện ra, đồng thời
// console của nó độc lập với console cha nên không bị đóng theo cửa sổ terminal.
// Không dùng DETACHED_PROCESS vì PowerShell 5.1 khởi động kiểu đó thoát ngay
// mà không thực thi lệnh.
func HiddenSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: noWindowProcess,
	}
}
