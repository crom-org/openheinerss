package motor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerfisMontamMotorModeloEAmbiente(t *testing.T) {
	cases := map[string]struct{ harness, provider, model, env string }{
		"codex":          {"codex", "", "gpt-5-codex", ""},
		"codex2":         {"codex2", "", "gpt-5-codex", "CODEX_HOME="},
		"claude":         {"claude", "", "claude-sonnet-5-5", ""},
		"claude-conta2":  {"claude", "", "claude-sonnet-5-5", "CLAUDE_CONFIG_DIR="},
		"cco-openrouter": {"cco", "openrouter", "", ""},
		"cco-zen":        {"cco", "opencode-zen", "", ""},
		"opencode":       {"opencode", "", "opencode/big-pickle", ""},
	}
	for name, want := range cases {
		p, err := Resolve(name, "", "")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p.Harness != want.harness || p.Provider != want.provider || p.Model != want.model {
			t.Errorf("%s: perfil inesperado: %+v", name, p)
		}
		if want.env != "" {
			found := false
			for k, v := range p.Env {
				if k+"=" == want.env || strings.HasPrefix(k+"="+v, want.env) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: ambiente ausente: %+v", name, p.Env)
			}
		}
	}
}

func TestLoadRolesEValidaErros(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motores.yaml")
	if err := os.WriteFile(path, []byte("roteirista: codex/gpt-reserve\nrapido: opencode/opencode/big-pickle esforco=low\n"), 0600); err != nil {
		t.Fatal(err)
	}
	roles, err := LoadRoles(path)
	if err != nil {
		t.Fatal(err)
	}
	if roles["roteirista"].Harness != "codex" || roles["roteirista"].Model != "gpt-reserve" {
		t.Fatalf("papel codex incorreto: %+v", roles["roteirista"])
	}
	if roles["rapido"].Effort != "low" {
		t.Fatalf("esforço não lido: %+v", roles["rapido"])
	}
	if _, err := Resolve("inexistente", "", ""); err == nil || !strings.Contains(err.Error(), "desconhecido") {
		t.Fatalf("erro de motor desconhecido pouco claro: %v", err)
	}
	bad := filepath.Join(dir, "ruim.yaml")
	_ = os.WriteFile(bad, []byte("revisor: inexistente/modelo\n"), 0600)
	if _, err := LoadRoles(bad); err == nil || !strings.Contains(err.Error(), "papel 'revisor'") {
		t.Fatalf("erro de papel desconhecido pouco claro: %v", err)
	}
}
