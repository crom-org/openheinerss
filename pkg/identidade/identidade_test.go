package identidade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
)

func TestIDParaIdentificadorAninhadoEhDeterministico(t *testing.T) {
	dir := t.TempDir()
	b, err := json.Marshal(map[string]interface{}{
		"z": map[string]interface{}{"email": "email@example.test", "accountUuid": "uuid-prioritario"},
		"a": map[string]interface{}{"account_id": "id-prioritario"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	var anterior string
	for i := 0; i < 50; i++ {
		id, fonte := IDPara("codex", dir)
		if fonte != "id-real" {
			t.Fatalf("fonte inesperada: %q", fonte)
		}
		if i > 0 && id != anterior {
			t.Fatalf("contaId mudou na repetição %d: %s != %s", i, anterior, id)
		}
		anterior = id
	}
}

func TestParaCanonicalizaSymlinkENome(t *testing.T) {
	root := t.TempDir()
	conta := filepath.Join(root, "conta")
	if err := os.Mkdir(conta, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "atalho")
	if err := os.Symlink(conta, link); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, "instancias")
	if err := os.Mkdir(cfg, 0700); err != nil {
		t.Fatal(err)
	}
	arquivo := filepath.Join(cfg, "Conta2.yaml")
	conteudo := "name: claude-conta2\nbase: claude-code\nenv:\n  CLAUDE_CONFIG_DIR: " + link + "\n"
	if err := os.WriteFile(arquivo, []byte(conteudo), 0600); err != nil {
		t.Fatal(err)
	}
	contaResolvida, err := filepath.EvalSymlinks(conta)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.LoadCustomFile(arquivo); err != nil {
		t.Fatal(err)
	}
	a, err := Para("claude-conta2", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Para("CLAUDE-CONTA2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ContaID != b.ContaID {
		t.Fatalf("nome não deveria mudar contaId: %q != %q", a.ContaID, b.ContaID)
	}
	if a.ContaID == "" || a.ContaDir != contaResolvida {
		t.Fatalf("identidade inesperada: %+v", a)
	}
	if a.ConfigFonte != arquivo {
		t.Fatalf("configFonte=%q, esperado %q", a.ConfigFonte, arquivo)
	}
	outra := filepath.Join(root, "outra")
	if err := os.Mkdir(outra, 0700); err != nil {
		t.Fatal(err)
	}
	c, err := Para("claude-code", map[string]string{"CLAUDE_CONFIG_DIR": outra})
	if err != nil {
		t.Fatal(err)
	}
	if c.ContaID == a.ContaID {
		t.Fatal("pastas diferentes não podem compartilhar contaId")
	}
}
