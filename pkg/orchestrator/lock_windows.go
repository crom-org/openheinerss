//go:build windows

package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// No Windows, o fallback sem x/sys usa criação exclusiva. O arquivo é
// removido ao liberar a trava; a espera continua cancelável como no Unix.
func withFileLock(ctx context.Context, path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err == nil {
			defer os.Remove(path)
			defer f.Close()
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("esperando a trava %s: %w", filepath.Base(path), ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func acquireNameLock(agents, name string) (*os.File, error) {
	path := filepath.Join(agents, "logs", name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("agente %q já está rodando (trava de nome ocupada)", name)
		}
		return nil, fmt.Errorf("criar trava do agente %q: %w", name, err)
	}
	return f, nil
}

func releaseNameLock(f *os.File) {
	if f == nil {
		return
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
}
