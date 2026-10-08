//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package process

import (
	"os/exec"
	"syscall"
	"time"
)

// Configure isolates the child in its own process group on Unix systems. Cancelar o contexto
// do comando mata o grupo inteiro (o CLI e os filhos dele), não só o líder; sem isso os
// filhos ficavam órfãos, e um SIGINT herdado como "ignorado" (shell com `&`) não os parava.
func Configure(cmd *exec.Cmd) {
	// Mantém um supervisor como líder do grupo. Pdeathsig é aplicado ao
	// supervisor; o trap encerra o grupo inteiro, inclusive netos do CLI.
	path := cmd.Path
	args := append([]string(nil), cmd.Args[1:]...)
	cmd.Path = "/bin/sh"
	cmd.Args = append([]string{"sh", "-c", `trap 'kill -TERM -- -$$ 2>/dev/null' TERM; "$@" & pid=$!; wait "$pid"`, "sh", path}, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	morrerComOPai(cmd.SysProcAttr)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
}

func Interrupt(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

func Kill(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
