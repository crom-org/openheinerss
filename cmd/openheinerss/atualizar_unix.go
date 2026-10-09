//go:build !windows

package main

import (
	"os"
	"syscall"
)

func atributosDesacoplado() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// donoSouEu diz se o caminho pertence ao usuário atual (só reinicia processos próprios).
func donoSouEu(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Getuid()
}

func sinalizarTerm(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
