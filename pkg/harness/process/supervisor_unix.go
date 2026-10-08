//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// prazoTerm é quanto o supervisor espera depois de SIGTERM antes de mandar SIGKILL ao grupo.
const prazoTerm = 3 * time.Second

// RunSupervisor implementa `openheinerss __supervisor -- <cmd> <args>` e devolve o código de saída.
// Roda como líder do grupo (o runner usou Setpgid); o motor fica no mesmo grupo, com stdin/stdout/stderr
// herdados sem cópia. SIGTERM (inclusive o do Pdeathsig) derruba o grupo: TERM, e KILL após o prazo.
func RunSupervisor(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "uso: __supervisor -- <comando> [args...]")
		return 2
	}
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return 127
		}
		return 126
	}
	pgrp := syscall.Getpgrp()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			return codigoDeSaida(cmd, err)
		case s := <-sigs:
			if s == syscall.SIGINT {
				continue // o Interrupt do runner vai ao grupo todo; o motor decide como encerrar
			}
			_ = syscall.Kill(-pgrp, syscall.SIGTERM)
			select {
			case err := <-done:
				return codigoDeSaida(cmd, err)
			case <-time.After(prazoTerm):
			}
			_ = syscall.Kill(-pgrp, syscall.SIGKILL) // mata também o supervisor
			return 137
		}
	}
}

func codigoDeSaida(cmd *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}
	if st, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		if st.Signaled() {
			return 128 + int(st.Signal())
		}
		return st.ExitStatus()
	}
	return 1
}
