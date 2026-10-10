package versoes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// registrar grava o evento harness.atualizado no histórico (JSONL) e a linha no log de eventos.
// Falhas de gravação viram passo no resultado: não derrubam uma atualização já feita.
func (e Ambiente) registrar(r *Resultado, o Opcoes) {
	if r.Seco {
		return
	}
	b, _ := json.Marshal(struct {
		Evento string `json:"evento"`
		*Resultado
	}{"harness.atualizado", r})
	if p, err := ArquivoHistorico(); err == nil {
		if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			var f *os.File
			if f, err = os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				_, err = f.Write(append(b, '\n'))
				f.Close()
			}
		}
		if err != nil {
			r.Passos = append(r.Passos, "histórico não gravado: "+err.Error())
		}
	}
	if o.EventLog != "" {
		f, err := os.OpenFile(o.EventLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			r.Passos = append(r.Passos, "log de eventos não gravado: "+err.Error())
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "[harness] ATUALIZADO %s %s → %s resultado %s\n", r.Harness, r.Antes, r.Depois, r.Resultado)
	}
}
