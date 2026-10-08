//go:build windows

package process

import (
	"os"
	"os/exec"
)

// Configure is a no-op on Windows, where process groups use different APIs.
func Configure(cmd *exec.Cmd) {}

func Interrupt(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}

func Kill(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

// UseSupervisor não faz nada no Windows.
func UseSupervisor(string) {}

// SupervisorArg é o argumento do subcomando oculto de supervisão (inativo no Windows).
const SupervisorArg = "__supervisor"

// RunSupervisor não é suportado no Windows.
func RunSupervisor([]string) int { return 2 }
