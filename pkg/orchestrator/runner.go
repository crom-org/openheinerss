// Package orchestrator implementa a execução de uma missão no estilo do
// rodar.sh, sem embutir contas ou provedores no binário.
package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	"github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

const continuation = "\n\n--- CONTINUAÇÃO ---\nUma execução anterior desta MESMA tarefa foi interrompida (erro ou cota). NÃO recomece do zero: rode `git status` e `git log --oneline -10`, leia RELATORIO-AGENTE.md e os arquivos já alterados nesta pasta (ou os relatórios em .claude/agentes/relatorios/ se for missão), confira o que já está pronto e termine SOMENTE o que falta, depois finalize como a tarefa pede."
const missionRules = "\n\n--- MISSÃO SOMENTE LEITURA ---\nEsta é uma missão de inspeção. Não crie, altere, remova ou comite arquivos; não use worktree. Apenas leia e relate o que encontrar."
const defaultPromptRules = "--- REGRAS PADRÃO DO AGENTE ---\nTrabalhe somente dentro da pasta do agente e da worktree desta missão.\nÉ proibido buscar fora da worktree: não use `find /`, `find ~`, `locate`, varreduras de disco ou buscas equivalentes fora dela.\nSe lançar agentes filhos ou comandos em segundo plano, o openheinerss te acorda quando eles terminarem; não encerre dizendo que vai esperar sem ter lançado nada."

type Options struct {
	Name, Motor, Model, Effort, Mode, PromptFile string
	// PromptText é o prompt em texto; quando preenchido, vale no lugar de PromptFile e de prompts/<nome>.md.
	PromptText    string
	Conta, Regras string
	// RegrasPadrao é um arquivo opcional com regras adicionais/substitutas.
	RegrasPadrao          string
	SemRegras, Seco       bool
	KeysFile              string
	Retomar               bool
	AgentsDir, BranchBase string
	MaxLoad               float64
	MaxAgents, Attempts   int
	QuotaMax              float64
	EventLog              string
	// HarnessArgs vai intacto, na ordem, para o processo do harness (--arg/--harness-arg).
	HarnessArgs []string
	Load        func() (float64, error)
	Sleep       func(time.Duration)
	Now         func() time.Time
	// OnEvent recebe os eventos de orquestração (opcional; o CLI não usa).
	OnEvent func(Evento)
	// ViaServidor marca execuções lançadas por `serve`: o PID do meta.json é o do servidor.
	ViaServidor bool
	// Decidir responde pedidos de permissão do agente; nil aprova sempre.
	Decidir func(ctx context.Context, q Pergunta) (allow bool, msg string)
	// DecidirFim é como Decidir, mas uma negação com encerra=true termina a execução
	// (código CodigoNegado, motivo "negado") sem nova tentativa. Se definido, vence Decidir.
	DecidirFim func(ctx context.Context, q Pergunta) (allow bool, msg string, encerra bool)
	// Pai é o agente que lançou este; PaiLogs é a pasta de logs dele, onde este run se registra como
	// filho. O CLI preenche com OPENHEINERSS_PAI/OPENHEINERSS_PAI_LOGS (a biblioteca não lê o ambiente).
	Pai, PaiLogs string
	// EsperarFilhos limita a espera pelos agentes filhos antes de retomar o pai: 0 = padrão (2 h),
	// negativo = não espera nem retoma (comportamento antigo).
	EsperarFilhos time.Duration
	// RodadasFilhos é o máximo de retomadas automáticas do pai (padrão 5).
	RodadasFilhos int
	// IntervaloFilhos é o intervalo entre as leituras do meta.json dos filhos (padrão 2 s).
	IntervaloFilhos time.Duration
	// FilhosObrigatorios faz o pai terminar com CodigoFilhoFalhou quando algum filho terminou com
	// código ≠ 0, morreu sem FIM ou ainda roda no fim do pai (--filhos-obrigatorios).
	FilhosObrigatorios bool
	// LimiteContexto e AcaoContexto (--limite-contexto/--acao-contexto) vencem o config.yaml;
	// nil/vazio = não dados (LimiteContexto 0 desliga). Veja contexto.go.
	LimiteContexto *int
	AcaoContexto   string
	// ParadoAviso e ParadoParar (--parado-aviso/--parado-parar) vencem o config.yaml; nil = não dados
	// (0 desliga). Dar ParadoParar > 0 liga a ação "parar". Veja parado.go.
	ParadoAviso, ParadoParar *time.Duration
	// ParadoIntervalo é o intervalo entre as checagens do detector (padrão 60 s).
	ParadoIntervalo time.Duration
}

// CodigoNegado é o código de fim quando uma negação de permissão encerra a execução.
const CodigoNegado = 3

// CodigoFilhoFalhou é o código de fim do pai com --filhos-obrigatorios quando um filho falhou.
const CodigoFilhoFalhou = 4

// Motivos gravados no meta.json e no orq.fim.
const (
	MotivoNegado       = "negado"
	MotivoFilhoFalhou  = "filho falhou"
	MotivoFilhosOrfaos = "filhos órfãos"
)

// Tipos de Evento.
const (
	EvInicio    = "inicio"
	EvProgresso = "progresso"
	EvFim       = "fim"
	EvErro      = "erro"
	// EvFilhosOrfaos: o pai terminou com filhos ainda rodando (Filhos, Mensagem).
	EvFilhosOrfaos = "filhos_orfaos"
)

// Evento é um fato da execução do rodar, no vocabulário do protocolo orq.*.
type Evento struct {
	Tipo       string
	Agente     string
	Motor      string
	Modelo     string
	Tentativa  int
	Worktree   string
	Resumo     string
	Mensagem   string
	Cota       bool
	Codigo     int
	Tentativas int
	Duracao    time.Duration
	Relatorio  string
	Motivo     string // no EvFim: MotivoNegado, MotivoFilhoFalhou ou MotivoFilhosOrfaos
	Filhos     []string
}

// Pergunta é um permission_request que precisa de decisão.
type Pergunta struct {
	Agente   string
	Pergunta string
	Opcoes   []string
	Request  protocol.PermissionRequestParams
}

type Result struct {
	Name, WorkDir, LogFile, MetaFile string
	Attempts                         int
	Code                             int
	// Causa é a razão curta de um Code ≠ 0 (vazia no sucesso); o detalhe fica no log.
	Causa  string
	Resumo string
	// Relatorio é o RELATORIO-AGENTE.md do agente (nas missões, a cópia em relatorios/<nome>.md); vazio se não houver.
	Relatorio string
}

type meta struct {
	Projeto   string `json:"projeto"`
	Motor     string `json:"motor"`
	Modelo    string `json:"modelo"`
	Esforco   string `json:"esforco"`
	Conta     string `json:"conta"`
	Tentativa int    `json:"tentativa"`
	Inicio    string `json:"inicio"`
	PID       int    `json:"pid"`
	Servidor  bool   `json:"servidor,omitempty"`
	Fim       string `json:"fim,omitempty"`
	Codigo    *int   `json:"codigo,omitempty"`
	Motivo    string `json:"motivo,omitempty"`
	Pai       string `json:"pai,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	// PaiLogs é a pasta de logs do pai (para saber se o pai já terminou).
	PaiLogs string `json:"pai_logs,omitempty"`
	// FilhosFalhos e FilhosOrfaos são gravados no fim do pai.
	FilhosFalhos []string `json:"filhos_falhos,omitempty"`
	FilhosOrfaos []string `json:"filhos_orfaos,omitempty"`
	// UltimoEventoEm (RFC3339), Head, InicioPID e Checkpoints: ver orfao.go e checkpoint_git.go.
	UltimoEventoEm string       `json:"ultimo_evento_em,omitempty"`
	Head           string       `json:"head,omitempty"`
	InicioPID      string       `json:"inicio_pid,omitempty"`
	Checkpoints    []Checkpoint `json:"checkpoints,omitempty"`
	// ReiniciosContexto conta os recomeços em sessão nova por passar do limite de contexto.
	ReiniciosContexto int `json:"reinicios_contexto,omitempty"`
}

// nomeValido impede que o nome do agente saia da pasta de agentes (worktree, log e meta usam o nome em caminhos).
var nomeValido = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (o Options) emit(e Evento) {
	if o.OnEvent != nil {
		e.Agente = o.Name
		o.OnEvent(e)
	}
}

// resumoEvento devolve uma frase curta sobre o último texto ou ferramenta; texto acumula em buf.
func resumoEvento(ev harness.Event, buf *string) string {
	switch p := ev.Payload.(type) {
	case protocol.TextParams:
		*buf += p.Delta
		if len(*buf) > 2000 {
			*buf = (*buf)[len(*buf)-1000:]
		}
		return curto(*buf, 160)
	case protocol.ToolCallParams:
		*buf = ""
		in, _ := json.Marshal(p.Input)
		return curto("ferramenta "+p.Tool+" "+string(in), 160)
	case protocol.RawParams:
		return curto("raw "+p.Stream+": "+p.Line, 160)
	}
	return ""
}

// curto junta os espaços e devolve no máximo n runas do fim do texto.
func curto(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > n {
		return "…" + string(r[len(r)-n:])
	}
	return string(r)
}

func (o Options) defaults() Options {
	if o.BranchBase == "" {
		o.BranchBase = os.Getenv("BRANCH_BASE")
	}
	if o.MaxAgents <= 0 {
		o.MaxAgents = envInt("MAX_AGENTES", 4)
	}
	if o.Attempts <= 0 {
		o.Attempts = envInt("TENTATIVAS", 4)
	}
	if o.Model == "" {
		o.Model = os.Getenv("MODELO")
	}
	if o.Effort == "" {
		o.Effort = os.Getenv("ESFORCO")
	}
	if o.Conta == "" {
		o.Conta = os.Getenv("CONTA")
	}
	if !o.Seco {
		o.Seco = os.Getenv("RODAR_SECO") == "1"
	}
	if o.MaxLoad <= 0 {
		o.MaxLoad = envFloat("CARGA_MAXIMA", 0)
	}
	if o.Load == nil {
		o.Load = load1
	}
	if o.Sleep == nil {
		o.Sleep = time.Sleep
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.EventLog == "" {
		o.EventLog = os.Getenv("OPENHEINERSS_EVENTOS_LOG")
	}
	if o.Pai == o.Name {
		o.Pai = "" // nunca é filho de si mesmo (ex.: retomada manual de dentro da própria sessão)
	}
	if o.EsperarFilhos == 0 {
		if d, ok := esperaFilhosEnv(os.Getenv("OPENHEINERSS_ESPERAR_FILHOS")); ok {
			o.EsperarFilhos = d
		}
	}
	if o.EsperarFilhos == 0 {
		o.EsperarFilhos = esperaFilhosPadrao
	}
	if o.RodadasFilhos <= 0 {
		o.RodadasFilhos = envInt("OPENHEINERSS_RODADAS_FILHOS", rodadasFilhosPadrao)
	}
	if o.IntervaloFilhos <= 0 {
		o.IntervaloFilhos = intervaloFilhosPadrao
	}
	return o
}

func Run(ctx context.Context, cwd string, opts Options) (Result, error) {
	o := opts.defaults()
	if o.Name == "" || o.Motor == "" {
		return Result{}, fmt.Errorf("rodar exige nome e instância/harness")
	}
	if !nomeValido.MatchString(o.Name) {
		return Result{}, fmt.Errorf("nome de agente inválido %q: use letras, números, '.', '_' ou '-' (sem barras)", o.Name)
	}
	if len([]rune(o.Name)) > 80 {
		return Result{}, fmt.Errorf("nome de agente longo demais (%d caracteres; máximo 80)", len([]rune(o.Name)))
	}
	if o.MaxLoad == 0 {
		o.MaxLoad = envFloat("OPENHEINERSS_CARGA_MAXIMA", 0)
	}
	if o.QuotaMax <= 0 {
		o.QuotaMax = envFloat("OPENHEINERSS_COTA_MAX", envFloat("COTA_MAX", 0))
	}
	// A raiz é a do REPOSITÓRIO (git-common-dir), mesmo quando chamado de dentro de uma worktree de agente.
	repo, err := gitRoot(cwd)
	if err != nil {
		return Result{}, err
	}
	if o.EventLog == "" {
		if cfg, cfgErr := config.LoadProject(repo); cfgErr == nil {
			o.EventLog = cfg.EventosLog
		}
	}
	if cfg, cfgErr := config.LoadProject(repo); cfgErr == nil {
		if o.RegrasPadrao == "" {
			o.RegrasPadrao = cfg.RegrasPadrao
		}
		if cfg.SemRegrasPadrao {
			o.SemRegras = true
		}
	}
	if o.RegrasPadrao != "" && !filepath.IsAbs(o.RegrasPadrao) {
		o.RegrasPadrao = filepath.Join(repo, o.RegrasPadrao)
	}
	agents := o.AgentsDir
	if agents == "" {
		agents = os.Getenv("AGENTES")
	}
	if agents == "" {
		agents = ".claude/agentes"
	}
	agents, err = ValidateAgentsDir(repo, agents)
	if err != nil {
		return Result{}, err
	}
	// O prompt é lido antes: sem ele não vale criar worktree e branch que ninguém vai usar.
	prompt, err := readPromptOptions(agents, o.Name, o.PromptFile, o.PromptText, o.Regras, o.RegrasPadrao, o.SemRegras)
	if err != nil {
		return Result{}, err
	}
	// --seco não toca no disco: nada de pastas, travas ou meta.json.
	if o.Seco {
		fmt.Printf("SECO: %s\n", dryRunCommand(o))
		return Result{Name: o.Name, Code: 0}, nil
	}
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		return Result{}, err
	}
	lock, err := acquireNameLock(agents, o.Name)
	if err != nil {
		return Result{}, err
	}
	defer releaseNameLock(lock)
	if err := rejectLiveMeta(agents, o.Name); err != nil {
		return Result{}, err
	}
	cancelMarker := filepath.Join(agents, "logs", o.Name+".cancelado")
	if _, err := os.Stat(cancelMarker); err == nil {
		_ = os.Remove(cancelMarker)
		return Result{}, fmt.Errorf("agente %s foi cancelado antes de começar", o.Name)
	}
	start := o.Now()
	metaPath := filepath.Join(agents, "logs", o.Name+".meta.json")
	conta := o.Conta
	if conta == "" {
		conta = o.Motor
	}
	cur := meta{Projeto: filepath.Base(repo), Motor: o.Motor, Modelo: "padrão", Conta: conta, Tentativa: 1, Inicio: start.Format(time.RFC3339), PID: os.Getpid(), InicioPID: inicioProcesso(os.Getpid()), Servidor: o.ViaServidor, Pai: o.Pai}
	if o.Pai != "" && o.PaiLogs != "" {
		cur.PaiLogs = o.PaiLogs
	}
	// Respeita os limites antes de criar uma worktree (operação cara). Contar as vagas e registrar o
	// meta.json acontecem sob a mesma trava: assim o limite vale mesmo com agentes entrando juntos.
	if err := reserveSlot(ctx, agents, o, cur); err != nil {
		_ = os.Remove(cancelMarker)
		return Result{}, err
	}
	if o.Pai != "" && o.PaiLogs != "" {
		if err := registrarFilho(o.PaiLogs, o.Pai, o.Name, metaPath); err != nil {
			fmt.Fprintln(os.Stderr, "AVISO: não registrei o agente no pai "+o.Pai+": "+err.Error())
		}
	}
	logsDir := filepath.Join(agents, "logs")
	if !o.Retomar {
		_ = os.RemoveAll(filhosDir(logsDir, o.Name)) // filhos de uma execução antiga não contam
	}
	finalized := false
	defer func() {
		if !finalized { // saída inesperada: nunca deixa uma vaga ocupada para sempre
			code := 1
			cur.Fim, cur.Codigo = o.Now().Format(time.RFC3339), &code
			_ = writeMeta(metaPath, cur)
		}
	}()
	logPath := filepath.Join(agents, "logs", o.Name+".log")
	if !o.Retomar {
		if err := os.WriteFile(logPath, nil, 0644); err != nil {
			return Result{}, err
		}
	}
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return Result{}, err
	}
	// Quem acompanha o log (tail, agentes, servidor) lê pelo cache do sistema. Nada de fsync:
	// com o disco ocupado ele levava segundos por chamada e atrasava até o fim do processo.
	defer lf.Close()
	write := func(s string) { _, _ = lf.WriteString(s) }
	ultimoResumo := ""
	work := ""
	attempts := 0
	// finish fecha meta, log, evento e resultado; é o único caminho de saída depois da reserva da vaga.
	finish := func(code int, causa, motorName string, nl bool) Result {
		if code != 130 {
			// Interrompido já parou os filhos; nos demais fins, nada de FIM em silêncio com filho vivo ou falho.
			code, causa = balancoFilhos(o, logsDir, &cur, code, causa, write)
		}
		cur.Fim, cur.Codigo = o.Now().Format(time.RFC3339), &code
		_ = writeMeta(metaPath, cur)
		prefix := ""
		if nl {
			prefix = "\n"
		}
		write(fmt.Sprintf("%sFIM %s código %d\n", prefix, o.Now().Format("15:04"), code))
		if err := appendEventLog(o, filepath.Base(repo), o.Name, code, motorName); err != nil {
			aviso := "AVISO: não gravei o FIM no log de eventos: " + err.Error()
			write(aviso + "\n")
			fmt.Fprintln(os.Stderr, aviso)
		}
		rel := relatorio(work)
		if strings.HasPrefix(o.Name, "missao-") && work != "" {
			// A pasta da missão é descartável: o relatório precisa sair dela antes de ser apagada.
			rel = salvarRelatorio(work, agents, o.Name)
		}
		o.emit(Evento{Tipo: EvFim, Motor: motorName, Codigo: code, Tentativas: attempts, Duracao: o.Now().Sub(start), Relatorio: rel, Motivo: cur.Motivo, Filhos: filhosDoMotivo(cur)})
		finalized = true
		return Result{Name: o.Name, WorkDir: work, LogFile: logPath, MetaFile: metaPath, Attempts: attempts, Code: code, Causa: causa, Resumo: curto(ultimoResumo, 240), Relatorio: rel}
	}
	// abort registra no log e no FIM um erro de preparação: nada de sair mudo.
	abort := func(e error) (Result, error) {
		write("ERRO: " + e.Error() + "\n")
		o.emit(Evento{Tipo: EvErro, Motor: o.Motor, Mensagem: e.Error()})
		return finish(1, curto(e.Error(), 200), o.Motor, false), e
	}
	keys, err := loadKeys(o.KeysFile)
	if err != nil {
		return abort(err)
	}
	cx, err := novoCtxRun(repo, o)
	if err != nil {
		return abort(err)
	}
	work, cleanup, err := prepareWorktree(ctx, repo, agents, o.Name, o.BranchBase)
	if err != nil {
		return abort(err)
	}
	defer cleanup()
	cur.Worktree, cur.Branch = work, branchAtual(work)
	turnos := novoRegistroTurnos(work, o.Name, &cur, metaPath, o.Now)
	turnos.Marcar("base")
	estado := novoEstadoAgente(logsDir, o.Name, work, repo, o.BranchBase, prompt, o.Now)
	parado, err := iniciarParado(ctx, o, repo, work, logPath, logsDir, write)
	if err != nil {
		return abort(err)
	}
	defer parado.Fechar()
	// Quem for lançado de dentro do harness (`openheinerss rodar` filho) sabe quem é o pai e onde se registrar.
	if absLogs, err := filepath.Abs(logsDir); err == nil {
		env := make(map[string]string, len(keys)+2)
		for k, v := range keys {
			env[k] = v
		}
		env[EnvPai], env[EnvPaiLogs] = o.Name, absLogs
		keys = env
	}
	profile, err := motor.Resolve(o.Motor, o.Model, o.Effort)
	if err != nil {
		return abort(err)
	}
	candidates := []string{o.Motor}
	if spec, ok := harness.CustomSpecFor(o.Motor); ok {
		candidates = append(candidates, spec.Reserva...)
	}
	if len(candidates) > o.Attempts {
		candidates = candidates[:o.Attempts]
	}
	for len(candidates) < o.Attempts {
		candidates = append(candidates, candidates[len(candidates)-1])
	}
	var lastErr error
	finalCode := 1
	quotaSkipped, ran := false, false
	resumeID, resumeMotor := "", ""
	filhos := &rodadasFilhos{reportados: map[string]bool{}}
	turnoMsg := "" // mensagem de retomada depois que os agentes filhos terminam
	for i := 0; i < len(candidates); i++ {
		candidate := candidates[i]
		attempts = i + 1
		if o.QuotaMax > 0 && turnoMsg == "" && !cx.pendente {
			if percentual, ok := limites.Percentual(candidate); ok && percentual >= o.QuotaMax {
				quotaSkipped = true
				lastErr = fmt.Errorf("cota de %s em %.1f%% (limite %.1f%%)", candidate, percentual, o.QuotaMax)
				write(fmt.Sprintf("pulando %s: %v\n", candidate, lastErr))
				o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: lastErr.Error(), Cota: true})
				continue
			}
		}
		if err := waitLoad(ctx, o); err != nil {
			return finish(1, err.Error(), o.Motor, false), err
		}
		p := profile
		if candidate != o.Motor {
			// A reserva usa o modelo e o esforço da própria instância: o modelo pedido para o motor
			// principal (ex.: claude-sonnet-5-5) não vale em outro harness (o codex devolvia 400).
			p, err = motor.Resolve(candidate, "", "")
			if err != nil {
				lastErr = err
				continue
			}
		}
		modelName, effort := p.Model, p.Effort
		if modelName == "" {
			if s, ok := harness.CustomSpecFor(candidate); ok {
				modelName, effort = s.Model, s.Effort
			}
		}
		// "padrão" só aparece no meta.json; o harness recebe vazio e usa o próprio padrão.
		// No codex o padrão é conhecido: o meta mostra o modelo e o esforço que o `codex exec` vai usar de fato.
		modeloMeta, esforcoMeta := modelName, effort
		if baseHarness(candidate) == "codex" {
			if modeloMeta == "" {
				modeloMeta = codex.ModeloPadrao
			}
			if esforcoMeta == "" {
				esforcoMeta = codex.EsforcoPadrao
			}
		}
		if modeloMeta == "" {
			modeloMeta = "padrão"
		}
		cur.Motor, cur.Modelo, cur.Esforco, cur.Tentativa = candidate, modeloMeta, esforcoMeta, attempts
		if o.Conta == "" {
			cur.Conta = candidate
		}
		if err := writeMeta(metaPath, cur); err != nil {
			return finish(1, err.Error(), candidate, false), err
		}
		if err := os.WriteFile(filepath.Join(agents, "logs", o.Name+".modelo"), []byte(modeloMeta+"\n"), 0644); err != nil {
			return finish(1, err.Error(), candidate, false), err
		}
		ran = true
		if turnoMsg != "" {
			write(fmt.Sprintf("\n### retomada %d de %d (%s) motor %s: agentes filhos\n", filhos.feitas, o.RodadasFilhos, o.Now().Format("15:04"), candidate))
		} else {
			write(fmt.Sprintf("### tentativa %d (%s) motor %s\n", attempts, o.Now().Format("15:04"), candidate))
		}
		o.emit(Evento{Tipo: EvInicio, Motor: candidate, Modelo: modeloMeta, Tentativa: attempts, Worktree: work})
		mode := harness.ModeCLI
		if o.Mode != "" {
			mode = harness.Mode(o.Mode)
		} else if s, ok := harness.CustomSpecFor(candidate); ok && s.Mode != "" {
			mode = harness.Mode(s.Mode)
		}
		h, e := harness.Create(candidate, mode)
		if e != nil {
			lastErr = e
			write("ERRO: " + e.Error() + "\n")
			o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: e.Error()})
			continue
		}
		hctx, cancel := context.WithCancel(ctx)
		options := map[string]interface{}{"effort": effort}
		options["rodar"] = true
		if len(o.HarnessArgs) > 0 {
			options[harness.OptionHarnessArgs] = append([]string(nil), o.HarnessArgs...)
		}
		if resumeID != "" && resumeMotor == candidate {
			options["codex_session_id"] = resumeID
			options["claude_session_id"] = resumeID
		}
		cfg := harness.SessionConfig{SessionID: fmt.Sprintf("rodar-%s-%d", o.Name, attempts), CWD: work, Model: modelName, Options: options, Env: keys}
		envio, regras := separarRegras(prompt)
		if envio, e = entregarRegras(candidate, envio, regras, filepath.Join(agents, "logs", o.Name+".regras.md"), &cfg); e != nil {
			lastErr = e
			write("ERRO: " + e.Error() + "\n")
			cancel()
			continue
		}
		if e = h.Start(hctx, cfg); e == nil {
			text := envio
			if turnoMsg != "" {
				// Com sessão nativa a conversa continua e basta a mensagem; sem ela o motor precisa do prompt.
				if options["claude_session_id"] != nil {
					text = turnoMsg
				} else {
					text = envio + "\n\n" + turnoMsg
				}
				turnoMsg = ""
			} else if cx.pendente {
				text += cx.textoContinuacao(estado.Continuacao())
			} else if attempts > 1 || o.Retomar {
				text += estado.ContinuacaoOu(continuation)
			}
			if strings.TrimSpace(prompt) == "" {
				e = fmt.Errorf("prompt vazio recebido pelo motor %s", candidate)
			} else {
				e = h.SendPrompt(hctx, text, nil)
			}
		}
		if e != nil {
			lastErr = e
			write("ERRO: " + e.Error() + "\n")
			o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: e.Error()})
			_ = h.Stop()
			cancel()
			continue
		}
		var textBuf string
		quota := quotaPattern(candidate)
		failed, quotaHit, providerFailure, reiniciarCtx := false, false, false, false
		quotaNotice := ""
		providerNotice := ""
		providerRe := providerPattern(candidate)
		// Cota e sobrecarga só valem em canal de erro (evento de erro, stderr do motor, resultado com
		// is_error); o texto livre do agente nunca é examinado.
		errMsg, endReason := "", ""
		parado.Ativo(true)
		for {
			select {
			case msg := <-parado.Parar():
				// O detector já mandou SIGTERM ao grupo do motor; fecha como erro retomável.
				_ = h.Stop()
				cancel()
				turnos.Marcar("parado")
				pararFilhos(logsDir, o.Name, o.Now())
				cur.Motivo = MotivoParado
				return finish(CodigoParado, MotivoParado+": "+curto(msg, 160), candidate, true), nil
			case ev := <-h.Events():
				turnos.TocarEvento()
				line := eventText(ev)
				if line != "" {
					write(line)
				}
				if p, ok := ev.Payload.(protocol.ErrorParams); ok {
					errMsg = p.Message
				}
				if cx.observar(ev) && cx.aoPassar(o, filepath.Base(repo), candidate, write) {
					reiniciarCtx = true
					goto done
				}
				// Cota só em eventos de erro: o texto do agente pode falar de "cota" sem estar sem cota (achado real na etapa 6).
				if ev.Type == harness.EventError && quota != nil && quota.MatchString(line) {
					quotaHit = true
					if quotaNotice == "" {
						quotaNotice = quotaResetNotice(line)
					}
				}
				if ev.Type == harness.EventError && providerRe != nil && providerRe.MatchString(line) {
					providerFailure = true
					if providerNotice == "" {
						providerNotice = strings.TrimSpace(line)
					}
				}
				if ev.Type == harness.EventError && unknownOptionPattern.MatchString(line) {
					failed = true
					if errMsg == "" {
						errMsg = curto(line, 200)
					}
				}
				// Harness custom já detecta a cota pelo próprio regex e avisa com este erro.
				if e, ok := ev.Payload.(protocol.ErrorParams); ok && strings.HasPrefix(e.Message, "limite de cota detectado") {
					quotaHit = true
					if quotaNotice == "" {
						quotaNotice = quotaResetNotice(e.Message)
					}
				}
				if resumo := resumoEvento(ev, &textBuf); resumo != "" {
					ultimoResumo = resumo
					o.emit(Evento{Tipo: EvProgresso, Motor: candidate, Resumo: resumo})
				}
				if ev.Type == harness.EventPermission {
					if q, ok := ev.Payload.(protocol.PermissionRequestParams); ok {
						allow, msg, encerra := true, "", false
						pq := Pergunta{Agente: o.Name, Pergunta: perguntaDe(q), Opcoes: []string{"permitir", "negar"}, Request: q}
						if o.DecidirFim != nil {
							allow, msg, encerra = o.DecidirFim(ctx, pq)
						} else if o.Decidir != nil {
							allow, msg = o.Decidir(ctx, pq)
						}
						_ = h.RespondPermission(hctx, q.RequestID, allow, msg)
						if !allow && encerra && ctx.Err() == nil {
							// Negação que encerra: para o agente já, sem reserva nem nova tentativa.
							_ = h.Stop()
							cancel()
							write("permissão negada; execução encerrada\n")
							cur.Motivo = MotivoNegado
							return finish(CodigoNegado, "negado: "+curto(pq.Pergunta, 160), candidate, true), nil
						}
					}
				}
				// Avisos no stderr (rede, MCP) chegam como erro mas não derrubam a tarefa;
				// quem decide é o motivo do fim.
				if ev.Type == harness.EventComplete {
					failed = quotaHit || providerFailure
					if c, ok := ev.Payload.(protocol.CompleteParams); ok && falhaNoFim[c.Reason] {
						failed = true
						endReason = c.Reason
					}
					finalCode = 0
					if failed {
						finalCode = 1
					}
					goto done
				}
			case <-ctx.Done():
				_ = h.Stop()
				cancel()
				turnos.Marcar("interrompido")
				estado.FimDeTurno()
				pararFilhos(logsDir, o.Name, o.Now())
				// Interrompido (rodar.parar ou Ctrl-C): fecha meta e log para ninguém achar que ainda roda.
				return finish(130, "interrompido", candidate, true), ctx.Err()
			}
		}
	done:
		parado.Ativo(false)
		if resumable, ok := h.(interface{ ResumeID() string }); ok {
			if id := resumable.ResumeID(); id != "" {
				resumeID, resumeMotor = id, candidate
			}
		}
		_ = h.Stop()
		cancel()
		turnos.Marcar(fmt.Sprintf("tentativa %d", attempts))
		estado.FimDeTurno()
		if reiniciarCtx {
			// Sessão nova do MESMO motor: sem sessão nativa e sem contar como tentativa.
			resumeID, resumeMotor = "", ""
			cur.ReiniciosContexto = cx.reinicios
			i--
			continue
		}
		if !failed {
			msg, err := posTurno(ctx, o, logsDir, work, textBuf, filhos, write)
			if err != nil {
				pararFilhos(logsDir, o.Name, o.Now())
				return finish(130, "interrompido", candidate, true), err
			}
			if msg != "" {
				turnoMsg = msg
				i-- // mesma instância, mesma tentativa: retoma a sessão
				continue
			}
			return finish(finalCode, "", candidate, false), nil
		}
		switch {
		case quotaHit:
			lastErr = fmt.Errorf("execução interrompida por falta de cota")
			if quotaNotice != "" {
				lastErr = fmt.Errorf("execução interrompida por falta de cota (%s)", quotaNotice)
			}
		case providerFailure:
			lastErr = fmt.Errorf("erro do provedor: %s", curto(providerNotice, 160))
		case errMsg != "":
			lastErr = fmt.Errorf("execução interrompida: %s", curto(errMsg, 160))
		case endReason != "":
			lastErr = fmt.Errorf("execução interrompida (motivo %s)", endReason)
		default:
			lastErr = fmt.Errorf("execução interrompida")
		}
		if quotaHit && !hasReserva(candidate) {
			finalCode = 2
			if quotaNotice != "" {
				write("Falta de cota: " + quotaNotice + "\n")
			}
			break
		}
		finalCode = 1
		if providerFailure && providerNotice != "" && !hasReserva(candidate) {
			write("Erro do provedor: " + providerNotice + "\n")
		}
		if i+1 >= len(candidates) {
			write("saiu com erro; sem mais tentativas\n")
			o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: lastErr.Error(), Cota: quotaHit})
			break
		}
		if providerFailure && !quotaHit && !hasReserva(candidate) {
			// Erro transitório sem reserva: repete a MESMA instância, com espera curta e crescente.
			espera := backoff(attempts)
			write(fmt.Sprintf("erro do provedor; repetindo %s em %s (tentativa %d de %d)\n", candidate, espera, attempts+1, len(candidates)))
			o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: lastErr.Error()})
			if err := esperar(ctx, o, espera); err != nil {
				return finish(130, "interrompido", candidate, true), err
			}
			continue
		}
		write("saiu com erro; tentando continuar...\n")
		o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: lastErr.Error(), Cota: quotaHit})
	}
	if quotaSkipped && !ran {
		finalCode = 2 // todos os motores acima do limite de cota: mesmo código de "sem cota"
	}
	if lastErr == nil {
		if finalCode != 2 {
			lastErr = fmt.Errorf("nenhuma tentativa foi iniciada")
		}
	}
	code := finalCode
	if quotaHitAtEnd(lastErr) {
		code = 2
	}
	causa := ""
	if lastErr != nil {
		causa = curto(lastErr.Error(), 200)
	}
	result := finish(code, causa, cur.Motor, false)
	if code == 2 {
		return result, nil
	}
	return result, lastErr
}

func hasReserva(name string) bool {
	s, ok := harness.CustomSpecFor(name)
	return ok && len(s.Reserva) > 0
}

func quotaResetNotice(line string) string {
	line = strings.TrimSpace(line)
	if i := strings.Index(strings.ToLower(line), "resets"); i >= 0 {
		return strings.TrimSpace(line[i:])
	}
	if i := strings.Index(strings.ToLower(line), "reinicia"); i >= 0 {
		return strings.TrimSpace(line[i:])
	}
	return ""
}

func quotaHitAtEnd(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "falta de cota")
}

func relatorio(work string) string {
	if work == "" {
		return ""
	}
	p := filepath.Join(work, "RELATORIO-AGENTE.md")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func perguntaDe(q protocol.PermissionRequestParams) string {
	s := "Permitir a ferramenta " + q.Tool
	if q.Command != "" {
		s += ": " + q.Command
	}
	if q.Risk != "" {
		s += " (risco " + q.Risk + ")"
	}
	return s + "?"
}

func readPrompt(agents, name, explicit, text string) (string, error) {
	return readPromptOptions(agents, name, explicit, text, "", "", false)
}

func readPromptOptions(agents, name, explicit, text, rulesPath, defaultRulesPath string, noRules bool) (string, error) {
	b := []byte(text)
	if text == "" {
		prompt := explicit
		if prompt == "" {
			prompt = filepath.Join(agents, "prompts", name+".md")
		}
		var err error
		if b, err = os.ReadFile(prompt); err != nil {
			return "", fmt.Errorf("abrir prompt %s: %w", prompt, err)
		}
	}
	rules := rulesPath
	if rules == "" {
		rules = "_regras.md"
	}
	if strings.HasPrefix(name, "missao-") {
		if rulesPath == "" {
			rules = "_regras-missao.md"
		}
	}
	var prefixos []string
	if !noRules {
		defaultRules := defaultPromptRules
		if defaultRulesPath != "" {
			rb, err := os.ReadFile(defaultRulesPath)
			if err != nil && !filepath.IsAbs(defaultRulesPath) {
				rb, err = os.ReadFile(filepath.Join(agents, "..", "..", defaultRulesPath))
			}
			if err != nil {
				return "", fmt.Errorf("abrir regras padrão %s: %w", defaultRulesPath, err)
			}
			defaultRules = strings.TrimSpace(string(rb))
		}
		if defaultRules != "" {
			prefixos = append(prefixos, defaultRules)
		}
		rb, err := os.ReadFile(filepath.Join(agents, "prompts", rules))
		if err == nil {
			prefixos = append([]string{string(rb)}, prefixos...)
		}
	}
	missao := ""
	if strings.HasPrefix(name, "missao-") {
		missao = missionRules
	}
	texto := string(b)
	// Um /comando precisa ser o começo do prompt: as regras vão depois do separador e o
	// runner as entrega por outro canal (veja separarRegras).
	if _, _, ok := harness.SlashCommand(texto); ok && (len(prefixos) > 0 || missao != "") {
		return texto + separadorRegras + strings.TrimSpace(strings.Join(prefixos, "\n\n")+missao), nil
	}
	return strings.Join(append(prefixos, texto), "\n\n") + missao, nil
}

// separadorRegras separa um /comando das regras que o acompanham (só quando o prompt é um /comando).
const separadorRegras = "\n\n\x00regras-do-rodar\x00\n"

// separarRegras devolve o /comando e as regras que o readPromptOptions pôs depois dele.
func separarRegras(prompt string) (envio, regras string) {
	envio, regras, _ = strings.Cut(prompt, separadorRegras)
	return envio, regras
}

// entregarRegras manda as regras de um /comando por um canal que não tira o comando do começo
// do prompt: claude → --append-system-prompt; codex → -c developer_instructions; aider → --read
// com um arquivo em logs/; os demais recebem as regras depois do comando, no mesmo prompt.
func entregarRegras(motor, envio, regras, arquivo string, cfg *harness.SessionConfig) (string, error) {
	if regras == "" {
		return envio, nil
	}
	switch baseHarness(motor) {
	case "claude-code", "claude":
		cfg.SystemPrompt = regras
	case "codex":
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(regras) // string JSON também é string básica TOML
		cfg.Options["config"] = append(harness.OptionStrings(cfg.Options, "config"), "developer_instructions="+strings.TrimSpace(buf.String()))
	case "aider":
		if err := os.WriteFile(arquivo, []byte(regras+"\n"), 0o600); err != nil {
			return "", err
		}
		cfg.Options["read_files"] = append(harness.OpcaoLista(cfg.Options, "read_files", "read"), arquivo)
	default:
		return envio + "\n\n" + regras, nil
	}
	return envio, nil
}

func eventText(e harness.Event) string {
	b, _ := json.Marshal(e.Payload)
	if p, ok := e.Payload.(protocol.TextParams); ok {
		return p.Delta
	}
	if p, ok := e.Payload.(protocol.ErrorParams); ok {
		return "\nERRO: " + p.Message + "\n"
	}
	if e.Type == harness.EventComplete {
		return "\n[completo]\n"
	}
	if p, ok := e.Payload.(protocol.RawParams); ok {
		return fmt.Sprintf("\n[raw %s] %s\n", p.Stream, p.Line)
	}
	// O texto chega em pedaços sem quebra de linha; os outros eventos começam numa linha nova.
	return fmt.Sprintf("\n[%s] %s\n", e.Type, b)
}
func quotaPattern(name string) *regexp.Regexp {
	if s, ok := harness.CustomSpecFor(name); ok && s.QuotaRegex != "" {
		return regexp.MustCompile(s.QuotaRegex)
	}
	return regexp.MustCompile(`(?i)(SEM COTA|RESOURCE_EXHAUSTED|quota.*(exceeded|limit)|session limit|usage limit|hit your limit|rate limit|limite.*cota)`)
}

// providerPattern identifica erros transitórios ou respostas inválidas do provedor
// que às vezes chegam antes de um fim com reason=completed. A instância pode
// substituir este padrão com error_regex/erro_regex.
func providerPattern(name string) *regexp.Regexp {
	if s, ok := harness.CustomSpecFor(name); ok && s.ErrorRegex != "" {
		return regexp.MustCompile(s.ErrorRegex)
	}
	return regexp.MustCompile(`(?i)(upstream error|serviceunavailableerror|service temporarily overloaded|temporarily unavailable|too many requests|\b429\b|\b5\d\d\b|bad gateway|gateway timeout|internal server error|overloaded)`)
}

var unknownOptionPattern = regexp.MustCompile(`(?i)(unknown option|error:\s*unknown)`)

func rejectLiveMeta(agents, name string) error {
	path := filepath.Join(agents, "logs", name+".meta.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ler meta do agente %q: %w", name, err)
	}
	var m meta
	if json.Unmarshal(b, &m) != nil || m.Fim != "" || m.PID <= 0 {
		return nil
	}
	if processAlive(m.PID) {
		return fmt.Errorf("agente %q já está rodando (PID %d); use 'agentes parar %s'", name, m.PID, name)
	}
	return nil
}

func appendEventLog(o Options, projeto, nome string, codigo int, motor string) error {
	path := o.EventLog
	if path == "" {
		if spec, ok := harness.CustomSpecFor(motor); ok {
			path = spec.EventLog
		}
	}
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "[%s] FIM %s código %d\n", projeto, nome, codigo)
	return err
}
func writeMeta(path string, m meta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmpFile, err := os.CreateTemp(dir, base+".tmp-")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	defer os.Remove(tmp)
	if err = tmpFile.Chmod(0600); err == nil {
		_, err = tmpFile.Write(append(b, '\n'))
	}
	if closeErr := tmpFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadKeys lê o arquivo de chaves (KEY=valor) para o ambiente DESTA execução (SessionConfig.Env).
// Nada de os.Setenv: o processo é compartilhado e o segredo não pode vazar para outros agentes.
func loadKeys(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("abrir arquivo de chaves: %w", err)
	}
	config.ArquivoPrivado(path)
	values := make(map[string]string)
	for numero, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) {
			fmt.Fprintf(os.Stderr, "Aviso: linha inválida %d no arquivo de chaves (ignorada)\n", numero+1)
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return values, nil
}
func pause(ctx context.Context, o Options) error {
	done := make(chan struct{})
	go func() { o.Sleep(100 * time.Millisecond); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
func activeAgents(agents string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(agents, "logs"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		b, er := os.ReadFile(filepath.Join(agents, "logs", e.Name()))
		if er != nil {
			continue
		}
		var m meta
		if json.Unmarshal(b, &m) != nil || m.Fim != "" || m.PID <= 0 {
			continue
		}
		logName := strings.TrimSuffix(e.Name(), ".meta.json") + ".log"
		if !staleLog(filepath.Join(agents, "logs", logName), time.Now()) {
			if processAlivePlatform(m.PID) {
				n++
			}
		}
	}
	return n, nil
}
func load1() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if os.IsNotExist(err) {
		return 0, nil // sem /proc (macOS, Windows): o limite de carga não pode ser medido
	}
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, fmt.Errorf("carga vazia")
	}
	return strconv.ParseFloat(f[0], 64)
}
func envInt(k string, d int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return d
}
func envFloat(k string, d float64) float64 {
	if n, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil && n > 0 {
		return n
	}
	return d
}

// falhaNoFim lista os motivos de fim que contam como falha; os demais ("completed", "finished", "process_exit") são sucesso.
var falhaNoFim = map[string]bool{"process_error": true, "permission_denied": true, "error": true, "cancelled": true}
