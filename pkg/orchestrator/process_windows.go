//go:build windows

package orchestrator

import (
	"os"
	"syscall"
)

const windowsStillActive = 259

func processAlivePlatform(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == windowsStillActive
}

func stopAgentProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// DesligarDoGrupo não tem efeito no Windows.
func DesligarDoGrupo() {}
