//go:build !windows

package cli

import "syscall"

func guiSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
