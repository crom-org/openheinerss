package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestSextaVerificacaoMissaoRemoveOrigin(t *testing.T) {
	root, agents := repoFixture(t)
	marcador := filepath.Join(t.TempDir(), "remote.txt")
	script := scriptTeste(t, root, "remote", "git remote -v > "+marcador+"\ngit push origin HEAD:refs/heads/nao-deve-existir >/dev/null 2>&1 || true\n"+texto("ok"))
	registrar(t, harness.CustomSpec{Name: "remote-r6", Command: script})
	res, err := Run(context.Background(), root, Options{Name: "missao-r6", Motor: "remote-r6", AgentsDir: agents, PromptText: "--- REGRAS PADRÃO\nfaça o teste", MaxAgents: 9})
	if err != nil || res.Code != 0 {
		t.Fatalf("missão: err=%v código=%d", err, res.Code)
	}
	if got := mustRead(t, marcador); strings.Contains(got, "origin") {
		t.Fatalf("clone de missão ainda tinha origin: %q", got)
	}
	if out, _ := exec.Command("git", "-C", root, "branch", "--list", "nao-deve-existir").Output(); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatal("push da missão criou branch no repositório real")
	}
}

func TestSextaVerificacaoPararAntesDaMetaCancela(t *testing.T) {
	root, agents := repoFixture(t)
	if err := StopAgent(agents, "fila-r6", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), root, Options{Name: "fila-r6", Motor: "mock", AgentsDir: agents, PromptText: "x", MaxAgents: 9})
	if err == nil || !strings.Contains(err.Error(), "cancelado antes de começar") {
		t.Fatalf("esperava cancelamento antes da inicialização, veio %v", err)
	}
}

func TestSextaVerificacaoPastaAgentesForaRecusada(t *testing.T) {
	root, _ := repoFixture(t)
	if _, err := ValidateAgentsDir(root, filepath.Join(root, "..", "fora")); err == nil || !strings.Contains(err.Error(), "fora do repositório") {
		t.Fatalf("erro de caminho externo pouco claro: %v", err)
	}
}

func TestSextaVerificacaoErrosDeCLIUnknownOptionSaoFalha(t *testing.T) {
	if !unknownOptionPattern.MatchString("error: unknown option '--regras'") || !unknownOptionPattern.MatchString("unknown option") {
		t.Fatal("padrão de opção desconhecida não reconhecido")
	}
}

func TestSextaVerificacaoNomeLongoTemErroClaro(t *testing.T) {
	root, agents := repoFixture(t)
	_, err := Run(context.Background(), root, Options{Name: strings.Repeat("a", 81), Motor: "mock", AgentsDir: agents, PromptText: "x"})
	if err == nil || !strings.Contains(err.Error(), "longo demais") {
		t.Fatalf("esperava erro de nome longo, veio %v", err)
	}
}

func TestSetimaVerificacaoPastaAgentesSymlinkComDoisNiveisInexistentes(t *testing.T) {
	root, _ := repoFixture(t)
	fora := t.TempDir()
	link := filepath.Join(root, "link-r7")
	if err := os.Symlink(fora, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAgentsDir(root, filepath.Join("link-r7", "inexistente", "agentes")); err == nil || !strings.Contains(err.Error(), "fora do repositório") {
		t.Fatalf("symlink para fora com dois níveis inexistentes foi aceito: %v", err)
	}
	if _, err := ValidateAgentsDir(root, filepath.Join("novo", "a", "b")); err != nil {
		t.Fatalf("caminho novo dentro do repositório deveria valer: %v", err)
	}
}

func TestSetimaVerificacaoPromptVazioEhFalha(t *testing.T) {
	root, agents := repoFixture(t)
	res, err := Run(context.Background(), root, Options{Name: "vazio-r7", Motor: "mock", AgentsDir: agents, PromptText: "   ", SemRegras: true, MaxAgents: 9})
	if err == nil && res.Code == 0 {
		t.Fatal("prompt vazio deveria falhar")
	}
}
