package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// ---- bug 3: chaves por execução, sem os.Setenv global ----

func TestChavesNaoVazamParaOutroAgenteNemTravam(t *testing.T) {
	root, agents := repoFixture(t)
	chaves := filepath.Join(t.TempDir(), "fake.env")
	if err := os.WriteFile(chaves, []byte("R5_PRIVATE=FAKE_SENTINEL\n"), 0600); err != nil {
		t.Fatal(err)
	}
	solto := filepath.Join(t.TempDir(), "solto")
	lento := scriptTeste(t, root, "lento", "while [ ! -e \""+solto+"\" ]; do sleep 0.05; done\necho \"lento=$R5_PRIVATE\" >&2\n"+texto("ok"))
	eco := scriptTeste(t, root, "eco", "echo \"eco=[$R5_PRIVATE]\"\nprintf '%s\\n' '{\"type\":\"tool_call\",\"tool\":\"x\"}'\n"+texto("ok"))
	registrar(t, harness.CustomSpec{Name: "lento-r5", Command: lento})
	registrar(t, harness.CustomSpec{Name: "eco-r5", Command: eco})

	feito := make(chan Result, 1)
	go func() {
		res, _ := Run(context.Background(), root, Options{Name: "com-chaves", Motor: "lento-r5", AgentsDir: agents, PromptText: "x", KeysFile: chaves, MaxAgents: 9})
		feito <- res
	}()
	// Espera o primeiro agente estar rodando (meta escrito) e roda outro, SEM arquivo de chaves.
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(filepath.Join(agents, "logs", "com-chaves.log")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if v := os.Getenv("R5_PRIVATE"); v != "" {
		t.Fatalf("segredo foi parar no ambiente do processo: %q", v)
	}
	res, err := Run(context.Background(), root, Options{Name: "sem-chaves", Motor: "eco-r5", AgentsDir: agents, PromptText: "x", MaxAgents: 9})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if log := mustRead(t, res.LogFile); strings.Contains(log, "FAKE_SENTINEL") || !strings.Contains(log, "eco=[]") {
		t.Fatalf("o agente sem chaves enxergou o segredo: %s", log)
	}
	// Outra execução com chaves e contexto já cancelado não pode ficar presa esperando a primeira.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	retorno := make(chan struct{})
	go func() {
		_, _ = Run(ctx, root, Options{Name: "cancelado", Motor: "eco-r5", AgentsDir: agents, PromptText: "x", KeysFile: chaves, MaxAgents: 9})
		close(retorno)
	}()
	select {
	case <-retorno:
	case <-time.After(5 * time.Second):
		t.Fatal("Run com contexto cancelado ficou preso (trava global de chaves)")
	}
	if err := os.WriteFile(solto, nil, 0644); err != nil {
		t.Fatal(err)
	}
	r := <-feito
	if !strings.Contains(mustRead(t, r.LogFile), "lento=FAKE_SENTINEL") {
		t.Fatalf("o agente com chaves deveria receber o segredo: %s", mustRead(t, r.LogFile))
	}
}

// ---- bug 4: espera de trava cancelável ----

func TestTravaDeArquivoEsperaCancelavel(t *testing.T) {
	agents := filepath.Join(t.TempDir(), "agentes")
	trava := filepath.Join(agents, "logs", ".worktree.lock")
	if err := os.MkdirAll(filepath.Dir(trava), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(trava, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ini := time.Now()
	err = withFileLock(ctx, trava, func() error { t.Error("não devia entrar na seção crítica"); return nil })
	if err == nil || time.Since(ini) > 3*time.Second {
		t.Fatalf("a espera ignorou o cancelamento: err=%v em %v", err, time.Since(ini))
	}
}

func TestRunCanceladoNaoFicaPresoNaTravaDeWorktreeNemDeVagas(t *testing.T) {
	for _, nome := range []string{".worktree.lock", ".vagas.lock"} {
		root, agents := repoFixture(t)
		if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(agents, "logs", nome), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		retorno := make(chan error, 1)
		go func() {
			_, err := Run(ctx, root, Options{Name: "preso", Motor: "mock", AgentsDir: agents, PromptText: "x", MaxAgents: 9})
			retorno <- err
		}()
		select {
		case err := <-retorno:
			if err == nil {
				t.Errorf("%s: esperava erro de cancelamento", nome)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("%s: Run não terminou depois do cancelamento", nome)
		}
		cancel()
		f.Close()
	}
}

// ---- bug 7: --seco mascara todo valor de ambiente ----

func TestSecoMascaraTodosOsValoresDeAmbiente(t *testing.T) {
	registrar(t, harness.CustomSpec{Name: "seco-r5", Base: "codex", Env: map[string]string{
		"AUTHORIZATION": "FAKE_BEARER_SECRET", "CREDENTIALS": "FAKE_CREDENTIAL", "COOKIE": "FAKE_COOKIE",
		"CODEX_HOME": "/h/.codex2", "CLAUDE_CONFIG_DIR": "/h/uma pasta/.claude",
	}})
	cmd := dryRunCommand(Options{Motor: "seco-r5"})
	for _, segredo := range []string{"FAKE_BEARER_SECRET", "FAKE_CREDENTIAL", "FAKE_COOKIE"} {
		if strings.Contains(cmd, segredo) {
			t.Errorf("--seco imprimiu %s: %s", segredo, cmd)
		}
	}
	for _, quer := range []string{"AUTHORIZATION=***", "CREDENTIALS=***", "CODEX_HOME=/h/.codex2", "CLAUDE_CONFIG_DIR='/h/uma pasta/.claude'"} {
		if !strings.Contains(cmd, quer) {
			t.Errorf("faltou %q: %s", quer, cmd)
		}
	}
}

// ---- --seco: agy com -p e caminhos com espaço entre aspas ----

func TestSecoAgyTemPEECaminhosComEspacoEntreAspas(t *testing.T) {
	if c := dryRunCommand(Options{Motor: "agy", Model: "m-x"}); !strings.Contains(c, " -p <prompt>") || !strings.Contains(c, "--model m-x") {
		t.Errorf("agy sem -p: %s", c)
	}
	registrar(t, harness.CustomSpec{Name: "espaco-r5", Command: "/tmp/pasta com espaço/meu motor.sh", Args: []string{"--flag", "dois valores"}})
	c := dryRunCommand(Options{Motor: "espaco-r5"})
	if !strings.Contains(c, "'/tmp/pasta com espaço/meu motor.sh' --flag 'dois valores'") {
		t.Errorf("caminho com espaço sem aspas: %s", c)
	}
}

// ---- bug 8: missão não escreve no Git do repositório e o relatório sobrevive ----

func TestMissaoNaoCriaRefNoRepositorioEPreservaRelatorio(t *testing.T) {
	root, agents := repoFixture(t)
	antes, _ := exec.Command("git", "-C", root, "for-each-ref").Output()
	sh := scriptTeste(t, root, "injeta", "git update-ref refs/heads/r5-injected HEAD\ngit tag r5-tag 2>/dev/null\necho '# relatório da missão' > RELATORIO-AGENTE.md\n"+texto("ok"))
	registrar(t, harness.CustomSpec{Name: "injeta-r5", Command: sh})
	res, err := Run(context.Background(), root, Options{Name: "missao-r5", Motor: "injeta-r5", AgentsDir: agents, PromptText: "x", MaxAgents: 9})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	depois, _ := exec.Command("git", "-C", root, "for-each-ref").Output()
	if string(antes) != string(depois) {
		t.Fatalf("a missão alterou as refs do repositório:\nantes:\n%s\ndepois:\n%s", antes, depois)
	}
	if out, _ := exec.Command("git", "-C", root, "branch", "--list", "r5-injected").Output(); len(out) != 0 {
		t.Fatalf("branch injetada apareceu no repositório: %s", out)
	}
	dest := filepath.Join(agents, "relatorios", "missao-r5.md")
	if !strings.Contains(mustRead(t, dest), "relatório da missão") || res.Relatorio != dest {
		t.Fatalf("relatório perdido: Result.Relatorio=%q", res.Relatorio)
	}
}

// ---- 14.8a: meta do codex com o modelo e o esforço reais ----

func TestMetaDoCodexTemModeloEEsforcoReais(t *testing.T) {
	root, agents := repoFixture(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"t1\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	registrar(t, harness.CustomSpec{Name: "codex-r5", Base: "codex"})
	for _, motor := range []string{"codex", "codex-r5"} {
		res, err := Run(context.Background(), root, Options{Name: "m-" + motor, Motor: motor, AgentsDir: agents, PromptText: "x", MaxAgents: 9})
		if err != nil || res.Code != 0 {
			t.Fatalf("%s: err=%v código=%d", motor, err, res.Code)
		}
		m := mustRead(t, res.MetaFile)
		if !strings.Contains(m, `"modelo":"gpt-reserve"`) || !strings.Contains(m, `"esforco":"medium"`) {
			t.Errorf("%s: meta sem o modelo/esforço reais: %s", motor, m)
		}
	}
}

// ---- --eventos-log com pasta inexistente avisa ----

func TestEventosLogComPastaInexistenteAvisaNoLog(t *testing.T) {
	root, agents := repoFixture(t)
	ruim := filepath.Join(t.TempDir(), "nao", "existe", "eventos.log")
	res, err := Run(context.Background(), root, Options{Name: "evlog", Motor: "mock", AgentsDir: agents, PromptText: "x", MaxAgents: 9, EventLog: ruim})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if log := mustRead(t, res.LogFile); !strings.Contains(log, "AVISO") || !strings.Contains(log, ruim) {
		t.Fatalf("falha do log de eventos ficou calada: %s", log)
	}
}
