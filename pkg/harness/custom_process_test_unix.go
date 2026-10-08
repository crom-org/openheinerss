//go:build !windows

package harness

import "syscall"

func processAliveForTest(pid int) bool { return syscall.Kill(pid, 0) == nil }
func killForTest(pid int) error        { return syscall.Kill(pid, syscall.SIGKILL) }
