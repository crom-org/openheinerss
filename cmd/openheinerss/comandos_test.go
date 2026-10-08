package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// comandos/commands: instância de --config herda o catálogo da base; anotar e confirmar gravam em <config>/comandos.yaml.
func TestComandosCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("compila o binário")
	}
	bin := binarioTeste(t)
	home := t.TempDir()
	cfg := t.TempDir()
	escrever(t, filepath.Join(cfg, "harnesses", "conta2.yaml"), "name: conta2\nbase: claude-code\nenv:\n  CLAUDE_CONFIG_DIR: "+filepath.Join(home, "c2")+"\n")
	escrever(t, filepath.Join(home, "c2", "commands", "deploy.md"), "---\ndescription: Faz deploy\n---\n")
	rodar := func(args ...string) string {
		t.Helper()
		c := exec.Command(bin, args...)
		c.Dir = t.TempDir()
		c.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "OPENHEINERSS_CONFIG=", "CLAUDE_CONFIG_DIR=")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	rodar("--config", cfg, "comandos", "anotar", "claude-code", "/compact", "serve para compactar o claude code; o central não usa")
	rodar("--config", cfg, "comandos", "confirmar", "conta2", "/compact")
	var l struct {
		Base     string `json:"base"`
		Comandos []struct {
			Nome, Repasse, Origem, Anotacao string
			Confirmado                      bool
		} `json:"comandos"`
	}
	if err := json.Unmarshal([]byte(rodar("--config", cfg, "commands", "conta2", "--json")), &l); err != nil {
		t.Fatal(err)
	}
	vistos := map[string]bool{}
	for _, c := range l.Comandos {
		vistos[c.Nome] = true
		if c.Nome == "/compact" && (c.Anotacao != "serve para compactar o claude code; o central não usa" || !c.Confirmado || c.Repasse != "literal") {
			t.Fatalf("/compact: %+v", c)
		}
		if c.Nome == "/deploy" && c.Origem != "descoberto" {
			t.Fatalf("/deploy: %+v", c)
		}
	}
	if l.Base != "claude-code" || !vistos["/compact"] || !vistos["/deploy"] || !vistos["/model"] {
		t.Fatalf("lista: base=%s vistos=%v", l.Base, vistos)
	}
	b, err := os.ReadFile(filepath.Join(cfg, "comandos.yaml"))
	if err != nil || !strings.Contains(string(b), "conta2:") || !strings.Contains(string(b), "claude-code:") {
		t.Fatalf("comandos.yaml: %v\n%s", err, b)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "openheinerss", "comandos.yaml")); !os.IsNotExist(err) {
		t.Fatal("com --config não deveria gravar no global")
	}
	if out := rodar("comandos", "codex"); !strings.Contains(out, "/compact") || !strings.Contains(out, "sem_equivalente") {
		t.Fatalf("texto codex:\n%s", out)
	}
}
