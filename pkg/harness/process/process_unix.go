//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package process

import (
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// SupervisorArg é o primeiro argumento do subcomando oculto do binário openheinerss que supervisiona um motor.
const SupervisorArg = "__supervisor"

var supervisorBin atomic.Value // string

// UseSupervisor informa o binário que entende `__supervisor`. Só o main do openheinerss chama; como
// biblioteca (SDK/servidor, testes) nada é registrado e Configure cai para Setpgid + kill do grupo.
func UseSupervisor(bin string) {
	if bin != "" {
		supervisorBin.Store(bin)
	}
}

// Configure isola o filho em um grupo de processos próprio. Cancelar o contexto do comando mata o
// grupo inteiro (o CLI e os filhos dele), não só o líder. Com supervisor disponível, ele é o líder do
// grupo, herda stdin/stdout/stderr intactos e, se o runner morrer (até por SIGKILL), recebe SIGTERM
// do kernel (Pdeathsig) e derruba o grupo todo, netos incluídos.
func Configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	morrerComOPai(cmd.SysProcAttr)
	if bin, _ := supervisorBin.Load().(string); bin != "" && cmd.Err == nil {
		args := append([]string{bin, SupervisorArg, "--", cmd.Path}, cmd.Args[1:]...)
		cmd.Path, cmd.Args = bin, args
	}
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
