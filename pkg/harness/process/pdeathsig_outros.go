//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package process

import "syscall"

// morrerComOPai: só o Linux tem Pdeathsig; nos outros sistemas o grupo de processos é o que resta.
func morrerComOPai(*syscall.SysProcAttr) {}
