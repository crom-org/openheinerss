package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func estadoFixture(t *testing.T, prompt string) (*estadoAgente, string, string) {
	t.Helper()
	agents, wt := worktreeFixture(t)
	logs := filepath.Join(agents, "logs")
	_ = os.MkdirAll(logs, 0755)
	repo := gitOut(t, wt, "rev-parse", "--git-common-dir")
	repo = filepath.Dir(repo)
	escrever(t, filepath.Join(logs, "ag.log"), "### tentativa 1 (10:00) motor mock\nfiz a parte A\n[usage] {}\nfiz a parte B com token=abc123segredo\n\nFIM 10:01 código 1\n")
	e := novoEstadoAgente(logs, "ag", wt, repo, "", prompt, time.Now)
	return e, wt, logs
}

func TestEstadoGeraDiffHeadEventosEMascara(t *testing.T) {
	e, wt, logs := estadoFixture(t, "# Fazer X\n\ndetalhes com api_key=SUPERSEGREDO123")
	escrever(t, filepath.Join(wt, "novo.txt"), "oi\n")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "novo")
	escrever(t, filepath.Join(wt, "solto.txt"), "x\n")
	e.FimDeTurno()
	b, err := os.ReadFile(filepath.Join(logs, "ag.estado.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, quer := range []string{"turno: 1", "head: " + headDe(wt), "novo.txt", "solto.txt", "fiz a parte A", "fiz a parte B", "Fazer X"} {
		if !strings.Contains(s, quer) {
			t.Errorf("estado sem %q:\n%s", quer, s)
		}
	}
	for _, nao := range []string{"abc123segredo", "SUPERSEGREDO123"} {
		if strings.Contains(s, nao) {
			t.Errorf("segredo %q vazou:\n%s", nao, s)
		}
	}
}

func TestEstadoResumoNovoEhUsado(t *testing.T) {
	e, wt, _ := estadoFixture(t, "# Tarefa")
	escrever(t, filepath.Join(wt, "RELATORIO-AGENTE.md"), "feito: tudo menos o Z")
	e.FimDeTurno()
	c := e.Continuacao()
	if !strings.Contains(c, "feito: tudo menos o Z") || !strings.Contains(c, "o git manda") && !strings.Contains(c, "fonte da verdade") {
		t.Fatalf("resumo em dia deveria ir:\n%s", c)
	}
}

func TestEstadoHeadMudouDescartaResumo(t *testing.T) {
	e, wt, _ := estadoFixture(t, "# Tarefa original")
	escrever(t, filepath.Join(wt, "RELATORIO-AGENTE.md"), "resumo antigo")
	e.FimDeTurno()
	escrever(t, filepath.Join(wt, "a.txt"), "a\n")
	git(t, wt, "add", "a.txt")
	git(t, wt, "commit", "-m", "depois do resumo")
	e.FimDeTurno()
	c := e.Continuacao()
	if strings.Contains(c, "resumo antigo") || !strings.Contains(c, "HEAD mudou") || !strings.Contains(c, "Tarefa original") || !strings.Contains(c, "a.txt") {
		t.Fatalf("esperava histórico sem o resumo:\n%s", c)
	}
}

func TestEstadoResumoAtrasadoDescartado(t *testing.T) {
	t.Setenv(EnvResumoMaxTurnos, "1")
	e, wt, _ := estadoFixture(t, "# T")
	escrever(t, filepath.Join(wt, "RELATORIO-AGENTE.md"), "resumo parado")
	e.FimDeTurno() // turno 1: resumo visto
	if c := e.Continuacao(); !strings.Contains(c, "resumo parado") {
		t.Fatalf("1 turno de atraso é aceito:\n%s", c)
	}
	e.FimDeTurno()
	e.FimDeTurno() // turno 3, resumo do turno 1: atraso 2 > 1
	if c := e.Continuacao(); strings.Contains(c, "resumo parado") || !strings.Contains(c, "turnos atrasado") {
		t.Fatalf("resumo atrasado deveria cair:\n%s", c)
	}
}

func TestEstadoSemGitNemLogCaiNoFixo(t *testing.T) {
	dir := t.TempDir()
	e := novoEstadoAgente(dir, "ag", dir, "", "", "# T", time.Now)
	if got := e.ContinuacaoOu(continuation); got != continuation {
		t.Fatalf("esperava o texto fixo, veio %q", got)
	}
	// só log, sem git: usa os eventos
	escrever(t, filepath.Join(dir, "ag.log"), "fiz algo\n")
	if got := e.ContinuacaoOu(continuation); !strings.Contains(got, "fiz algo") {
		t.Fatalf("esperava estado com o log: %q", got)
	}
}
