package main

import (
	"bytes"
	"github.com/crom-org/openheinerss/pkg/harness"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredPort(t *testing.T) {
	t.Setenv("OPENHEINERSS_PORTA", "4931")
	if got := configuredPort(); got != 4931 {
		t.Fatalf("porta env: %d", got)
	}
	t.Setenv("OPENHEINERSS_PORTA", "invalida")
	if got := configuredPort(); got != 4820 {
		t.Fatalf("fallback: %d", got)
	}
}

func TestServeFlagsPorta(t *testing.T) {
	t.Setenv("OPENHEINERSS_PORTA", "4820")
	cmd := newServeCmd()
	if got := cmd.Flag("porta").DefValue; got != "4820" {
		t.Fatalf("default --porta: %s", got)
	}
	if got := cmd.Flag("port").DefValue; got != "4820" {
		t.Fatalf("default --port: %s", got)
	}
}

func TestManualCLIAtualizado(t *testing.T) {
	gerado, err := renderCLIDoc(newRootCmd())
	if err != nil {
		t.Fatal(err)
	}
	manual, err := os.ReadFile(filepath.Join("..", "..", "docs", "09-cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gerado, manual) {
		t.Fatal("docs/09-cli.md está desatualizado; execute 'go run ./cmd/openheinerss docs'")
	}
}

func TestInstanciaCustomDaRaizCarregaDeDentroDaWorktree(t *testing.T) {
	root := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i")
	dir := filepath.Join(root, ".openheinerss", "harnesses")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// A instância existe só na raiz (não versionada): a worktree não a tem.
	if err := os.WriteFile(filepath.Join(dir, "so-na-raiz-r5.yaml"), []byte("name: so-na-raiz-r5\nbase: codex\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	run(root, "worktree", "add", "-q", wt, "-b", "agente/x")
	if err := carregarInstancias(wt); err != nil {
		t.Fatal(err)
	}
	if !harness.Exists("so-na-raiz-r5") {
		t.Fatal("instância da raiz do repositório não carregou ao lançar de dentro da worktree")
	}
}

func TestFlagVersionFunciona(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"--version"})
	var out strings.Builder
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("--version falhou: %v", err)
	}
	if !strings.Contains(out.String(), "commit") {
		t.Fatalf("--version sem commit: %q", out.String())
	}
}
