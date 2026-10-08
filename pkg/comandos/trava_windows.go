//go:build windows

package comandos

import (
	"fmt"
	"os"
	"time"
)

// comTrava no Windows usa criação exclusiva do arquivo de trava (removido ao liberar).
// Uma trava abandonada por um processo morto vence depois de travaAbandonada.
func comTrava(path string, fn func() error) error {
	limite := time.Now().Add(esperaTrava)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			defer os.Remove(path)
			defer f.Close()
			return fn()
		}
		if !os.IsExist(err) {
			return fmt.Errorf("criar trava %s: %w", path, err)
		}
		if st, e := os.Stat(path); e == nil && time.Since(st.ModTime()) > travaAbandonada {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(limite) {
			return fmt.Errorf("trava %s ocupada há mais de %s", path, esperaTrava)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const travaAbandonada = 2 * time.Minute
