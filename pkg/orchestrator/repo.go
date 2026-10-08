package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// RepoRoot (alias em inglês: RepoRoot) devolve a raiz do REPOSITÓRIO que contém cwd. Dentro de uma
// worktree (inclusive a de um agente) devolve a raiz do repositório principal, via
// `git rev-parse --git-common-dir`; assim pasta de agentes, logs e limites são sempre os mesmos.
func RepoRoot(cwd string) (string, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err == nil {
		common := strings.TrimSpace(string(out))
		if !filepath.IsAbs(common) {
			common = filepath.Join(cwd, common)
		}
		if filepath.Base(common) == ".git" {
			return filepath.Dir(common), nil
		}
	}
	// Repositório sem .git comum (submódulo, bare): cai na raiz da árvore de trabalho.
	out, err = exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("descobrir raiz git: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveAgentsDir resolve a pasta de agentes: caminho absoluto fica como está; relativo (ou vazio,
// que usa $AGENTES e depois .claude/agentes) é relativo à raiz do repositório, não ao cwd.
func ResolveAgentsDir(cwd, dir string) string {
	if dir == "" {
		dir = os.Getenv("AGENTES")
	}
	if dir == "" {
		dir = ".claude/agentes"
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	if repo, err := RepoRoot(cwd); err == nil {
		return filepath.Join(repo, dir)
	}
	abs, err := filepath.Abs(filepath.Join(cwd, dir))
	if err != nil {
		return dir
	}
	return abs
}

func gitRoot(cwd string) (string, error) { return RepoRoot(cwd) }

func gitOK(repo string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", repo}, args...)...).Run() == nil
}

// resolveBase escolhe a base da worktree: a pedida; senão `main` se existir; senão a branch atual (HEAD)
// da raiz do repositório. Repositório sem commits ou base inexistente é erro claro.
func resolveBase(repo, requested string) (string, error) {
	if requested != "" {
		if !gitOK(repo, "rev-parse", "--verify", "--quiet", requested+"^{commit}") {
			return "", fmt.Errorf("branch base %q não existe em %s", requested, repo)
		}
		return requested, nil
	}
	if gitOK(repo, "rev-parse", "--verify", "--quiet", "refs/heads/main^{commit}") {
		return "main", nil
	}
	if !gitOK(repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}") {
		return "", fmt.Errorf("repositório %s sem commits e sem branch main: faça um commit ou use --branch-base", repo)
	}
	if out, err := exec.Command("git", "-C", repo, "symbolic-ref", "--quiet", "--short", "HEAD").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name, nil
		}
	}
	return "HEAD", nil
}

// prepareWorktree devolve a pasta de trabalho do agente e a função que a limpa. Agentes comuns têm worktree
// permanente (agente/<nome>). Missões (missao-*) são somente leitura: rodam numa pasta descartável
// (worktree solta do repositório, ou pasta vazia se isso falhar) apagada no fim, e nada fica no repositório.
func prepareWorktree(ctx context.Context, repo, agents, name, requestedBase string) (string, func(), error) {
	if strings.HasPrefix(name, "missao-") {
		return missionDir(ctx, repo, agents, requestedBase)
	}
	target := filepath.Join(agents, name)
	if _, err := os.Stat(target); err == nil {
		return target, func() {}, nil
	}
	base, err := resolveBase(repo, requestedBase)
	if err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", func() {}, err
	}
	// Criar worktrees em paralelo no mesmo repositório disputa travas do git.
	err = withFileLock(filepath.Join(agents, "logs", ".worktree.lock"), func() error {
		out, err := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", target, "-b", "agente/"+name, base).CombinedOutput()
		if err != nil {
			return fmt.Errorf("criar worktree: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	})
	if err != nil {
		return "", func() {}, err
	}
	return target, func() {}, nil
}

func missionDir(ctx context.Context, repo, agents, requestedBase string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "oh-missao-")
	if err != nil {
		return "", func() {}, err
	}
	remove := func() { _ = os.RemoveAll(dir) }
	if base, err := resolveBase(repo, requestedBase); err == nil {
		err = withFileLock(filepath.Join(agents, "logs", ".worktree.lock"), func() error {
			return exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", "--detach", dir, base).Run()
		})
		if err == nil {
			return dir, func() {
				_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", dir).Run()
				remove()
				_ = exec.Command("git", "-C", repo, "worktree", "prune").Run()
			}, nil
		}
	}
	return dir, remove, nil // sem como montar a worktree: pasta vazia, como no rodar.sh
}

// withFileLock executa fn segurando um flock exclusivo (bloqueante) no arquivo de trava.
func withFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// reserveSlot espera haver carga e vaga e, dentro da mesma trava, registra o meta.json do agente:
// contar as vagas e ocupá-las é uma operação só, então dois agentes nunca passam juntos do limite.
func reserveSlot(ctx context.Context, agents string, o Options, m meta) error {
	lock := filepath.Join(agents, "logs", ".vagas.lock")
	metaPath := filepath.Join(agents, "logs", o.Name+".meta.json")
	for {
		if err := waitLoad(ctx, o); err != nil {
			return err
		}
		got := false
		err := withFileLock(lock, func() error {
			if o.MaxAgents > 0 {
				n, err := activeAgents(agents)
				if err != nil {
					return err
				}
				if n >= o.MaxAgents {
					return nil
				}
			}
			got = true
			return writeMeta(metaPath, m)
		})
		if err != nil {
			return err
		}
		if got {
			return nil
		}
		if err := pause(ctx, o); err != nil {
			return err
		}
	}
}

func waitLoad(ctx context.Context, o Options) error {
	for o.MaxLoad > 0 {
		l, err := o.Load()
		if err != nil {
			return err
		}
		if l <= o.MaxLoad {
			return nil
		}
		if err := pause(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

// esperar dorme d (via o.Sleep) respeitando o cancelamento.
func esperar(ctx context.Context, o Options, d time.Duration) error {
	done := make(chan struct{})
	go func() { o.Sleep(d); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// backoff é a espera crescente entre repetições da mesma instância: 2 s, 4 s, 6 s… até 10 s.
func backoff(tentativa int) time.Duration {
	d := time.Duration(tentativa) * 2 * time.Second
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

// providerStdoutPattern vale só para o texto final de um turno sem resultado: sem os números soltos
// (429/5xx) do padrão de eventos de erro, para não confundir uma resposta curta qualquer com falha.
func providerStdoutPattern(name string) *regexp.Regexp {
	if s, ok := harness.CustomSpecFor(name); ok && s.ErrorRegex != "" {
		return regexp.MustCompile(s.ErrorRegex)
	}
	return regexp.MustCompile(`(?i)(upstream error|serviceunavailableerror|service temporarily overloaded|temporarily unavailable|too many requests|bad gateway|gateway timeout|internal server error)`)
}

// textoSemResultado diz se o turno terminou sem trabalho útil: nenhuma ferramenta usada e texto final curto.
// Só nesse caso o padrão de cota/sobrecarga é aplicado ao texto (agentes falam de "cota" nos resumos).
const textoCurtoMax = 160

func textoSemResultado(ferramentas, tamanho int) bool {
	return ferramentas == 0 && tamanho > 0 && tamanho <= textoCurtoMax
}

var introMensagem = regexp.MustCompile(`(?i)^(?:(?:api\s+)?(?:error|erro|warning|aviso)\s*[:\-–—]?\s*|you(?:'ve|\s+have)?\s+(?:hit|reached|exceeded)\s+(?:your|the)\s+)+`)

// eMensagemDoProvedor diz se o texto final É a mensagem do provedor e não um agente falando sobre ela:
// curto, e o padrão aparece logo no início (depois de "Error:", "You've hit your"…).
func eMensagemDoProvedor(texto string, re *regexp.Regexp) bool {
	texto = strings.Join(strings.Fields(texto), " ")
	if re == nil || texto == "" || len([]rune(texto)) > textoCurtoMax || len(strings.Fields(texto)) > 20 {
		return false
	}
	resto := strings.TrimLeft(introMensagem.ReplaceAllString(texto, ""), " \"'`[(")
	loc := re.FindStringIndex(resto)
	return loc != nil && loc[0] == 0
}

// dryRunCommand monta o comando completo que o `rodar` executaria: binário, modelo efetivo e argumentos.
func dryRunCommand(o Options) string {
	spec, isCustom := harness.CustomSpecFor(o.Motor)
	model, effort := o.Model, o.Effort
	envSpec := map[string]string{}
	base := o.Motor
	for isCustom {
		if model == "" {
			model = spec.Model
		}
		if effort == "" {
			effort = spec.Effort
		}
		for k, v := range spec.Env {
			if _, ok := envSpec[k]; !ok {
				envSpec[k] = v
			}
		}
		if spec.Command != "" {
			parts := append([]string{spec.Command}, spec.Args...)
			if model != "" {
				parts = append(parts, "--model", model)
			}
			return envPrefix(envSpec) + strings.Join(parts, " ") + " <prompt>" + modeloEfetivo(model)
		}
		base = spec.Base
		next, ok := harness.CustomSpecFor(base)
		if !ok {
			break
		}
		spec = next
	}
	var cmd string
	switch base {
	case "codex":
		if model == "" {
			model = "gpt-reserve"
		}
		if effort == "" {
			effort = "medium"
		}
		cmd = "codex exec --json -m " + model + " -c model_reasoning_effort=" + effort + " --dangerously-bypass-approvals-and-sandbox <prompt>"
	case "claude-code", "claude":
		cmd = "claude -p <prompt> --output-format stream-json --verbose"
		if model != "" {
			cmd += " --model " + model
		}
		if o.Mode == "sdk" || (isCustom && (spec.Mode == "sdk")) {
			cmd = "node <worker SDK do Claude Agent> (modo sdk)" + modelArg(model)
		}
	case "agy":
		cmd = "agy" + modelArg(model) + " --dangerously-skip-permissions --output-format stream-json <prompt>"
	case "opencode":
		cmd = "opencode run --format json" + modelArg(model) + " <prompt>"
	case "aider":
		cmd = "aider --yes-always --no-pretty --no-stream --no-check-update --no-analytics --no-show-model-warnings --no-browser --message <prompt>" + modelArg(model)
	default:
		cmd = base + modelArg(model) + " <prompt>"
	}
	return envPrefix(envSpec) + cmd + modeloEfetivo(model)
}

func modelArg(model string) string {
	if model == "" {
		return ""
	}
	return " --model " + model
}

func modeloEfetivo(model string) string {
	if model == "" {
		model = "<padrão>"
	}
	return "  [modelo efetivo: " + model + "]"
}

// envPrefix mostra as variáveis da instância; valores de chaves/tokens nunca aparecem.
func envPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var b strings.Builder
	for _, k := range keys {
		v := env[k]
		u := strings.ToUpper(k)
		if strings.Contains(u, "KEY") || strings.Contains(u, "TOKEN") || strings.Contains(u, "SECRET") || strings.Contains(u, "PASSWORD") {
			v = "***"
		}
		b.WriteString(k + "=" + v + " ")
	}
	return b.String()
}
