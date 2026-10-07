//go:build windows

package subs

import "syscall"

// singBoxProcAttr hides the sidecar's console window: sing-box runs for the
// whole app lifetime and a stray terminal would sit in the taskbar.
func singBoxProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
