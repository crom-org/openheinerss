//go:build linux

package process

import "syscall"

// morrerComOPai pede ao kernel que mate o filho se o processo que o criou morrer (inclusive por SIGKILL),
// para o CLI do motor não ficar vivo, órfão, depois de um runner derrubado.
func morrerComOPai(a *syscall.SysProcAttr) { a.Pdeathsig = syscall.SIGKILL }
