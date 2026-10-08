package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

const fimOK = "printf '%s\\n' '{\"type\":\"end\"}'\n"

func scriptTeste(t *testing.T, dir, nome, corpo string) string {
	t.Helper()
	p := filepath.Join(dir, nome+".sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nread p\n"+corpo), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func registrar(t *testing.T, spec harness.CustomSpec) {
	t.Helper()
	if err := harness.RegisterCustom(spec); err != nil {
		t.Fatal(err)
	}
}

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ---- item 1: raiz do repositório, não da worktree ----

func TestRodarDeDentroDeWorktreeDeAgenteUsaPastaDoRepositorio(t *testing.T) {
	root, agents := repoFixture(t)
	if _, err := Run(context.Background(), root, Options{Name: "primeiro", Motor: "mock", PromptText: "ok", MaxAgents: 9}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(agents, "primeiro")
	got, err := RepoRoot(wt)
	if err != nil || real(t, got) != real(t, root) {
		t.Fatalf("RepoRoot(worktree) = %q (%v), esperado %q", got, err, root)
	}
	res, err := Run(context.Background(), wt, Options{Name: "segundo", Motor: "mock", PromptText: "ok", MaxAgents: 9})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("criou .claude dentro da worktree do agente: %v", err)
	}
	if real(t, filepath.Dir(res.WorkDir)) != real(t, agents) {
		t.Fatalf("worktree do segundo em %s; esperado dentro de %s", res.WorkDir, agents)
	}
	if real(t, filepath.Dir(res.LogFile)) != real(t, filepath.Join(agents, "logs")) {
		t.Fatalf("log em %s", res.LogFile)
	}
	if !strings.Contains(mustRead(t, res.MetaFile), `"projeto":"`+filepath.Base(root)+`"`) {
		t.Fatalf("projeto do meta deveria ser %q: %s", filepath.Base(root), mustRead(t, res.MetaFile))
	}
	if d := ResolveAgentsDir(wt, ""); real(t, d) != real(t, agents) {
		t.Fatalf("ResolveAgentsDir de dentro da worktree = %s", d)
	}
}

// ---- item 2: corrida na fila ----

func TestReservarVagaNuncaPassaDoLimite(t *testing.T) {
	agents := filepath.Join(t.TempDir(), "agentes")
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	var ativos, maximo int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			nome := "a" + string(rune('a'+i))
			o := Options{Name: nome, MaxAgents: 3, Sleep: func(time.Duration) { time.Sleep(2 * time.Millisecond) }}
			m := meta{PID: os.Getpid(), Inicio: time.Now().Format(time.RFC3339)}
			if err := reserveSlot(context.Background(), agents, o, m); err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&ativos, 1)
			for {
				old := atomic.LoadInt32(&maximo)
				if n <= old || atomic.CompareAndSwapInt32(&maximo, old, n) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond)
			atomic.AddInt32(&ativos, -1)
			code := 0
			m.Fim, m.Codigo = time.Now().Format(time.RFC3339), &code
			_ = writeMeta(filepath.Join(agents, "logs", nome+".meta.json"), m)
		}(i)
	}
	wg.Wait()
	if maximo > 3 || maximo == 0 {
		t.Fatalf("máximo simultâneo %d (limite 3)", maximo)
	}
	t.Logf("máximo simultâneo medido: %d", maximo)
}

func TestFilaDoRodarComParadaNuncaPassaDeDois(t *testing.T) {
	root, agents := repoFixture(t)
	contagem := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(contagem, 0755); err != nil {
		t.Fatal(err)
	}
	// Conta só processos vivos: o agente parado no meio morre sem apagar o seu arquivo.
	sh := scriptTeste(t, root, "lento", "d=\""+contagem+"\"\ntouch \"$d/run.$$\"\nn=0\nfor f in \"$d\"/run.*; do kill -0 \"${f##*.}\" 2>/dev/null && n=$((n+1)); done\necho $n >> \"$d/../contagens\"\nsleep 0.4\nrm -f \"$d/run.$$\"\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"ok\"}'\n"+fimOK)
	registrar(t, harness.CustomSpec{Name: "lento-teste", Command: sh})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		if i == 0 { // um dos agentes é parado no meio
			time.AfterFunc(250*time.Millisecond, cancel)
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer cancel()
			_, _ = Run(ctx, root, Options{Name: "lento" + string(rune('0'+i)), Motor: "lento-teste", AgentsDir: agents, PromptText: "x", MaxAgents: 2, Attempts: 1})
		}(i)
	}
	wg.Wait()
	b := mustRead(t, filepath.Join(filepath.Dir(contagem), "contagens"))
	max := 0
	for _, l := range strings.Fields(b) {
		if n := int(l[0] - '0'); n > max {
			max = n
		}
	}
	if max == 0 || max > 2 {
		t.Fatalf("simultâneos medidos: %d (limite 2); contagens=%q", max, b)
	}
	t.Logf("máximo de agentes rodando ao mesmo tempo: %d (limite 2)", max)
}

// ---- item 3: missão somente leitura ----

func TestMissaoNaoDeixaRastroNoRepositorio(t *testing.T) {
	root, agents := repoFixture(t)
	marcador := filepath.Join(t.TempDir(), "cwd.txt")
	sh := scriptTeste(t, root, "suja", "pwd > \""+marcador+"\"\ntouch .aider.chat.history.md\necho lixo > .gitignore\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"ok\"}'\n"+fimOK)
	registrar(t, harness.CustomSpec{Name: "suja-teste", Command: sh})
	if _, err := Run(context.Background(), root, Options{Name: "missao-suja", Motor: "suja-teste", AgentsDir: agents, PromptText: "x", MaxAgents: 9}); err != nil {
		t.Fatal(err)
	}
	cwd := strings.TrimSpace(mustRead(t, marcador))
	if real(t, root) == cwd || strings.HasPrefix(cwd, real(t, root)) {
		t.Fatalf("missão rodou dentro do repositório: %s", cwd)
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatalf("pasta descartável não foi apagada: %v", err)
	}
	if out, _ := exec.Command("git", "-C", root, "status", "--porcelain", "--", ".", ":!.claude", ":!suja.sh").Output(); len(out) != 0 {
		t.Fatalf("repositório sujo: %s", out)
	}
	for _, f := range []string{".aider.chat.history.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(root, f)); !os.IsNotExist(err) {
			t.Fatalf("%s vazou para o repositório", f)
		}
	}
	if out, _ := exec.Command("git", "-C", root, "worktree", "list").Output(); strings.Count(string(out), "\n") != 1 {
		t.Fatalf("worktree da missão ficou registrada: %s", out)
	}
}

// ---- item 4: repositório sem main ----

func repoMaster(t *testing.T, commits bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "master")
	git(t, root, "config", "user.email", "t@example.invalid")
	git(t, root, "config", "user.name", "T")
	if commits {
		os.WriteFile(filepath.Join(root, "README"), []byte("x\n"), 0644)
		git(t, root, "add", ".")
		git(t, root, "commit", "-m", "base")
	}
	return root, filepath.Join(root, ".claude", "agentes")
}

func TestRepositorioSemMainUsaBranchAtual(t *testing.T) {
	root, agents := repoMaster(t, true)
	res, err := Run(context.Background(), root, Options{Name: "m1", Motor: "mock", AgentsDir: agents, PromptText: "ok", MaxAgents: 9})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if out, _ := exec.Command("git", "-C", root, "rev-parse", "--verify", "agente/m1").Output(); len(out) == 0 {
		t.Fatal("branch agente/m1 não criada")
	}
}

func TestRepositorioSemCommitsFalhaComLogEFim(t *testing.T) {
	root, agents := repoMaster(t, false)
	res, err := Run(context.Background(), root, Options{Name: "vazio", Motor: "mock", AgentsDir: agents, PromptText: "ok", MaxAgents: 9})
	if err == nil || res.Name != "vazio" || res.Code != 1 || res.Causa == "" {
		t.Fatalf("esperava erro com resultado: res=%+v err=%v", res, err)
	}
	log := mustRead(t, res.LogFile)
	if !strings.Contains(log, "ERRO:") || !strings.Contains(log, "FIM ") {
		t.Fatalf("log mudo: %q", log)
	}
	if !strings.Contains(mustRead(t, res.MetaFile), `"codigo":1`) {
		t.Fatalf("meta não fechado: %s", mustRead(t, res.MetaFile))
	}
}

func TestBranchBaseInexistenteFalhaComLogEFim(t *testing.T) {
	root, agents := repoFixture(t)
	res, err := Run(context.Background(), root, Options{Name: "base-ruim", Motor: "mock", AgentsDir: agents, PromptText: "ok", BranchBase: "nao-existe", MaxAgents: 9})
	if err == nil || !strings.Contains(mustRead(t, res.LogFile), "nao-existe") || !strings.Contains(mustRead(t, res.LogFile), "FIM ") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// ---- item 5: cota/sobrecarga só no stdout ----

func rodarCom(t *testing.T, nome, corpo string, spec harness.CustomSpec, o Options) (Result, error, string) {
	t.Helper()
	root, agents := repoFixture(t)
	spec.Name, spec.Command = nome, scriptTeste(t, root, nome, corpo)
	registrar(t, spec)
	o.Name, o.Motor, o.AgentsDir, o.PromptText = "ag", nome, agents, "x"
	if o.MaxAgents == 0 {
		o.MaxAgents = 9
	}
	if o.Sleep == nil {
		o.Sleep = func(time.Duration) {}
	}
	res, err := Run(context.Background(), root, o)
	return res, err, mustRead(t, res.LogFile)
}

func texto(s string) string {
	return "printf '%s\\n' '{\"type\":\"text\",\"text\":\"" + s + "\"}'\n" + fimOK
}

func TestCotaSoNoStdoutSemRegexViraFalha(t *testing.T) {
	res, err, _ := rodarCom(t, "stdout-cota", texto("You have hit your session limit · resets 3pm"), harness.CustomSpec{}, Options{Attempts: 3})
	if err != nil || res.Code != 2 || res.Attempts != 1 {
		t.Fatalf("cota em stdout: err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
}

func TestSobrecargaSoNoStdoutUsaReserva(t *testing.T) {
	root, agents := repoFixture(t)
	registrar(t, harness.CustomSpec{Name: "stdout-sobrecarga", Command: scriptTeste(t, root, "so", texto("Service temporarily overloaded")), Reserva: []string{"stdout-reserva"}})
	registrar(t, harness.CustomSpec{Name: "stdout-reserva", Command: scriptTeste(t, root, "sr", texto("feito"))})
	res, err := Run(context.Background(), root, Options{Name: "ag", Motor: "stdout-sobrecarga", AgentsDir: agents, PromptText: "x", Attempts: 2, MaxAgents: 9})
	if err != nil || res.Code != 0 || res.Attempts != 2 {
		t.Fatalf("sobrecarga em stdout: err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
}

func TestTextoSobreCotaNaoEFalsoPositivo(t *testing.T) {
	longo := "Quota exceeded é o erro que tratei no código. " + strings.Repeat("Detalhes da implementação. ", 12)
	casos := map[string]string{
		"com-ferramenta": "printf '%s\\n' '{\"type\":\"tool_call\",\"tool\":\"Bash\"}'\n" + texto("Quota exceeded"),
		"resumo-curto":   texto("tratei o caso de quota exceeded e de SEM COTA no código"),
		"resumo-longo":   texto(longo),
		"diz-limite":     texto("O rate limit da API é de 60 por minuto, documentei"),
	}
	for nome, corpo := range casos {
		res, err, log := rodarCom(t, "fp-"+nome, corpo, harness.CustomSpec{}, Options{Attempts: 2})
		if err != nil || res.Code != 0 || res.Attempts != 1 {
			t.Errorf("%s: falso positivo: err=%v código=%d tentativas=%d log=%s", nome, err, res.Code, res.Attempts, log)
		}
	}
}

// ---- item 6: erro transitório sem reserva ----

func TestErroTransitorioRepeteMesmaInstanciaComEsperaCrescente(t *testing.T) {
	root, agents := repoFixture(t)
	n := filepath.Join(t.TempDir(), "n")
	sh := scriptTeste(t, root, "transitorio", "echo x >> \""+n+"\"\nif [ $(wc -l < \""+n+"\") -lt 3 ]; then printf '%s\\n' '{\"type\":\"error\",\"message\":\"ServiceUnavailableError: 503\"}'; fi\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"pronto\"}'\n"+fimOK)
	registrar(t, harness.CustomSpec{Name: "transitorio-teste", Command: sh})
	var mu sync.Mutex
	var esperas []time.Duration
	sleep := func(d time.Duration) {
		if d >= time.Second {
			mu.Lock()
			esperas = append(esperas, d)
			mu.Unlock()
		}
	}
	res, err := Run(context.Background(), root, Options{Name: "ag", Motor: "transitorio-teste", AgentsDir: agents, PromptText: "x", Attempts: 4, MaxAgents: 9, Sleep: sleep})
	if err != nil || res.Code != 0 || res.Attempts != 3 {
		t.Fatalf("err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
	if len(esperas) != 2 || esperas[0] >= esperas[1] {
		t.Fatalf("esperas deveriam crescer: %v", esperas)
	}
}

func TestErroTransitorioEsgotadoDizACausa(t *testing.T) {
	res, err, log := rodarCom(t, "sempre-503", "printf '%s\\n' '{\"type\":\"error\",\"message\":\"ServiceUnavailableError: 503\"}'\n"+fimOK, harness.CustomSpec{}, Options{Attempts: 3})
	if err == nil || res.Code != 1 || res.Attempts != 3 {
		t.Fatalf("err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
	if strings.Contains(err.Error(), "nenhuma tentativa") || !strings.Contains(err.Error(), "503") || !strings.Contains(res.Causa, "503") {
		t.Fatalf("mensagem enganosa: %v / causa %q", err, res.Causa)
	}
	if strings.Count(log, "### tentativa") != 3 {
		t.Fatalf("log: %s", log)
	}
}

func TestFimComCausaQuandoMotorSaiComErro(t *testing.T) {
	res, err, _ := rodarCom(t, "modelo-ruim", "printf '%s\\n' '{\"type\":\"error\",\"message\":\"model foo does not exist\"}'\nprintf '%s\\n' '{\"type\":\"end\",\"reason\":\"process_error\"}'\n", harness.CustomSpec{}, Options{Attempts: 1})
	if err == nil || res.Code != 1 || !strings.Contains(res.Causa, "model foo does not exist") {
		t.Fatalf("err=%v código=%d causa=%q", err, res.Code, res.Causa)
	}
}

// ---- item 7: --seco ----

func TestSecoNaoDeixaArquivosEMostraComandoCompleto(t *testing.T) {
	root, agents := repoFixture(t)
	registrar(t, harness.CustomSpec{Name: "seco-codex2", Base: "codex", Model: "gpt-reserve-x", Env: map[string]string{"CODEX_HOME": "/h/.codex2", "OPENAI_API_KEY": "segredo"}})
	if _, err := Run(context.Background(), root, Options{Name: "seco", Motor: "seco-codex2", AgentsDir: agents, PromptText: "x", Seco: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(agents, "logs")); !os.IsNotExist(err) {
		t.Fatalf("--seco criou a pasta logs: %v", err)
	}
	cmd := dryRunCommand(Options{Motor: "seco-codex2"})
	for _, quer := range []string{"codex exec", "-m gpt-reserve-x", "CODEX_HOME=/h/.codex2", "OPENAI_API_KEY=***"} {
		if !strings.Contains(cmd, quer) {
			t.Errorf("comando seco sem %q: %s", quer, cmd)
		}
	}
	if strings.Contains(cmd, "segredo") {
		t.Errorf("vazou segredo: %s", cmd)
	}
	for motor, bin := range map[string]string{"claude-code": "claude -p", "opencode": "opencode run", "aider": "aider --yes-always", "agy": "agy "} {
		if c := dryRunCommand(Options{Motor: motor, Model: "m-x"}); !strings.Contains(c, bin) || !strings.Contains(c, "m-x") {
			t.Errorf("%s: %s", motor, c)
		}
	}
	if c := dryRunCommand(Options{Motor: "codex"}); !strings.Contains(c, "-m gpt-reserve") {
		t.Errorf("modelo efetivo do codex: %s", c)
	}
}
