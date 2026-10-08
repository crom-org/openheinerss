package limites

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

func TestObterLêCodexEClaudePorInstancia(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := filepath.Join(home, "codex-conta")
	dir := filepath.Join(codexHome, "sessions", "2026", "10", "08")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "codex.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), b, 0644); err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, "claude-conta")
	if err := os.MkdirAll(filepath.Join(home, ".config", "crom-painel"), 0755); err != nil {
		t.Fatal(err)
	}
	status := `{"em":1791457200000,"rate_limits":{"five_hour":{"used_percentage":12,"resets_at":1791459000},"seven_day":{"used_percentage":34,"resets_at":1792058400}}}`
	if err := os.WriteFile(filepath.Join(home, ".config", "crom-painel", "statusline-conta-teste-limites.json"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "codex-teste-limites", Base: "codex", Env: map[string]string{"CODEX_HOME": codexHome}}); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-teste-limites", Base: "claude-code", Env: map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}}); err != nil {
		t.Fatal(err)
	}
	r := Obter()
	var codex, claude *Instancia
	for i := range r.Instancias {
		if r.Instancias[i].Nome == "codex-teste-limites" {
			codex = &r.Instancias[i]
		}
		if r.Instancias[i].Nome == "conta-teste-limites" {
			claude = &r.Instancias[i]
		}
	}
	if codex == nil || len(codex.Janelas) != 2 || codex.Janelas[0].Percentual != 42.5 {
		t.Fatalf("codex inesperado: %+v", codex)
	}
	if claude == nil || len(claude.Janelas) != 2 || claude.Janelas[0].Percentual != 12 {
		t.Fatalf("claude inesperado: %+v", claude)
	}
	if strings.Contains(status, "token") {
		t.Fatal("fixture de statusline contém token")
	}
}
