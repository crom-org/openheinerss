package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/codex"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// SecoResult é a descrição completa de uma execução que ainda não começou.
// Valores de ambiente potencialmente secretos aparecem sempre como ***.
type SecoResult struct {
	Seco             bool              `json:"seco"`
	Nome             string            `json:"nome"`
	Instancia        string            `json:"instancia"`
	Identidade       protocol.Identity `json:"identidade"`
	Base             string            `json:"base"`
	Modelo           string            `json:"modelo"`
	Esforco          string            `json:"esforco,omitempty"`
	Worktree         string            `json:"worktree"`
	Branch           string            `json:"branch"`
	Prompt           string            `json:"prompt"`
	Argv             []string          `json:"argv"`
	Env              map[string]string `json:"env"`
	Limites          SecoLimites       `json:"limites"`
	TrocaConta       SecoTrocaConta    `json:"trocaConta"`
	PastasPermitidas []string          `json:"pastasPermitidas,omitempty"`
	// PastasLeitura: pastas liberadas só para leitura (sistema por padrão + --permitir-leitura).
	PastasLeitura []string `json:"pastasLeitura,omitempty"`
}

type SecoLimites struct {
	MaxAgentes        int     `json:"maxAgentes"`
	Tentativas        int     `json:"tentativas"`
	CargaMaxima       float64 `json:"cargaMaxima"`
	QuandoCargaAbaixo float64 `json:"quandoCargaAbaixo"`
	CotaMax           float64 `json:"cotaMax"`
	LimiteContexto    int     `json:"limiteContexto"`
	AcaoContexto      string  `json:"acaoContexto,omitempty"`
}

type SecoTrocaConta struct {
	Decisao    string              `json:"decisao"`
	Fonte      string              `json:"fonte"`
	Instancia  string              `json:"instancia"`
	Percentual float64             `json:"percentual,omitempty"`
	Cache      []limites.Instancia `json:"cache,omitempty"`
}

// Seco resolve tudo que é possível sem criar arquivos, worktree, branch, log,
// meta ou processo de harness. A consulta de cota é somente local (cache).
func Seco(ctx context.Context, cwd string, opts Options) (SecoResult, error) {
	_ = ctx // reservado para manter o contrato compatível com Run
	o := opts.defaults()
	if o.Name == "" || o.Motor == "" {
		return SecoResult{}, fmt.Errorf("rodar seco exige nome e instância/harness")
	}
	if !nomeValido.MatchString(o.Name) {
		return SecoResult{}, fmt.Errorf("nome de agente inválido %q", o.Name)
	}
	repo, err := gitRoot(cwd)
	if err != nil {
		return SecoResult{}, err
	}
	if o.AgentsDir == "" {
		o.AgentsDir = os.Getenv("AGENTES")
	}
	if o.AgentsDir == "" {
		o.AgentsDir = ".claude/agentes"
	}
	agents, err := ValidateAgentsDir(repo, o.AgentsDir)
	if err != nil {
		return SecoResult{}, err
	}
	if cfg, e := config.LoadProject(repo); e == nil {
		if o.RegrasPadrao == "" {
			o.RegrasPadrao = cfg.RegrasPadrao
		}
		if cfg.SemRegrasPadrao {
			o.SemRegras = true
		}
		if o.QuotaMax <= 0 {
			o.QuotaMax = cfg.CotaMax
		}
	}
	if o.RegrasPadrao != "" && !filepath.IsAbs(o.RegrasPadrao) {
		o.RegrasPadrao = filepath.Join(repo, o.RegrasPadrao)
	}
	prompt, err := readPromptOptions(agents, o.Name, o.PromptFile, o.PromptText, o.Regras, o.RegrasPadrao, o.SemRegras)
	if err != nil {
		return SecoResult{}, err
	}
	work := filepath.Join(agents, o.Name)
	base := o.BranchBase
	if base == "" {
		base, err = resolveBase(repo, "")
	}
	if err != nil {
		return SecoResult{}, err
	}
	if o.Retomar {
		if b, e := os.ReadFile(filepath.Join(agents, "logs", o.Name+".estado.md")); e == nil && strings.TrimSpace(string(b)) != "" {
			prompt += "\n\n--- ESTADO DA EXECUÇÃO ANTERIOR ---\n" + strings.TrimSpace(string(b))
		}
		prompt += continuation
	}
	baseHarnessName, model, effort, env, argv := secoComando(o)
	inst := []string(nil)
	if s, ok := harness.CustomSpecFor(o.Motor); ok {
		inst = s.PastasPermitidas
	}
	permitidas, err := config.PastasPermitidasEfetivas(repo, append(append([]string(nil), o.PastasPermitidas...), inst...))
	if err != nil {
		return SecoResult{}, err
	}
	for _, dir := range permitidas {
		switch baseHarnessName {
		case "codex", "claude-code", "claude", "agy":
			argv = append(argv[:len(argv)-1], "--add-dir", dir, argv[len(argv)-1])
		}
	}
	leitura, err := config.PastasLeituraEfetivas(o.PastasLeitura)
	if err != nil {
		return SecoResult{}, err
	}
	if baseHarnessName == "opencode" {
		root := map[string]interface{}{}
		harness.AplicarPermissoesPastasOpenCode(root, work, permitidas, leitura)
		b, _ := json.Marshal(root)
		env["OPENCODE_CONFIG_CONTENT"] = string(b)
	}
	id, err := identidade.Para(o.Motor, env)
	if err != nil {
		return SecoResult{}, err
	}
	if model == "" && baseHarnessName == "codex" {
		model = codex.ModeloPadrao
	}
	if effort == "" && baseHarnessName == "codex" {
		effort = codex.EsforcoPadrao
	}
	if model == "" {
		model = "padrão"
	}
	cache := limites.Obter()
	contexto, _ := config.ContextoEfetivo(repo, o.Motor, baseHarnessName)
	if o.LimiteContexto != nil {
		contexto.LimiteTokens = *o.LimiteContexto
		contexto.Acao = o.AcaoContexto
	}
	percentual := 0.0
	for _, i := range cache.Instancias {
		if i.Nome == o.Motor {
			for _, j := range i.Janelas {
				if j.Percentual > percentual {
					percentual = j.Percentual
				}
			}
			break
		}
	}
	decisao := "não trocar: --cota-max desligado"
	if o.QuotaMax > 0 {
		decisao = fmt.Sprintf("trocar se o cache local indicar cota acima de %.1f%%; nenhuma consulta ativa", o.QuotaMax)
	}
	for k := range env {
		if !envPublico[k] {
			env[k] = "***"
		}
	}
	return SecoResult{Seco: true, Nome: o.Name, Instancia: o.Motor, Identidade: id, Base: baseHarnessName, Modelo: model, Esforco: effort,
		Worktree: work, Branch: "agente/" + o.Name, Prompt: prompt, Argv: argv, Env: env, PastasPermitidas: permitidas, PastasLeitura: leitura,
		Limites:    SecoLimites{MaxAgentes: o.MaxAgents, Tentativas: o.Attempts, CargaMaxima: o.MaxLoad, QuandoCargaAbaixo: o.WhenLoadBelow, CotaMax: o.QuotaMax, LimiteContexto: contexto.LimiteTokens, AcaoContexto: contexto.Acao},
		TrocaConta: SecoTrocaConta{Decisao: decisao, Fonte: "cache local; sem consulta ativa", Instancia: o.Motor, Percentual: percentual, Cache: cache.Instancias}}, nil
}

func secoComando(o Options) (string, string, string, map[string]string, []string) {
	env := map[string]string{}
	base, model, effort := o.Motor, o.Model, o.Effort
	for ok := true; ok; {
		s, custom := harness.CustomSpecFor(base)
		if !custom {
			break
		}
		if model == "" {
			model = s.Model
		}
		if effort == "" {
			effort = s.Effort
		}
		for k, v := range s.Env {
			env[k] = v
		}
		if s.Command != "" {
			a := append([]string{s.Command}, s.Args...)
			if s.Prompt == "argument" {
				a = append(a, "<prompt>")
			}
			return base, model, effort, env, a
		}
		base = s.Base
	}
	var a []string
	switch base {
	case "codex":
		a = []string{"codex", "exec", "--json", "-m", modelOr(model, codex.ModeloPadrao), "-c", "model_reasoning_effort=" + modelOr(effort, codex.EsforcoPadrao), "--dangerously-bypass-approvals-and-sandbox", "<prompt>"}
	case "claude-code", "claude":
		a = []string{"claude", "-p", "<prompt>", "--output-format", "stream-json", "--verbose"}
		if model != "" {
			a = append(a, "--model", model)
		}
	case "opencode":
		a = []string{"opencode", "run", "--format", "json"}
		if model != "" {
			a = append(a, "--model", model)
		}
		a = append(a, "<prompt>")
	case "agy":
		a = []string{"agy"}
		if model != "" {
			a = append(a, "--model", model)
		}
		a = append(a, "--dangerously-skip-permissions", "--output-format", "stream-json", "-p", "<prompt>")
	case "aider":
		a = []string{"aider", "--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--no-browser", "--message", "<prompt>"}
		if model != "" {
			a = append(a, "--model", model)
		}
	default:
		a = []string{base}
		if model != "" {
			a = append(a, "--model", model)
		}
		a = append(a, "<prompt>")
	}
	for k := range env {
		if _, ok := envPublico[k]; !ok {
			env[k] = "***"
		}
	}
	return base, model, effort, env, append(a, o.HarnessArgs...)
}

func modelOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// SecoJSON é uma conveniência para RPC/SDKs que precisam de bytes estáveis.
func SecoJSON(ctx context.Context, cwd string, opts Options) ([]byte, error) {
	r, e := Seco(ctx, cwd, opts)
	if e != nil {
		return nil, e
	}
	return json.Marshal(r)
}
