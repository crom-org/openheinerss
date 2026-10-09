//go:build windows

package limites

import (
	"fmt"
	"os"
	"time"
)

const (
	intervaloEsperaTrava = 5 * time.Millisecond
	limiteEsperaTrava    = 30 * time.Second
	travaAbandonada      = 2 * time.Minute
)

// travarConta usa criação exclusiva no Windows, onde syscall.Flock não existe.
// O arquivo é removido ao liberar a trava; uma trava velha é considerada
// abandonada para evitar bloqueio permanente após uma queda do processo.
func travarConta(path string) (func(), error) {
	limite := time.Now().Add(limiteEsperaTrava)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err == nil {
			return func() {
				_ = f.Close()
				_ = os.Remove(path)
			}, nil
		}
		if !os.IsExist(err) {
			return func() {}, err
		}
		if st, statErr := os.Stat(path); statErr == nil && time.Since(st.ModTime()) > travaAbandonada {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(limite) {
			return func() {}, fmt.Errorf("trava ocupada há mais de %s", limiteEsperaTrava)
		}
		time.Sleep(intervaloEsperaTrava)
	}
}
