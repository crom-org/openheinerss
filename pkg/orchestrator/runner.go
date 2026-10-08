// Package orchestrator implementa a execução de uma missão no estilo do
// rodar.sh, sem embutir contas ou provedores no binário.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

const continuation = "\n\n--- CONTINUAÇÃO ---\nUma execução anterior desta MESMA tarefa foi interrompida (erro ou cota). NÃO recomece do zero: rode `git status` e `git log --oneline -10`, leia RELATORIO-AGENTE.md e os arquivos já alterados nesta pasta (ou os relatórios em .claude/agentes/relatorios/ se for missão), confira o que já está pronto e termine SOMENTE o que falta, depois finalize como a tarefa pede."
const missionRules = "\n\n--- MISSÃO SOMENTE LEITURA ---\nEsta é uma missão de inspeção. Não crie, altere, remova ou comite arquivos; não use worktree. Apenas leia e relate o que encontrar."

type Options struct {
	Name, Motor, Model, Effort, PromptFile string
	// PromptText é o prompt em texto; quando preenchido, vale no lugar de PromptFile e de prompts/<nome>.md.
	PromptText            string
	Conta, Regras         string
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
	if o.AgentsDir == "" {
		o.AgentsDir = os.Getenv("AGENTES")
	}
	if o.AgentsDir == "" {
		o.AgentsDir = ".claude/agentes"
	}
	if o.BranchBase == "" {
		o.BranchBase = os.Getenv("BRANCH_BASE")
	}
	if o.BranchBase == "" {
		o.BranchBase = "main"
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
	if o.MaxLoad == 0 {
		o.MaxLoad = envFloat("OPENHEINERSS_CARGA_MAXIMA", 0)
	}
	if o.QuotaMax <= 0 {
		o.QuotaMax = envFloat("OPENHEINERSS_COTA_MAX", envFloat("COTA_MAX", 0))
	}
	repo, err := gitRoot(cwd)
	if err != nil {
		return Result{}, err
	}
	if o.EventLog == "" {
		if cfg, cfgErr := config.LoadProject(repo); cfgErr == nil {
			o.EventLog = cfg.EventosLog
		}
	}
	agents := o.AgentsDir
	if !filepath.IsAbs(agents) {
		agents = filepath.Join(repo, agents)
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
	// Respeita os limites antes de criar uma worktree, que pode ser uma
	// operação cara e não deve começar enquanto outro agente ocupa a vaga.
	if err := waitLimits(ctx, agents, o); err != nil {
		return Result{}, err
	}
	// O prompt é lido antes: sem ele não vale criar worktree e branch que ninguém vai usar.
	prompt, err := readPromptOptions(agents, o.Name, o.PromptFile, o.PromptText, o.Regras, o.SemRegras)
	if err != nil {
		return Result{}, err
	}
	if o.Seco {
		fmt.Printf("SECO: %s\n", dryRunCommand(o))
		return Result{Name: o.Name, Code: 0}, nil
	}
	restoreKeys, err := loadKeys(o.KeysFile)
	if err != nil {
		return Result{}, err
	}
	defer restoreKeys()
	work, err := prepareWorktree(ctx, repo, agents, o.Name, o.BranchBase)
	if err != nil {
		return Result{}, err
	}
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

	profile, err := motor.Resolve(o.Motor, o.Model, o.Effort)
	if err != nil {
		return Result{}, err
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
	start := o.Now()
	var lastErr error
	finalCode := 1
	attempts := 0
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
		if err := waitLimits(ctx, agents, o); err != nil {
			return Result{}, err
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
		modeloMeta := modelName
		if modeloMeta == "" {
			modeloMeta = "padrão"
		}
		conta := o.Conta
		if conta == "" {
			conta = candidate
		}
		m := meta{Projeto: filepath.Base(repo), Motor: candidate, Modelo: modeloMeta, Esforco: effort, Conta: conta, Tentativa: attempts, Inicio: start.Format(time.RFC3339), PID: os.Getpid(), Servidor: o.ViaServidor}
		metaPath := filepath.Join(agents, "logs", o.Name+".meta.json")
		if err := writeMeta(metaPath, m); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(filepath.Join(agents, "logs", o.Name+".modelo"), []byte(modeloMeta+"\n"), 0644); err != nil {
			return Result{}, err
		}
		ran = true
		write(fmt.Sprintf("### tentativa %d (%s) motor %s\n", attempts, o.Now().Format("15:04"), candidate))
		o.emit(Evento{Tipo: EvInicio, Motor: candidate, Modelo: modeloMeta, Tentativa: attempts, Worktree: work})
		h, e := harness.Create(candidate, harness.ModeCLI)
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
		cfg := harness.SessionConfig{SessionID: fmt.Sprintf("rodar-%s-%d", o.Name, attempts), CWD: work, Model: modelName, Options: options}
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
		if s, ok := harness.CustomSpecFor(candidate); ok && s.QuotaRegex != "" {
			quota = regexp.MustCompile(s.QuotaRegex)
		}
		failed, quotaHit := false, false
		quotaNotice := ""
		for {
			select {
			case ev := <-h.Events():
				line := eventText(ev)
				if line != "" {
					write(line)
				}
				// Cota só em eventos de erro: o texto do agente pode falar de "cota" sem estar sem cota (achado real na etapa 6).
				if ev.Type == harness.EventError && quota != nil && quota.MatchString(line) {
					quotaHit = true
					if quotaNotice == "" {
						quotaNotice = quotaResetNotice(line)
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
					failed = quotaHit
					if c, ok := ev.Payload.(protocol.CompleteParams); ok && falhaNoFim[c.Reason] {
						failed = true
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
				parado := 130
				m.Fim, m.Codigo = o.Now().Format(time.RFC3339), &parado
				_ = writeMeta(metaPath, m)
				write(fmt.Sprintf("\nFIM %s código %d\n", o.Now().Format("15:04"), parado))
				appendEventLog(o, filepath.Base(repo), o.Name, parado, candidate)
				o.emit(Evento{Tipo: EvFim, Motor: candidate, Codigo: parado, Tentativas: attempts, Duracao: o.Now().Sub(start), Relatorio: relatorio(work)})
				return Result{o.Name, work, logPath, metaPath, attempts, parado}, ctx.Err()
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
			m.Fim, m.Codigo = o.Now().Format(time.RFC3339), &finalCode
			_ = writeMeta(metaPath, m)
			write(fmt.Sprintf("FIM %s código %d\n", o.Now().Format("15:04"), finalCode))
			appendEventLog(o, filepath.Base(repo), o.Name, finalCode, candidate)
			o.emit(Evento{Tipo: EvFim, Motor: candidate, Codigo: finalCode, Tentativas: attempts, Duracao: o.Now().Sub(start), Relatorio: relatorio(work)})
			return Result{o.Name, work, logPath, metaPath, attempts, finalCode}, nil
		}
		if quotaHit && !hasReserva(candidate) {
			finalCode = 2
			if quotaNotice != "" {
				write("Falta de cota: " + quotaNotice + "\n")
			}
			break
		}
		lastErr = fmt.Errorf("execução interrompida%s", map[bool]string{true: " por falta de cota", false: ""}[quotaHit])
		write("saiu com erro; tentando continuar...\n")
		o.emit(Evento{Tipo: EvErro, Motor: candidate, Mensagem: lastErr.Error(), Cota: quotaHit})
	}
	if quotaSkipped && !ran {
		finalCode = 2 // todos os motores acima do limite de cota: mesmo código de "sem cota"
	}
	if lastErr == nil {
		if finalCode != 2 {
			lastErr = fmt.Errorf("nenhuma tentativa executada")
		}
	}
	conta := o.Conta
	if conta == "" {
		conta = o.Motor
	}
	m := meta{Projeto: filepath.Base(repo), Motor: o.Motor, Conta: conta, Tentativa: attempts, Inicio: start.Format(time.RFC3339), PID: os.Getpid(), Servidor: o.ViaServidor}
	code := finalCode
	if quotaHitAtEnd(lastErr) {
		code = 2
	}
	m.Fim, m.Codigo = o.Now().Format(time.RFC3339), &code
	_ = writeMeta(filepath.Join(agents, "logs", o.Name+".meta.json"), m)
	write(fmt.Sprintf("FIM %s código %d\n", o.Now().Format("15:04"), code))
	appendEventLog(o, filepath.Base(repo), o.Name, code, o.Motor)
	o.emit(Evento{Tipo: EvFim, Motor: o.Motor, Codigo: code, Tentativas: attempts, Duracao: o.Now().Sub(start), Relatorio: relatorio(work)})
	result := Result{o.Name, work, logPath, filepath.Join(agents, "logs", o.Name+".meta.json"), attempts, code}
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

func prepareWorktree(ctx context.Context, repo, agents, name, base string) (string, error) {
	target := filepath.Join(agents, name)
	if strings.HasPrefix(name, "missao-") {
		// Missões são somente leitura e os prompts esperam enxergar o projeto inteiro.
		return repo, nil
	}
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", target, "-b", "agente/"+name, base)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("criar worktree: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return target, nil
}
func gitRoot(cwd string) (string, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("descobrir raiz git: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
func readPrompt(agents, name, explicit, text string) (string, error) {
	return readPromptOptions(agents, name, explicit, text, "", false)
}

func readPromptOptions(agents, name, explicit, text, rulesPath string, noRules bool) (string, error) {
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

func appendEventLog(o Options, projeto, nome string, codigo int, motor string) {
	path := o.EventLog
	if path == "" {
		if spec, ok := harness.CustomSpecFor(motor); ok {
			path = spec.EventLog
		}
	}
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "[%s] FIM %s código %d\n", projeto, nome, codigo)
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

func dryRunCommand(o Options) string {
	model := o.Model
	if model == "" {
		model = "<padrão>"
	}
	commands := map[string]string{
		"codex":    "codex exec",
		"codex2":   "CODEX_HOME=~/.codex-compartilhado codex exec",
		"claude":   "claude -p",
		"agy":      "agy -p",
		"opencode": "opencode run",
		"aider":    "aider --message",
	}
	command := commands[o.Motor]
	if spec, ok := harness.CustomSpecFor(o.Motor); ok && spec.Command != "" {
		command = spec.Command + " " + strings.Join(spec.Args, " ")
	}
	if command == "" {
		command = o.Motor
	}
	return command + " --model " + model + " <prompt>"
}

var keysMu sync.Mutex

func loadKeys(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("abrir arquivo de chaves: %w", err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "export ") && strings.TrimSpace(strings.TrimPrefix(line, "export ")) == "" {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		values[key] = value
	}
	keysMu.Lock()
	previous := make(map[string]*string, len(values))
	for key, value := range values {
		if old, ok := os.LookupEnv(key); ok {
			copy := old
			previous[key] = &copy
		} else {
			previous[key] = nil
		}
		_ = os.Setenv(key, value)
	}
	return func() {
		for key, old := range previous {
			if old == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *old)
			}
		}
		keysMu.Unlock()
	}, nil
}
func waitLimits(ctx context.Context, agents string, o Options) error {
	for {
		if o.MaxAgents > 0 {
			n, err := activeAgents(agents)
			if err != nil {
				return err
			}
			if n >= o.MaxAgents {
				if err := pause(ctx, o); err != nil {
					return err
				}
				continue
			}
		}
		if o.MaxLoad > 0 {
			l, err := o.Load()
			if err != nil {
				return err
			}
			if l > o.MaxLoad {
				if err := pause(ctx, o); err != nil {
					return err
				}
				continue
			}
		}
		return nil
	}
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
