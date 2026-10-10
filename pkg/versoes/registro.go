package versoes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// Versões guardadas por base, para `harness voltar` sem argumento.
type guardada struct {
	Anterior string `json:"anterior"`
	Atual    string `json:"atual,omitempty"`
	Em       string `json:"em"`
}

func arquivoGuardadas() (string, error) {
	p, err := ArquivoHistorico()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "harness-versoes-anteriores.json"), nil
}

func lerGuardadas() map[string]guardada {
	m := map[string]guardada{}
	if p, err := arquivoGuardadas(); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &m)
		}
	}
	return m
}

// guardarAnterior grava a versão que estava instalada antes da troca.
func (e Ambiente) guardarAnterior(base, anterior, atual string) {
	p, err := arquivoGuardadas()
	if err != nil {
		return
	}
	m := lerGuardadas()
	m[base] = guardada{Anterior: anterior, Atual: atual, Em: e.Agora().UTC().Format("2006-01-02T15:04:05Z")}
	if b, err := json.MarshalIndent(m, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, b, 0o644)
	}
}

func (e Ambiente) anteriorGuardada(base string) (string, bool) {
	g, ok := lerGuardadas()[base]
	return g.Anterior, ok && g.Anterior != ""
}

var (
	reBearer   = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{8,}`)
	reSegredos = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{8,}|(?:api[_-]?key|token|secret|password|authorization)["']?\s*[:=]\s*["']?[^\s"',;]{6,})`)
)

// semSegredos mascara chaves e tokens que apareçam na saída de um teste.
func semSegredos(s string) string {
	return reSegredos.ReplaceAllString(reBearer.ReplaceAllString(s, "[segredo omitido]"), "[segredo omitido]")
}
