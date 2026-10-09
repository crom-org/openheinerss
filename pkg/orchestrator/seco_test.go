package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestSecoResolveTudoSemCriarArtefatosENaoExpoeSegredo(t *testing.T) {
	root, agents := repoFixture(t)
	marker := filepath.Join(t.TempDir(), "nao-rodou")
	registrar(t, harness.CustomSpec{Name: "seco-json", Base: "codex", Model: "gpt-teste", Env: map[string]string{
		"CODEX_HOME": "/tmp/codex-seco", "OPENAI_API_KEY": "segredo-nao-sair",
	}})
	res, err := Seco(context.Background(), root, Options{Name: "plano", Motor: "seco-json", AgentsDir: agents, PromptText: "oi", Retomar: true, QuotaMax: 80, WhenLoadBelow: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Seco || res.Base != "codex" || res.Modelo != "gpt-teste" || res.Branch != "agente/plano" {
		t.Fatalf("plano incompleto: %+v", res)
	}
	if !strings.Contains(res.Prompt, "CONTINUAÇÃO") || !strings.Contains(res.Prompt, "oi") {
		t.Fatalf("prompt seco não contém continuação: %q", res.Prompt)
	}
	if res.Env["OPENAI_API_KEY"] != "***" || res.Env["CODEX_HOME"] != "/tmp/codex-seco" {
		t.Fatalf("ambiente não mascarado: %#v", res.Env)
	}
	if res.Limites.QuandoCargaAbaixo != 3 || res.TrocaConta.Fonte != "cache local; sem consulta ativa" {
		t.Fatalf("limites/troca incorretos: %+v %+v", res.Limites, res.TrocaConta)
	}
	if _, err := os.Stat(filepath.Join(agents, "plano")); !os.IsNotExist(err) {
		t.Fatalf("modo seco criou worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents, "logs")); !os.IsNotExist(err) {
		t.Fatalf("modo seco criou logs: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("marcador inesperado")
	}
}
