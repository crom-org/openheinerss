//go:build linux

package process

import "syscall"

// morrerComOPai avisa o supervisor quando o processo que o criou morrer,
// inclusive por SIGKILL; o supervisor então encerra seu grupo inteiro.
func morrerComOPai(a *syscall.SysProcAttr) { a.Pdeathsig = syscall.SIGTERM }
