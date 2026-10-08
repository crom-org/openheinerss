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
	"syscall"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

const continuation = "\n\n--- CONTINUAÇÃO ---\nUma execução anterior desta MESMA tarefa foi interrompida (erro ou cota). NÃO recomece do zero: rode `git status` e `git log --oneline -10`, leia RELATORIO-AGENTE.md e os arquivos já alterados nesta pasta (ou os relatórios em .claude/agentes/relatorios/ se for missão), confira o que já está pronto e termine SOMENTE o que falta, depois finalize como a tarefa pede."

type Options struct {
	Name, Motor, Model, Effort, PromptFile string
	Retomar                                bool
	AgentsDir, BranchBase                  string
	MaxLoad                                float64
	MaxAgents, Attempts                    int
	Load                                   func() (float64, error)
	Sleep                                  func(time.Duration)
	Now                                    func() time.Time
}

type Result struct {
	Name, WorkDir, LogFile, MetaFile string
	Attempts                         int
	Code                             int
}

type meta struct {
	Motor     string `json:"motor"`
	Modelo    string `json:"modelo"`
	Esforco   string `json:"esforco"`
	Conta     string `json:"conta"`
	Tentativa int    `json:"tentativa"`
	Inicio    string `json:"inicio"`
	PID       int    `json:"pid"`
	Fim       string `json:"fim,omitempty"`
	Codigo    *int   `json:"codigo,omitempty"`
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
	return o
}

func Run(ctx context.Context, cwd string, opts Options) (Result, error) {
	o := opts.defaults()
	if o.Name == "" || o.Motor == "" {
		return Result{}, fmt.Errorf("rodar exige nome e instância/harness")
	}
	if o.MaxLoad == 0 {
		o.MaxLoad = envFloat("OPENHEINERSS_CARGA_MAXIMA", 0)
	}
	repo, err := gitRoot(cwd)
	if err != nil {
		return Result{}, err
	}
	agents := o.AgentsDir
	if !filepath.IsAbs(agents) {
		agents = filepath.Join(repo, agents)
	}
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		return Result{}, err
	}
	work, err := prepareWorktree(ctx, repo, agents, o.Name, o.BranchBase)
	if err != nil {
		return Result{}, err
	}
	prompt, err := readPrompt(agents, o.Name, o.PromptFile)
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
	defer lf.Close()
	write := func(s string) { _, _ = lf.WriteString(s); _ = lf.Sync() }

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
	resumeID, resumeMotor := "", ""
	for i, candidate := range candidates {
		attempts = i + 1
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
		if modelName == "" {
			modelName = "padrão"
		}
		m := meta{Motor: candidate, Modelo: modelName, Esforco: effort, Conta: candidate, Tentativa: attempts, Inicio: start.Format(time.RFC3339), PID: os.Getpid()}
		metaPath := filepath.Join(agents, "logs", o.Name+".meta.json")
		if err := writeMeta(metaPath, m); err != nil {
			return Result{}, err
		}
		write(fmt.Sprintf("### tentativa %d (%s) motor %s\n", attempts, o.Now().Format("15:04"), candidate))
		h, e := harness.Create(candidate, harness.ModeCLI)
		if e != nil {
			lastErr = e
			write("ERRO: " + e.Error() + "\n")
			continue
		}
		hctx, cancel := context.WithCancel(ctx)
		options := map[string]interface{}{"effort": effort}
		if resumeID != "" && resumeMotor == candidate {
			options["codex_session_id"] = resumeID
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
			_ = h.Stop()
			cancel()
			continue
		}
		quota := quotaPattern(candidate)
		if s, ok := harness.CustomSpecFor(candidate); ok && s.QuotaRegex != "" {
			quota = regexp.MustCompile(s.QuotaRegex)
		}
		failed, quotaHit := false, false
		for {
			select {
			case ev := <-h.Events():
				line := eventText(ev)
				if line != "" {
					write(line)
					if quota != nil && quota.MatchString(line) {
						quotaHit = true
						failed = true
					}
				}
				if ev.Type == harness.EventPermission {
					if q, ok := ev.Payload.(protocol.PermissionRequestParams); ok {
						_ = h.RespondPermission(hctx, q.RequestID, true, "")
					}
				}
				if ev.Type == harness.EventError {
					failed = true
				}
				if ev.Type == harness.EventComplete {
					finalCode = 0
					if failed {
						finalCode = 1
					}
					goto done
				}
			case <-ctx.Done():
				_ = h.Stop()
				cancel()
				return Result{}, ctx.Err()
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
			return Result{o.Name, work, logPath, metaPath, attempts, finalCode}, nil
		}
		lastErr = fmt.Errorf("execução interrompida%s", map[bool]string{true: " por falta de cota", false: ""}[quotaHit])
		write("saiu com erro; tentando continuar...\n")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("nenhuma tentativa executada")
	}
	m := meta{Motor: o.Motor, Conta: o.Motor, Tentativa: attempts, Inicio: start.Format(time.RFC3339), PID: os.Getpid()}
	code := finalCode
	m.Fim, m.Codigo = o.Now().Format(time.RFC3339), &code
	_ = writeMeta(filepath.Join(agents, "logs", o.Name+".meta.json"), m)
	write(fmt.Sprintf("FIM %s código %d\n", o.Now().Format("15:04"), code))
	return Result{o.Name, work, logPath, filepath.Join(agents, "logs", o.Name+".meta.json"), attempts, code}, lastErr
}

func prepareWorktree(ctx context.Context, repo, agents, name, base string) (string, error) {
	target := filepath.Join(agents, name)
	if strings.HasPrefix(name, "missao-") {
		return target, os.MkdirAll(target, 0755)
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
func readPrompt(agents, name, explicit string) (string, error) {
	prompt := explicit
	if prompt == "" {
		prompt = filepath.Join(agents, "prompts", name+".md")
	}
	b, err := os.ReadFile(prompt)
	if err != nil {
		return "", fmt.Errorf("abrir prompt %s: %w", prompt, err)
	}
	rules := "_regras.md"
	if strings.HasPrefix(name, "missao-") {
		rules = "_regras-missao.md"
	}
	rb, err := os.ReadFile(filepath.Join(agents, "prompts", rules))
	if err == nil {
		return string(rb) + "\n\n" + string(b), nil
	}
	return string(b), nil
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
	return regexp.MustCompile(`(?i)(SEM COTA|RESOURCE_EXHAUSTED|quota.*(exceeded|limit)|rate limit|limite.*cota)`)
}
func writeMeta(path string, m meta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
		if p, er := os.FindProcess(m.PID); er == nil && p.Signal(syscall.Signal(0)) == nil {
			n++
		}
	}
	return n, nil
}
func load1() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
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
