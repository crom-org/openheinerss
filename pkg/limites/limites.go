// Package limites lê as cotas que os CLIs já salvam localmente.
// Nunca abre arquivos de credenciais, tokens ou .env.
package limites

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

type Janela struct {
	Nome       string  `json:"nome"`
	Percentual float64 `json:"percentual"`
	ReiniciaEm string  `json:"reiniciaEm,omitempty"`
}

type Instancia struct {
	Nome          string   `json:"nome"`
	Base          string   `json:"base"`
	Janelas       []Janela `json:"janelas,omitempty"`
	DadoEm        string   `json:"dadoEm,omitempty"`
	IdadeSegundos int64    `json:"idadeSegundos,omitempty"`
	Nota          string   `json:"nota,omitempty"`
}

type Resultado struct {
	Agora      string      `json:"agora"`
	Instancias []Instancia `json:"instancias"`
}

type rateLimits struct {
	Primary   *rateWindow `json:"primary"`
	Secondary *rateWindow `json:"secondary"`
	PlanType  string      `json:"plan_type"`
}
type rateWindow struct {
	WindowMinutes  int     `json:"window_minutes"`
	UsedPercent    float64 `json:"used_percent"`
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       float64 `json:"resets_at"`
}

type codexEvent struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		RateLimits *rateLimits `json:"rate_limits"`
	} `json:"payload"`
}

func Obter() Resultado {
	agora := time.Now()
	instancias := make([]Instancia, 0)
	seen := map[string]bool{}
	add := func(i Instancia) {
		if !seen[i.Nome] {
			seen[i.Nome] = true
			instancias = append(instancias, i)
		}
	}
	addCodex := func(nome, home string) {
		if home == "" {
			return
		}
		if i, ok := lerCodex(nome, home, agora); ok {
			add(i)
		}
	}
	// A instância base usa o ambiente corrente; instâncias custom podem apontar
	// para outro CODEX_HOME ou CLAUDE_CONFIG_DIR.
	addCodex("codex", expandHome(os.Getenv("CODEX_HOME"), filepath.Join(userHome(), ".codex")))
	for _, nome := range nomesCustom() {
		spec, _ := harness.CustomSpecFor(nome)
		base := spec.Base
		if base == "codex" {
			addCodex(nome, expandHome(spec.Env["CODEX_HOME"], expandHome(os.Getenv("CODEX_HOME"), filepath.Join(userHome(), ".codex"))))
		}
		if base == "claude-code" {
			if i, ok := lerClaude(nome, expandHome(spec.Env["CLAUDE_CONFIG_DIR"], expandHome(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(userHome(), ".claude"))), agora); ok {
				add(i)
			}
		}
	}
	if !seen["claude-code"] {
		if i, ok := lerClaude("claude-code", expandHome(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(userHome(), ".claude")), agora); ok {
			add(i)
		}
	}
	sort.Slice(instancias, func(i, j int) bool { return instancias[i].Nome < instancias[j].Nome })
	return Resultado{Agora: agora.Format(time.RFC3339), Instancias: instancias}
}

// Percentual devolve o maior percentual conhecido da instância. Sem leitura,
// false evita trocar de reserva por falta de informação.
func Percentual(nome string) (float64, bool) {
	for _, i := range Obter().Instancias {
		if i.Nome == nome {
			var maior float64
			for _, j := range i.Janelas {
				if j.Percentual > maior {
					maior = j.Percentual
				}
			}
			return maior, len(i.Janelas) > 0
		}
	}
	return 0, false
}

func nomesCustom() []string {
	items := harness.ListCatalog()
	result := make([]string, 0)
	for _, item := range items {
		if item.Origin == "custom" {
			result = append(result, item.ID)
		}
	}
	return result
}

func lerCodex(nome, home string, agora time.Time) (Instancia, bool) {
	base := filepath.Join(home, "sessions")
	var paths []string
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool {
		ai, _ := os.Stat(paths[i])
		aj, _ := os.Stat(paths[j])
		return ai.ModTime().After(aj.ModTime())
	})
	if len(paths) > 14 {
		paths = paths[:14]
	}
	var best *codexEvent
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if st, statErr := f.Stat(); statErr == nil && st.Size() > 600_000 {
			_, _ = f.Seek(-600_000, 2)
		}
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for s.Scan() {
			if !strings.Contains(s.Text(), "rate_limits") {
				continue
			}
			var e codexEvent
			if json.Unmarshal([]byte(s.Text()), &e) == nil && e.Payload.RateLimits != nil {
				if best == nil || e.Timestamp > best.Timestamp {
					copy := e
					best = &copy
				}
			}
		}
		_ = f.Close()
	}
	if best == nil {
		return Instancia{Nome: nome, Base: "codex", Nota: "sem leitura de limite nos arquivos de sessão"}, true
	}
	em := parseTime(best.Timestamp, agora)
	i := Instancia{Nome: nome, Base: "codex", DadoEm: em.Format(time.RFC3339), IdadeSegundos: int64(agora.Sub(em).Seconds())}
	for _, w := range []*rateWindow{best.Payload.RateLimits.Primary, best.Payload.RateLimits.Secondary} {
		if w == nil {
			continue
		}
		nomeJanela := fmt.Sprintf("janela de %d min", w.WindowMinutes)
		if w.WindowMinutes == 300 {
			nomeJanela = "5 h"
		} else if w.WindowMinutes == 10080 {
			nomeJanela = "semanal"
		}
		j := Janela{Nome: nomeJanela, Percentual: w.UsedPercent}
		if w.ResetsAt > 0 {
			j.ReiniciaEm = time.Unix(int64(w.ResetsAt), 0).Format(time.RFC3339)
		}
		i.Janelas = append(i.Janelas, j)
	}
	return i, true
}

func lerClaude(nome, dir string, agora time.Time) (Instancia, bool) {
	_ = dir // O diretório identifica a instância; o statusline usa o id dela no nome do arquivo.
	statusPath := filepath.Join(userHome(), ".config", "crom-painel", "statusline-"+nome+".json")
	b, err := os.ReadFile(statusPath)
	if err != nil && nome == "claude-code" {
		b, err = os.ReadFile(filepath.Join(userHome(), ".config", "crom-painel", "statusline-conta1.json"))
	}
	if err != nil {
		return Instancia{Nome: nome, Base: "claude-code", Nota: "statusline não encontrado"}, true
	}
	var v struct {
		Em         float64 `json:"em"`
		RateLimits struct {
			Five  *rateWindow `json:"five_hour"`
			Seven *rateWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(b, &v) != nil {
		return Instancia{Nome: nome, Base: "claude-code", Nota: "statusline inválido"}, true
	}
	em := time.UnixMilli(int64(v.Em))
	if v.Em < 1e12 {
		em = time.Unix(int64(v.Em), 0)
	}
	i := Instancia{Nome: nome, Base: "claude-code", DadoEm: em.Format(time.RFC3339), IdadeSegundos: int64(agora.Sub(em).Seconds())}
	for _, x := range []struct {
		name string
		w    *rateWindow
	}{{"5 h", v.RateLimits.Five}, {"semanal", v.RateLimits.Seven}} {
		if x.w == nil {
			continue
		}
		percentual := x.w.UsedPercent
		if percentual == 0 && x.w.UsedPercentage != 0 {
			percentual = x.w.UsedPercentage
		}
		j := Janela{Nome: x.name, Percentual: percentual}
		if x.w.ResetsAt > 0 {
			r := x.w.ResetsAt
			if r < 1e12 {
				r *= 1000
			}
			j.ReiniciaEm = time.UnixMilli(int64(r)).Format(time.RFC3339)
		}
		i.Janelas = append(i.Janelas, j)
	}
	return i, true
}

func expandHome(value, fallback string) string {
	if value == "" {
		return fallback
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(userHome(), value[2:])
	}
	return value
}
func userHome() string { h, _ := os.UserHomeDir(); return h }
func parseTime(value string, fallback time.Time) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback
	}
	return t
}
