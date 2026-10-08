package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

func repoFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "teste@example.invalid")
	git(t, root, "config", "user.name", "Teste")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "base")
	agents := filepath.Join(root, ".claude", "agentes")
	if err := os.MkdirAll(filepath.Join(agents, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	return root, agents
}

func TestRunMockCriaWorktreeLogEMeta(t *testing.T) {
	root, agents := repoFixture(t)
	if err := os.WriteFile(filepath.Join(agents, "prompts", "agente.md"), []byte("faça a tarefa"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "agente", Motor: "mock", AgentsDir: agents, MaxAgents: 99})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(agents, "agente", ".git")); err != nil {
		t.Fatalf("worktree não criada: %v", err)
	}
	b, _ := os.ReadFile(res.LogFile)
	if !strings.Contains(string(b), "FIM ") {
		t.Fatalf("log sem FIM: %s", b)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(mustRead(t, res.MetaFile)), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"motor", "modelo", "esforco", "conta", "tentativa", "inicio", "pid", "fim", "codigo"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("meta sem %s: %s", k, mustRead(t, res.MetaFile))
		}
	}
}

func TestRunRecusaMesmoNomeComMetaVivo(t *testing.T) {
	root, agents := repoFixture(t)
	if err := os.WriteFile(filepath.Join(agents, "prompts", "duplicado.md"), []byte("prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(filepath.Join(agents, "logs", "duplicado.meta.json"), meta{PID: os.Getpid(), Inicio: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), root, Options{Name: "duplicado", Motor: "mock", AgentsDir: agents})
	if err == nil || !strings.Contains(err.Error(), "já está rodando") {
		t.Fatalf("esperava recusa clara, veio %v", err)
	}
}

func TestRunEscreveEventoFimQuandoConfigurado(t *testing.T) {
	root, agents := repoFixture(t)
	if err := os.WriteFile(filepath.Join(agents, "prompts", "evento.md"), []byte("prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "eventos.log")
	if _, err := Run(context.Background(), root, Options{Name: "evento", Motor: "mock", AgentsDir: agents, EventLog: log, MaxAgents: 99}); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, log)
	if !strings.Contains(got, "["+filepath.Base(root)+"] FIM evento código 0") {
		t.Fatalf("evento final ausente: %q", got)
	}
}

func TestRunRetomarPreservaLogEUsaReserva(t *testing.T) {
	root, agents := repoFixture(t)
	script := filepath.Join(root, "falso.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nread p\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"SEM COTA\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	reserve := filepath.Join(root, "reserva.sh")
	if err := os.WriteFile(reserve, []byte("#!/bin/sh\nread p\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"OK reserva\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "quota-teste", Command: script, QuotaRegex: "SEM COTA", Reserva: []string{"reserva-teste"}}); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "reserva-teste", Command: reserve}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "prompts", "missao.md"), []byte("prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "missao", Motor: "quota-teste", AgentsDir: agents, Attempts: 2, MaxAgents: 99})
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 {
		t.Fatalf("tentativas: %d", res.Attempts)
	}
	before := mustRead(t, res.LogFile)
	if !strings.Contains(before, "OK reserva") || !strings.Contains(before, "FIM ") {
		t.Fatalf("log da reserva: %s", before)
	}
	res2, err := Run(context.Background(), root, Options{Name: "missao", Motor: "reserva-teste", AgentsDir: agents, Retomar: true, MaxAgents: 99})
	if err != nil {
		t.Fatal(err)
	}
	after := mustRead(t, res2.LogFile)
	if len(after) <= len(before) || !strings.Contains(after, "OK reserva") {
		t.Fatal("retomar apagou o log")
	}
}

func TestRunCotaSemReservaParaSemRepetir(t *testing.T) {
	root, agents := repoFixture(t)
	script := filepath.Join(root, "cota-sem-reserva.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nread p\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"SEM COTA resets 9am\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cota-sem-reserva", Command: script, QuotaRegex: "SEM COTA"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "prompts", "cota.md"), []byte("prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "cota", Motor: "cota-sem-reserva", AgentsDir: agents, Attempts: 4, MaxAgents: 99})
	if err != nil || res.Code != 2 || res.Attempts != 1 {
		t.Fatalf("cota sem reserva: err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
	log := mustRead(t, res.LogFile)
	if !strings.Contains(log, "FIM ") || !strings.Contains(log, "resets 9am") {
		t.Fatalf("log sem encerramento ou aviso de retorno: %s", log)
	}
}

func TestPadraoCotaClaudeReconheceAmostrasReais(t *testing.T) {
	for _, amostra := range []string{"You've hit your session limit · resets 9am", "You have hit your session limit", "usage limit reached"} {
		if !quotaPattern("claude-code").MatchString(amostra) {
			t.Fatalf("amostra não reconhecida: %q", amostra)
		}
	}
}

func TestRunBloqueiaCargaEAgentes(t *testing.T) {
	root, agents := repoFixture(t)
	if err := os.WriteFile(filepath.Join(agents, "prompts", "limite.md"), []byte("p"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	b, _ := json.Marshal(meta{PID: pid, Inicio: time.Now().Format(time.RFC3339)})
	if err := os.WriteFile(filepath.Join(agents, "logs", "ocupado.meta.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	_, err := Run(ctx, root, Options{Name: "limite", Motor: "mock", AgentsDir: agents, MaxAgents: 1, MaxLoad: 99})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("esperava bloqueio por simultâneos: %v", err)
	}
	_ = os.Remove(filepath.Join(agents, "logs", "ocupado.meta.json"))
	ctx, cancel = context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	_, err = Run(ctx, root, Options{Name: "limite", Motor: "mock", AgentsDir: agents, MaxAgents: 99, MaxLoad: 1, Load: func() (float64, error) { return 2, nil }, Sleep: func(time.Duration) {}})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("esperava bloqueio por carga: %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFimComAvisoNoStderrNaoEFalha(t *testing.T) {
	root := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, a...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	agents := filepath.Join(root, ".claude", "agentes")
	os.MkdirAll(filepath.Join(agents, "prompts"), 0755)
	os.WriteFile(filepath.Join(agents, "prompts", "aviso.md"), []byte("x"), 0644)
	script := filepath.Join(root, "aviso.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nread p\necho 'ERROR rede caiu, tentando de novo' >&2\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"pronto\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "aviso-teste", Command: script}); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "aviso", Motor: "aviso-teste", AgentsDir: agents, MaxAgents: 99})
	if err != nil || res.Code != 0 || res.Attempts != 1 {
		t.Fatalf("aviso no stderr virou falha: err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
}

func TestMissaoExecutaNaRaizDoProjeto(t *testing.T) {
	root, agents := repoFixture(t)
	marker := filepath.Join(root, "cwd.txt")
	script := filepath.Join(root, "cwd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd > \""+marker+"\"\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"ok\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cwd-missao-teste", Command: script}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "prompts", "missao-lacunas.md"), []byte("prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), root, Options{Name: "missao-lacunas", Motor: "cwd-missao-teste", AgentsDir: agents, MaxAgents: 99}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != root {
		t.Fatalf("CWD da missão: %q; esperado %q", strings.TrimSpace(string(b)), root)
	}
}

func TestTextoDoAgenteFalandoDeCotaNaoTrocaDeMotor(t *testing.T) {
	root := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, a...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	agents := filepath.Join(root, ".claude", "agentes")
	os.MkdirAll(filepath.Join(agents, "prompts"), 0755)
	os.WriteFile(filepath.Join(agents, "prompts", "texto.md"), []byte("x"), 0644)
	script := filepath.Join(root, "texto.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nread p\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"implementei a parada por SEM COTA e quota exceeded\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "texto-teste", Command: script}); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "texto", Motor: "texto-teste", AgentsDir: agents, MaxAgents: 99})
	if err != nil || res.Code != 0 || res.Attempts != 1 {
		t.Fatalf("texto sobre cota virou falta de cota: err=%v código=%d tentativas=%d", err, res.Code, res.Attempts)
	}
}

func TestRunRejeitaNomeQueSaiDaPasta(t *testing.T) {
	root, agents := repoFixture(t)
	for _, nome := range []string{"../fora", "a/b", ".oculto", "com espaço"} {
		if _, err := Run(context.Background(), root, Options{Name: nome, Motor: "mock", AgentsDir: agents}); err == nil {
			t.Errorf("nome %q deveria ser recusado", nome)
		}
	}
}

func TestRunSemPromptNaoCriaWorktree(t *testing.T) {
	root, agents := repoFixture(t)
	if _, err := Run(context.Background(), root, Options{Name: "sem-prompt", Motor: "mock", AgentsDir: agents}); err == nil {
		t.Fatal("esperava erro por falta do prompt")
	}
	if _, err := os.Stat(filepath.Join(agents, "sem-prompt")); err == nil {
		t.Error("worktree criada mesmo sem prompt")
	}
}

func TestRunComPromptEmTextoNaoPrecisaDeArquivo(t *testing.T) {
	root, agents := repoFixture(t)
	if err := os.WriteFile(filepath.Join(agents, "prompts", "_regras.md"), []byte("REGRAS-DO-PROJETO"), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := readPrompt(agents, "x", "", "faça isso")
	if err != nil || p != "REGRAS-DO-PROJETO\n\nfaça isso" {
		t.Fatalf("prompt = %q, err = %v", p, err)
	}
	res, err := Run(context.Background(), root, Options{Name: "texto-inline", Motor: "mock", AgentsDir: agents, PromptText: "responda OK", Attempts: 1})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
}

func TestWriteMetaConcorrenteNaoUsaTemporarioCompartilhado(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agente.meta.json")
	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := writeMeta(path, meta{Motor: "teste", Tentativa: i + 1, PID: i + 1}); err != nil {
				t.Errorf("escrita %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	var got meta
	if err := json.Unmarshal([]byte(mustRead(t, path)), &got); err != nil {
		t.Fatalf("meta final inválido: %v", err)
	}
	if got.Motor != "teste" || got.Tentativa < 1 || got.Tentativa > n {
		t.Fatalf("meta final inesperado: %+v", got)
	}
}

func TestRunSecoNaoCriaWorktree(t *testing.T) {
	root, agents := repoFixture(t)
	res, err := Run(context.Background(), root, Options{Name: "seco", Motor: "codex", AgentsDir: agents, PromptText: "não executar", Seco: true})
	if err != nil || res.Code != 0 {
		t.Fatalf("seco: resultado=%+v erro=%v", res, err)
	}
	if _, err := os.Stat(filepath.Join(agents, "seco")); !os.IsNotExist(err) {
		t.Fatalf("modo seco criou worktree: %v", err)
	}
}
