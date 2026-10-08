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
