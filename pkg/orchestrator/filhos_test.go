package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

const textoAguardando = "printf '%s\\n' '{\"type\":\"text\",\"text\":\"Lancei tudo; aguardando os executores.\"}'\n"

// paiScript grava cada prompt recebido em <dir>/prompt-N.txt e roda corpo1 na 1ª chamada e corpoN nas seguintes.
func paiScript(t *testing.T, dir, corpo1, corpoN string) string {
	t.Helper()
	p := filepath.Join(dir, "pai.sh")
	s := "#!/bin/sh\nn=$(cat '" + dir + "/n' 2>/dev/null || echo 0); n=$((n+1)); echo $n > '" + dir + "/n'\n" +
		"cat > '" + dir + "'/prompt-$n.txt\n" +
		"if [ $n -eq 1 ]; then\n" + corpo1 + "else\n" + corpoN + "fi\n" + fimOK
	if err := os.WriteFile(p, []byte(s), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func chamadas(t *testing.T, dir string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "n"))
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range strings.TrimSpace(string(b)) {
		n = n*10 + int(c-'0')
	}
	return n
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if binPath != "" {
		_ = os.RemoveAll(filepath.Dir(binPath))
	}
	os.Exit(code)
}

// binario compila o openheinerss uma vez por execução dos testes (o filho real é um `openheinerss rodar`).
func binario(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "openheinerss-bin-")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "openheinerss")
		out, err := exec.Command("go", "build", "-o", binPath, "github.com/crom-org/openheinerss/cmd/openheinerss").CombinedOutput()
		if err != nil {
			binErr = &exec.ExitError{Stderr: out}
		}
	})
	if binErr != nil {
		t.Fatalf("compilar openheinerss: %v", binErr)
	}
	return binPath
}

func TestPaiLancaFilhoEERetomadoComResumoDoFilho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts sh")
	}
	if testing.Short() {
		t.Skip("compila o binário")
	}
	bin := binario(t)
	root, agents := repoFixture(t)
	dados := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(filepath.Join(cfg, "harnesses"), 0755); err != nil {
		t.Fatal(err)
	}
	filhoSh := filepath.Join(dados, "filho.sh")
	// O filho demora mais que o turno do pai: o pai termina com ele ainda vivo.
	if err := os.WriteFile(filhoSh, []byte("#!/bin/sh\ncat >/dev/null\nsleep 1\necho feito > filho.txt\ngit add filho.txt && git commit -qm filho\n"+texto("filho pronto")), 0755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: filho-lento\ncommand: " + filhoSh + "\nprompt: stdin\n"
	if err := os.WriteFile(filepath.Join(cfg, "harnesses", "filho-lento.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	lanca := "OPENHEINERSS_CONFIG='" + cfg + "' '" + bin + "' rodar filho-a filho-lento --texto 'faça o filho' --max-agentes 9 </dev/null >'" + dados + "/filho.out' 2>&1 &\n" +
		"i=0; while [ ! -d \"$OPENHEINERSS_PAI_LOGS/$OPENHEINERSS_PAI.filhos\" ] && [ $i -lt 100 ]; do sleep 0.05; i=$((i+1)); done\n" + textoAguardando
	registrar(t, harness.CustomSpec{Name: "pai-mock", Command: paiScript(t, dados, lanca, texto("juntei o trabalho"))})
	res, err := Run(context.Background(), root, Options{Name: "pai", Motor: "pai-mock", AgentsDir: agents, PromptText: "orquestre", MaxAgents: 9, IntervaloFilhos: 50 * time.Millisecond})
	if err != nil || res.Code != 0 {
		t.Fatalf("pai: err=%v código=%d causa=%s", err, res.Code, res.Causa)
	}
	if n := chamadas(t, dados); n != 2 {
		t.Fatalf("pai deveria rodar 2 turnos (lançar e juntar), rodou %d; saída do filho: %s", n, mustRead(t, filepath.Join(dados, "filho.out")))
	}
	p2 := mustRead(t, filepath.Join(dados, "prompt-2.txt"))
	for _, want := range []string{"RETOMADA AUTOMÁTICA (rodada 1 de 5)", "- filho-a: FIM", "código 0", "branch agente/filho-a"} {
		if !strings.Contains(p2, want) {
			t.Fatalf("mensagem de retomada sem %q:\n%s", want, p2)
		}
	}
	var m meta
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(agents, "logs", "filho-a.meta.json"))), &m); err != nil || m.Pai != "pai" || m.Fim == "" {
		t.Fatalf("meta do filho: pai=%q fim=%q err=%v", m.Pai, m.Fim, err)
	}
	log := mustRead(t, res.LogFile)
	if !strings.Contains(log, "aguardando 1 agente(s) filho(s): filho-a") || !strings.Contains(log, "### retomada 1 de 5") {
		t.Fatalf("log do pai sem a espera/retomada:\n%s", log)
	}
	lista, err := ListAgents(agents, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range lista {
		if a.Nome == "pai" && (len(a.Filhos) != 1 || a.Filhos[0] != "filho-a") {
			t.Fatalf("agentes não mostra os filhos do pai: %+v", a)
		}
		if a.Nome == "filho-a" && a.Pai != "pai" {
			t.Fatalf("agentes não mostra o pai do filho: %+v", a)
		}
	}
}

func TestSemFilhosComportamentoAtual(t *testing.T) {
	root, agents := repoFixture(t)
	dados := t.TempDir()
	// Diz que vai esperar, mas sem filhos e sem mudança na worktree: não há o que retomar.
	registrar(t, harness.CustomSpec{Name: "pai-sem-filhos", Command: paiScript(t, dados, textoAguardando, texto("nunca"))})
	res, err := Run(context.Background(), root, Options{Name: "sozinho", Motor: "pai-sem-filhos", AgentsDir: agents, PromptText: "x", MaxAgents: 9, IntervaloFilhos: 50 * time.Millisecond})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if n := chamadas(t, dados); n != 1 {
		t.Fatalf("sem filhos o pai roda um turno só, rodou %d", n)
	}
	if strings.Contains(mustRead(t, res.LogFile), "retomada") {
		t.Fatal("log não deveria ter retomada")
	}
}

func TestTurnoQueDizQueEsperaComMudancasERetomadoAteOLimite(t *testing.T) {
	root, agents := repoFixture(t)
	dados := t.TempDir()
	corpo := "echo x >> sujo.txt\n" + textoAguardando
	registrar(t, harness.CustomSpec{Name: "pai-teimoso", Command: paiScript(t, dados, corpo, corpo)})
	res, err := Run(context.Background(), root, Options{Name: "teimoso", Motor: "pai-teimoso", AgentsDir: agents, PromptText: "x", MaxAgents: 9, RodadasFilhos: 2, IntervaloFilhos: 50 * time.Millisecond})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if n := chamadas(t, dados); n != 3 {
		t.Fatalf("esperava 1 turno + 2 retomadas, veio %d", n)
	}
	p2 := mustRead(t, filepath.Join(dados, "prompt-2.txt"))
	if !strings.Contains(p2, "nada vai te acordar") || !strings.Contains(p2, "PRIMEIRO plano") || !strings.Contains(p2, "rodada 1 de 2") {
		t.Fatalf("mensagem de retomada sem filhos:\n%s", p2)
	}
	if !strings.Contains(mustRead(t, res.LogFile), "limite de 2 rodada(s)") {
		t.Fatal("log sem o aviso de limite de rodadas")
	}
}

func TestEsperarFilhosDesligado(t *testing.T) {
	root, agents := repoFixture(t)
	dados := t.TempDir()
	corpo := "echo x >> sujo.txt\n" + textoAguardando
	registrar(t, harness.CustomSpec{Name: "pai-desligado", Command: paiScript(t, dados, corpo, corpo)})
	res, err := Run(context.Background(), root, Options{Name: "desligado", Motor: "pai-desligado", AgentsDir: agents, PromptText: "x", MaxAgents: 9, EsperarFilhos: -1})
	if err != nil || res.Code != 0 || chamadas(t, dados) != 1 {
		t.Fatalf("com --esperar-filhos=nao roda um turno só: err=%v código=%d chamadas=%d", err, res.Code, chamadas(t, dados))
	}
}

func TestFilhoSeRegistraNoPai(t *testing.T) {
	root, agents := repoFixture(t)
	paiLogs := t.TempDir()
	res, err := Run(context.Background(), root, Options{Name: "filho-reg", Motor: "mock", AgentsDir: agents, PromptText: "x", MaxAgents: 9, Pai: "chefe", PaiLogs: paiLogs})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	fs := listarFilhos(paiLogs, "chefe")
	if len(fs) != 1 || fs[0].Nome != "filho-reg" {
		t.Fatalf("registro do filho: %+v", fs)
	}
	e := lerFilho(fs[0])
	if e.Vivo || e.Fim == "" || e.Branch != "agente/filho-reg" || !strings.Contains(descreverFilho(e), "código 0") {
		t.Fatalf("estado do filho: %+v", e)
	}
}

func TestRegraPadraoFalaDosFilhos(t *testing.T) {
	if !strings.Contains(defaultPromptRules, "o openheinerss te acorda quando eles terminarem") {
		t.Fatal("regra dos filhos ausente")
	}
}

func TestParseEsperarFilhos(t *testing.T) {
	casos := map[string]time.Duration{"": 0, "sim": 0, "nao": -1, "false": -1, "30m": 30 * time.Minute, "45": 45 * time.Minute}
	for v, want := range casos {
		if got, err := ParseEsperarFilhos(v); err != nil || got != want {
			t.Fatalf("%q: got %v err %v", v, got, err)
		}
	}
	if _, err := ParseEsperarFilhos("talvez"); err == nil {
		t.Fatal("valor inválido aceito")
	}
}
