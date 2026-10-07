//go:build !windows

package subs

import "syscall"

// singBoxProcAttr is a no-op off Windows: a daemonised background process
// needs no window-hiding attributes there.
func singBoxProcAttr() *syscall.SysProcAttr { return nil }
