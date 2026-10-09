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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/identidade"
)

type Janela struct {
	Nome       string  `json:"nome"`
	Percentual float64 `json:"percentual"`
	// ReiniciaEm permanece no Go para compatibilidade; o contrato JSON chama o
	// horário de voltaEm.
	ReiniciaEm string `json:"-"`
	VoltaEm    string `json:"voltaEm,omitempty"`
}

type Instancia struct {
	Nome          string   `json:"nome"`
	Base          string   `json:"base"`
	Janelas       []Janela `json:"janelas,omitempty"`
	DadoEm        string   `json:"dadoEm,omitempty"`
	IdadeSegundos int64    `json:"idadeSegundos,omitempty"`
	Nota          string   `json:"nota,omitempty"`
	Fonte         string   `json:"fonte"`
	ContaID       string   `json:"contaId,omitempty"`
	ContaIDFonte  string   `json:"contaIdFonte,omitempty"`
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
	porConta := map[string]string{}
	add := func(i Instancia) {
		i.Janelas = janelasVigentes(i.Janelas, agora)
		if i.ContaID != "" {
			if outra, ok := porConta[i.ContaID]; ok && outra != i.Nome {
				i.Nota = fmt.Sprintf("mesma conta que %s", outra)
				for _, anterior := range instancias {
					if anterior.Nome == outra && len(i.Janelas) == 0 {
						i.Janelas, i.DadoEm, i.Fonte = anterior.Janelas, anterior.DadoEm, anterior.Fonte
					}
				}
			} else {
				porConta[i.ContaID] = i.Nome
			}
		}
		if !seen[i.Nome] {
			seen[i.Nome] = true
			instancias = append(instancias, i)
		}
	}
	addCodex := func(nome, home string) {
		if home == "" || seen[nome] {
			return
		}
		if i, ok := lerCodex(nome, home, agora); ok {
			add(i)
		}
	}
	// As instâncias base são independentes do diretório atual. O ambiente ainda
	// pode escolher outro diretório para a instância principal.
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
	// Descobre contas locais sem nomes fixos no código: a conta principal e
	// todas as pastas ~/.claude-contaN. A existência da pasta é suficiente para
	// listá-la; a ausência do statusline vira uma nota "sem dado".
	if !seen["claude-code"] {
		addClaude("claude-code", expandHome(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(userHome(), ".claude")), agora, add)
	}
	for _, conta := range contasClaudeLocais() {
		addClaude(conta.nome, conta.dir, agora, add)
	}
	sort.Slice(instancias, func(i, j int) bool { return instancias[i].Nome < instancias[j].Nome })
	return Resultado{Agora: agora.Format(time.RFC3339), Instancias: instancias}
}

type contaLocal struct {
	nome string
	dir  string
}

func addClaude(nome, dir string, agora time.Time, add func(Instancia)) {
	if nome == "" || dir == "" {
		return
	}
	if i, ok := lerClaude(nome, dir, agora); ok {
		add(i)
	}
}

func contasClaudeLocais() []contaLocal {
	home := userHome()
	result := make([]contaLocal, 0)
	entries, err := os.ReadDir(home)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() || !reContaClaude.MatchString(entry.Name()) {
			continue
		}
		result = append(result, contaLocal{nome: strings.TrimPrefix(entry.Name(), "."), dir: filepath.Join(home, entry.Name())})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].nome < result[j].nome })
	return result
}

// Percentual devolve o maior percentual conhecido da instância. Sem leitura,
// false evita trocar de reserva por falta de informação.
func Percentual(nome string) (float64, bool) {
	agora := time.Now()
	for _, i := range Obter().Instancias {
		if i.Nome == nome {
			return maiorPercentual(i, agora)
		}
	}
	return 0, false
}

// maiorPercentual ignora janelas cujo horário de reinício já passou: a leitura é de antes da
// renovação e manter o percentual antigo bloquearia a instância para sempre.
func maiorPercentual(i Instancia, agora time.Time) (float64, bool) {
	var maior float64
	validas := 0
	for _, j := range i.Janelas {
		if voltaEm(j) != "" {
			if t, err := time.Parse(time.RFC3339, voltaEm(j)); err == nil && t.Before(agora) {
				continue
			}
		}
		validas++
		if j.Percentual > maior {
			maior = j.Percentual
		}
	}
	return maior, validas > 0
}

func janelasVigentes(janelas []Janela, agora time.Time) []Janela {
	result := make([]Janela, 0, len(janelas))
	legadas := 0
	for _, j := range janelas {
		if voltaEm(j) != "" {
			if t, err := time.Parse(time.RFC3339, voltaEm(j)); err == nil && t.Before(agora) {
				continue
			}
		}
		if j.VoltaEm == "" {
			j.VoltaEm = j.ReiniciaEm
		}
		j.ReiniciaEm = j.VoltaEm
		if j.Nome == "limite" || j.Nome == "" {
			if legadas == 0 {
				j.Nome = "5h"
			} else if legadas == 1 {
				j.Nome = "semana"
			} else {
				j.Nome = fmt.Sprintf("janela-%d", legadas+1)
			}
			legadas++
		}
		result = append(result, j)
	}
	return result
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
	// Uma leitura de data por arquivo (o comparador chamava os.Stat a cada comparação).
	datas := make(map[string]time.Time, len(paths))
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			datas[p] = st.ModTime()
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if datas[paths[i]].Equal(datas[paths[j]]) {
			return paths[i] < paths[j]
		}
		return datas[paths[i]].After(datas[paths[j]])
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
		i := Instancia{Nome: nome, Base: "codex", Nota: "sem dado: sem leitura de limite nos arquivos de sessão", Fonte: "log"}
		i.ContaID, i.ContaIDFonte = identidade.IDPara("codex", home)
		return i, true
	}
	em := parseTime(best.Timestamp, agora)
	i := Instancia{Nome: nome, Base: "codex", DadoEm: em.Format(time.RFC3339), IdadeSegundos: int64(agora.Sub(em).Seconds())}
	i.ContaID, i.ContaIDFonte = identidade.IDPara("codex", home)
	i.Fonte = "log"
	for _, w := range []*rateWindow{best.Payload.RateLimits.Primary, best.Payload.RateLimits.Secondary} {
		if w == nil {
			continue
		}
		nomeJanela := fmt.Sprintf("janela-%dmin", w.WindowMinutes)
		if w.WindowMinutes == 300 {
			nomeJanela = "5h"
		} else if w.WindowMinutes == 10080 {
			nomeJanela = "semana"
		}
		j := Janela{Nome: nomeJanela, Percentual: w.UsedPercent}
		if w.ResetsAt > 0 {
			j = comVoltaEm(j, time.Unix(int64(w.ResetsAt), 0).Format(time.RFC3339))
		}
		i.Janelas = append(i.Janelas, j)
	}
	return i, true
}

func lerClaude(nome, dir string, agora time.Time) (Instancia, bool) {
	// O statusline do crom-painel usa o id da conta: ~/.claude → conta1, ~/.claude-contaN → contaN.
	statusPath := filepath.Join(userHome(), ".config", "crom-painel", "statusline-"+contaClaude(nome, dir)+".json")
	b, err := os.ReadFile(statusPath)
	if err != nil {
		i := Instancia{Nome: nome, Base: "claude-code", Nota: "sem dado: statusline não encontrado", Fonte: "statusline"}
		i.ContaID, i.ContaIDFonte = identidade.IDPara("claude-code", dir)
		return i, true
	}
	var v struct {
		Em         float64 `json:"em"`
		RateLimits struct {
			Five  *rateWindow `json:"five_hour"`
			Seven *rateWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(b, &v) != nil {
		i := Instancia{Nome: nome, Base: "claude-code", Nota: "sem dado: statusline inválido", Fonte: "statusline"}
		i.ContaID, i.ContaIDFonte = identidade.IDPara("claude-code", dir)
		return i, true
	}
	em := time.UnixMilli(int64(v.Em))
	if v.Em < 1e12 {
		em = time.Unix(int64(v.Em), 0)
	}
	i := Instancia{Nome: nome, Base: "claude-code", DadoEm: em.Format(time.RFC3339), IdadeSegundos: int64(agora.Sub(em).Seconds()), Fonte: "statusline"}
	i.ContaID, i.ContaIDFonte = identidade.IDPara("claude-code", dir)
	for _, x := range []struct {
		name string
		w    *rateWindow
	}{{"5h", v.RateLimits.Five}, {"semana", v.RateLimits.Seven}} {
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
			j = comVoltaEm(j, time.UnixMilli(int64(r)).Format(time.RFC3339))
		}
		i.Janelas = append(i.Janelas, j)
	}
	return i, true
}

func voltaEm(j Janela) string {
	if j.VoltaEm != "" {
		return j.VoltaEm
	}
	return j.ReiniciaEm
}

func comVoltaEm(j Janela, valor string) Janela {
	j.ReiniciaEm = valor
	j.VoltaEm = valor
	return j
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

var reContaClaude = regexp.MustCompile(`^\.claude-(conta\d+)$`)

// contaClaude acha o id da conta do Claude a partir do CLAUDE_CONFIG_DIR da instância.
func contaClaude(nome, dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	if dir == "" || base == ".claude" {
		return "conta1"
	}
	if m := reContaClaude.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	return nome
}
