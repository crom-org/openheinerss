package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/codex"
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

// ValidateAgentsDir resolve e garante que a pasta de agentes permaneça dentro
// da raiz do repositório. Isso evita que uma opção de linha de comando escape
// por caminho absoluto, .. ou um symlink já existente.
func ValidateAgentsDir(cwd, dir string) (string, error) {
	repo, err := RepoRoot(cwd)
	if err != nil {
		return "", err
	}
	resolved := ResolveAgentsDir(cwd, dir)
	repoAbs, _ := filepath.Abs(repo)
	pathAbs, _ := filepath.Abs(resolved)
	check, err := resolveExistingAncestor(pathAbs)
	if err != nil {
		return "", err
	}
	realRepo := repoAbs
	if real, e := filepath.EvalSymlinks(repoAbs); e == nil {
		realRepo = real
	}
	rel, err := filepath.Rel(realRepo, check)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("pasta de agentes fora do repositório: %s (raiz: %s)", pathAbs, repoAbs)
	}
	return pathAbs, nil
}

// resolveExistingAncestor resolve symlinks do ancestral existente mais próximo e reanexa os níveis
// que ainda não existem; assim um symlink acima de vários níveis inexistentes não passa batido.
func resolveExistingAncestor(p string) (string, error) {
	rest := ""
	cur := p
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest), nil
		}
		if _, err := os.Lstat(cur); err == nil {
			return "", fmt.Errorf("não foi possível resolver %s (symlink quebrado ou em laço)", cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, nil
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
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
	err = withFileLock(ctx, filepath.Join(agents, "logs", ".worktree.lock"), func() error {
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

// missionDir monta a pasta descartável de uma missão SEM escrita no Git do repositório: um clone raso
// (--shared, só lê os objetos do original) numa pasta temporária, com refs próprias. Um `git update-ref`,
// `branch` ou `commit` feito pela missão fica no clone e some com ele. Sem como clonar: pasta vazia.
// O RELATORIO-AGENTE.md é copiado para relatorios/<nome>.md antes de apagar (ver salvarRelatorio).
func missionDir(ctx context.Context, repo, agents, requestedBase string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "oh-missao-")
	if err != nil {
		return "", func() {}, err
	}
	remove := func() { _ = os.RemoveAll(dir) }
	base, err := resolveBase(repo, requestedBase)
	if err != nil {
		return dir, remove, nil // sem base: pasta vazia, como no rodar.sh
	}
	sha, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", base+"^{commit}").Output()
	if err != nil {
		return dir, remove, nil
	}
	if exec.CommandContext(ctx, "git", "clone", "--quiet", "--shared", "--no-checkout", repo, dir).Run() != nil {
		return dir, remove, nil
	}
	if exec.CommandContext(ctx, "git", "-C", dir, "checkout", "--quiet", "--detach", strings.TrimSpace(string(sha))).Run() != nil {
		_ = os.RemoveAll(dir)
		d2, e := os.MkdirTemp("", "oh-missao-")
		if e != nil {
			return "", func() {}, e
		}
		dir = d2
		return dir, func() { _ = os.RemoveAll(dir) }, nil
	}
	// A missão é descartável e nunca deve conseguir publicar no repositório
	// usado como origem pelo clone compartilhado.
	if err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "remove", "origin").Run(); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("remover remote origin da missão: %w", err)
	}
	return dir, remove, nil
}

// salvarRelatorio copia o RELATORIO-AGENTE.md de uma pasta descartável para <agentes>/relatorios/<nome>.md
// e devolve o caminho que sobrevive; vazio se a missão não escreveu relatório.
func salvarRelatorio(work, agents, name string) string {
	b, err := os.ReadFile(filepath.Join(work, "RELATORIO-AGENTE.md"))
	if err != nil {
		return ""
	}
	dest := filepath.Join(agents, "relatorios", name+".md")
	if os.MkdirAll(filepath.Dir(dest), 0755) != nil || os.WriteFile(dest, b, 0644) != nil {
		return ""
	}
	return dest
}

// withFileLock executa fn segurando um flock exclusivo no arquivo de trava. A espera é cancelável:
// tenta LOCK_NB a cada poucos milissegundos e desiste quando ctx é cancelado, mesmo com o detentor vivo.
func withFileLock(ctx context.Context, path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("esperando a trava %s: %w", filepath.Base(path), ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
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
		err := withFileLock(ctx, lock, func() error {
			if _, err := os.Stat(filepath.Join(agents, "logs", o.Name+".cancelado")); err == nil {
				return fmt.Errorf("agente %s foi cancelado antes de começar", o.Name)
			}
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

// baseHarness segue a cadeia de `base:` de uma instância até o harness base (ou o próprio nome).
func baseHarness(name string) string {
	for i := 0; i < 16; i++ {
		spec, ok := harness.CustomSpecFor(name)
		if !ok || spec.Command != "" || spec.Base == "" {
			return name
		}
		name = spec.Base
	}
	return name
}

// dryRunCommand monta o comando completo que o `rodar` executaria: binário, modelo efetivo e argumentos.
// Caminhos com espaço vão entre aspas (o texto pode ser colado num shell); valores de ambiente são mascarados.
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
			parts := []string{shQuote(spec.Command)}
			for _, a := range spec.Args {
				parts = append(parts, shQuote(a))
			}
			if model != "" {
				parts = append(parts, "--model", shQuote(model))
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
			model = codex.ModeloPadrao
		}
		if effort == "" {
			effort = codex.EsforcoPadrao
		}
		cmd = "codex exec --json -m " + shQuote(model) + " -c model_reasoning_effort=" + shQuote(effort) + " --dangerously-bypass-approvals-and-sandbox <prompt>"
	case "claude-code", "claude":
		cmd = "claude -p <prompt> --output-format stream-json --verbose"
		if model != "" {
			cmd += " --model " + shQuote(model)
		}
		if o.Mode == "sdk" || (isCustom && (spec.Mode == "sdk")) {
			cmd = "node <worker SDK do Claude Agent> (modo sdk)" + modelArg(model)
		}
	case "agy":
		cmd = "agy" + modelArg(model) + " --dangerously-skip-permissions --output-format stream-json -p <prompt>"
	case "opencode":
		cmd = "opencode run --format json" + modelArg(model) + " <prompt>"
	case "aider":
		cmd = "aider --yes-always --no-pretty --no-stream --no-check-update --no-analytics --no-show-model-warnings --no-browser --message <prompt>" + modelArg(model)
	default:
		cmd = shQuote(base) + modelArg(model) + " <prompt>"
	}
	return envPrefix(envSpec) + cmd + modeloEfetivo(model)
}

func modelArg(model string) string {
	if model == "" {
		return ""
	}
	return " --model " + shQuote(model)
}

func modeloEfetivo(model string) string {
	if model == "" {
		model = "<padrão>"
	}
	return "  [modelo efetivo: " + model + "]"
}

var shSeguro = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./~-]+$`)

// shQuote devolve s pronto para colar num shell: sem aspas se for seguro, senão entre aspas simples.
func shQuote(s string) string {
	if s != "" && shSeguro.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// envPublico lista as únicas variáveis cujo valor o --seco mostra (caminhos de configuração, nunca credenciais).
var envPublico = map[string]bool{"CODEX_HOME": true, "CLAUDE_CONFIG_DIR": true, "PWD": true, "HOME": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "OPENCODE_CONFIG_DIR": true, "AIDER_HOME": true}

// envPrefix mostra as variáveis da instância; só as de envPublico aparecem com valor, as demais viram ***
// (AUTHORIZATION, CREDENTIALS, cookies… não têm um padrão de nome que dê para adivinhar).
func envPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := "***"
		if envPublico[k] {
			v = shQuote(env[k])
		}
		b.WriteString(k + "=" + v + " ")
	}
	return b.String()
}
