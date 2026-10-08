//go:build !windows

package orchestrator

import (
	"errors"
	"os"
	"syscall"
)

func processAlivePlatform(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

// stopAgentProcess preserva o comportamento Unix: só mata o grupo quando o
// processo registrado é o líder; caso contrário, sinaliza apenas o PID.
func stopAgentProcess(pid int) error {
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// DesligarDoGrupo põe o processo numa sessão própria. Um `rodar` filho, lançado com `&` de dentro do
// harness do pai, sairia junto quando o pai encerra o grupo de processos no fim do turno.
func DesligarDoGrupo() { _, _ = syscall.Setsid() }
