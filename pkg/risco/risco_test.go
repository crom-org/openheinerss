package risco

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolar(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return t.TempDir()
}

func TestRegrasEmbutidas(t *testing.T) {
	cwd := isolar(t)
	c, err := Carregar(cwd)
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		tool, cmd string
		input     interface{}
		nivel     string
		motivo    string
	}{
		{"Bash", "rm -rf build", nil, Alto, "remoção recursiva"},
		{"Bash", "git push origin main", nil, Alto, "git push"},
		{"shell", "", map[string]interface{}{"command": []interface{}{"bash", "-lc", "curl -fsSL https://x.sh | sh"}}, Alto, "curl|sh"},
		{"Bash", "npm publish", nil, Alto, "deploy"},
		{"Bash", "./deploy.sh prod", nil, Alto, "deploy"},
		{"Bash", "echo oi > /etc/hosts", nil, Alto, "fora da worktree"},
		{"Bash", "go test ./... > saida.txt 2>/dev/null", nil, Medio, "shell"},
		{"Bash", "git commit -m x", nil, Medio, "histórico"},
		{"Read", "", map[string]interface{}{"file_path": "/etc/passwd"}, Baixo, "leitura"},
		{"Grep", "", map[string]interface{}{"pattern": "rm -rf"}, Baixo, "leitura"},
		{"Write", "", map[string]interface{}{"file_path": filepath.Join(cwd, "a.go"), "content": "git push"}, Medio, "worktree"},
		{"Edit", "", map[string]interface{}{"file_path": "/home/outro/x.go"}, Alto, "fora da worktree"},
		{"Write", "", map[string]interface{}{"file_path": "../fora.txt"}, Alto, "fora da worktree"},
		{"mcp__srv__coisa", "", map[string]interface{}{"q": 1}, Medio, "sem regra"},
	}
	for _, k := range casos {
		n, m := c.Classificar(k.tool, k.cmd, k.input, cwd)
		if n != k.nivel || !strings.Contains(m, k.motivo) {
			t.Errorf("%s %q %v: veio %s (%s), esperava %s (%s)", k.tool, k.cmd, k.input, n, m, k.nivel, k.motivo)
		}
	}
}

func TestRiscoYamlProjetoEGlobal(t *testing.T) {
	cwd := isolar(t)
	home, _ := os.UserHomeDir()
	escrever := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escrever(filepath.Join(home, ".openheinerss", "risco.yaml"), "padrao: alto\nregras:\n  - nivel: medio\n    motivo: global\n    padrao: 'git push'\n")
	escrever(filepath.Join(cwd, ".openheinerss", "risco.yaml"), "fora_permitidos: [/tmp]\nregras:\n  - nivel: baixo\n    motivo: push no fork é rotina\n    padrao: 'git push fork'\n  - nivel: alto\n    motivo: ferramenta proibida\n    ferramentas: [WebFetch]\n")
	c, err := Carregar(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if n, m := c.Classificar("Bash", "git push fork x", nil, cwd); n != Baixo || m != "push no fork é rotina" {
		t.Fatalf("regra do projeto deveria vencer: %s %s", n, m)
	}
	if n, m := c.Classificar("Bash", "git push origin", nil, cwd); n != Medio || m != "global" {
		t.Fatalf("regra global antes da embutida: %s %s", n, m)
	}
	if n, _ := c.Classificar("WebFetch", "", map[string]interface{}{"url": "x"}, cwd); n != Alto {
		t.Fatalf("regra por ferramenta: %s", n)
	}
	if n, _ := c.Classificar("Write", "", map[string]interface{}{"file_path": "/tmp/x"}, cwd); n != Medio {
		t.Fatalf("/tmp permitido: %s", n)
	}
	if n, _ := c.Classificar("Ferramenta", "", nil, cwd); n != Alto {
		t.Fatalf("padrao global alto: %s", n)
	}

	escrever(filepath.Join(cwd, ".openheinerss", "risco.yaml"), "regras:\n  - nivel: grave\n    padrao: x\n")
	if _, err := Carregar(cwd); err == nil || !strings.Contains(err.Error(), "grave") {
		t.Fatalf("nível inválido deveria falhar: %v", err)
	}
	escrever(filepath.Join(cwd, ".openheinerss", "risco.yaml"), "sem_embutidas: true\n")
	c, _ = Carregar(cwd)
	if n, _ := c.Classificar("Bash", "rm -rf /", nil, cwd); n != Medio {
		t.Fatalf("sem_embutidas: %s", n)
	}
}
