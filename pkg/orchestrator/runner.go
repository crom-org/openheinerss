// Package orchestrator implementa a execução de uma missão no estilo do
// rodar.sh, sem embutir contas ou provedores no binário.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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
const defaultPromptRules = "--- REGRAS PADRÃO DO AGENTE ---\nTrabalhe somente dentro da pasta do agente e da worktree desta missão.\nÉ proibido buscar fora da worktree: não use `find /`, `find ~`, `locate`, varreduras de disco ou buscas equivalentes fora dela."

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
	Load                  func() (float64, error)
	Sleep                 func(time.Duration)
	Now                   func() time.Time
	// OnEvent recebe os eventos de orquestração (opcional; o CLI não usa).
	OnEvent func(Evento)
	// ViaServidor marca execuções lançadas por `serve`: o PID do meta.json é o do servidor.
	ViaServidor bool
	// Decidir responde pedidos de permissão do agente; nil aprova sempre.
	Decidir func(ctx context.Context, q Pergunta) (allow bool, msg string)
}

// Tipos de Evento.
const (
	EvInicio    = "inicio"
	EvProgresso = "progresso"
	EvFim       = "fim"
	EvErro      = "erro"
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
	cur := meta{Projeto: filepath.Base(repo), Motor: o.Motor, Modelo: "padrão", Conta: conta, Tentativa: 1, Inicio: start.Format(time.RFC3339), PID: os.Getpid(), Servidor: o.ViaServidor}
	// Respeita os limites antes de criar uma worktree (operação cara). Contar as vagas e registrar o
	// meta.json acontecem sob a mesma trava: assim o limite vale mesmo com agentes entrando juntos.
	if err := reserveSlot(ctx, agents, o, cur); err != nil {
		_ = os.Remove(cancelMarker)
		return Result{}, err
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
		o.emit(Evento{Tipo: EvFim, Motor: motorName, Codigo: code, Tentativas: attempts, Duracao: o.Now().Sub(start), Relatorio: rel})
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
	work, cleanup, err := prepareWorktree(ctx, repo, agents, o.Name, o.BranchBase)
	if err != nil {
		return abort(err)
	}
	defer cleanup()
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
	for i, candidate := range candidates {
		attempts = i + 1
		if o.QuotaMax > 0 {
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
			p, err = motor.Resolve(candidate, o.Model, o.Effort)
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
		write(fmt.Sprintf("### tentativa %d (%s) motor %s\n", attempts, o.Now().Format("15:04"), candidate))
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
		if resumeID != "" && resumeMotor == candidate {
			options["codex_session_id"] = resumeID
			options["claude_session_id"] = resumeID
		}
		cfg := harness.SessionConfig{SessionID: fmt.Sprintf("rodar-%s-%d", o.Name, attempts), CWD: work, Model: modelName, Options: options, Env: keys}
		if e = h.Start(hctx, cfg); e == nil {
			text := prompt
			if attempts > 1 || o.Retomar {
				text += continuation
			}
			e = h.SendPrompt(hctx, text, nil)
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
		failed, quotaHit, providerFailure := false, false, false
		quotaNotice := ""
		providerNotice := ""
		providerRe := providerPattern(candidate)
		// Cota e sobrecarga só valem em canal de erro (evento de erro, stderr do motor, resultado com
		// is_error); o texto livre do agente nunca é examinado.
		errMsg, endReason := "", ""
		for {
			select {
			case ev := <-h.Events():
				line := eventText(ev)
				if line != "" {
					write(line)
				}
				if p, ok := ev.Payload.(protocol.ErrorParams); ok {
					errMsg = p.Message
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
						allow, msg := true, ""
						if o.Decidir != nil {
							allow, msg = o.Decidir(ctx, Pergunta{Agente: o.Name, Pergunta: perguntaDe(q), Opcoes: []string{"permitir", "negar"}, Request: q})
						}
						_ = h.RespondPermission(hctx, q.RequestID, allow, msg)
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
				// Interrompido (rodar.parar ou Ctrl-C): fecha meta e log para ninguém achar que ainda roda.
				return finish(130, "interrompido", candidate, true), ctx.Err()
			}
		}
	done:
		if resumable, ok := h.(interface{ ResumeID() string }); ok {
			if id := resumable.ResumeID(); id != "" {
				resumeID, resumeMotor = id, candidate
			}
		}
		_ = h.Stop()
		cancel()
		if !failed {
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
	result := string(b)
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
			result = defaultRules + "\n\n" + result
		}
		rb, err := os.ReadFile(filepath.Join(agents, "prompts", rules))
		if err == nil {
			result = string(rb) + "\n\n" + result
		}
	}
	if strings.HasPrefix(name, "missao-") {
		result += missionRules
	}
	return result, nil
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

func acquireNameLock(agents, name string) (*os.File, error) {
	path := filepath.Join(agents, "logs", name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("criar trava do agente %q: %w", name, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("agente %q já está rodando (trava de nome ocupada)", name)
	}
	return f, nil
}

func releaseNameLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

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
			if p, er := os.FindProcess(m.PID); er == nil && p.Signal(syscall.Signal(0)) == nil {
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
