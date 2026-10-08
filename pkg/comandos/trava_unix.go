//go:build !windows

package comandos

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// comTrava segura uma trava exclusiva entre processos (flock em path) enquanto fn roda.
func comTrava(path string, fn func() error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("criar trava %s: %w", path, err)
	}
	defer f.Close()
	limite := time.Now().Add(esperaTrava)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return fmt.Errorf("travar %s: %w", path, err)
		}
		if time.Now().After(limite) {
			return fmt.Errorf("trava %s ocupada há mais de %s", path, esperaTrava)
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
