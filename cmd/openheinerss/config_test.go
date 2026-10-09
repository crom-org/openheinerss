package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// binarioTeste compila o CLI uma vez num diretório temporário.
func binarioTeste(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "openheinerss")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func escrever(t *testing.T, path, conteudo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conteudo), 0644); err != nil {
		t.Fatal(err)
	}
}

// O resultado de harness list, motores e limites --json é o mesmo rodando de pastas
// diferentes quando a pasta de configuração é dada por --config, --configuracao ou OPENHEINERSS_CONFIG.
func TestConfigIndependeDaPastaAtual(t *testing.T) {
	if testing.Short() {
		t.Skip("compila o binário")
	}
	bin := binarioTeste(t)
	home := t.TempDir()
	cfg := t.TempDir()
	escrever(t, filepath.Join(cfg, "harnesses", "cfg-externa.yaml"), "name: cfg-externa\nbase: codex\n")
	escrever(t, filepath.Join(cfg, "motores.yaml"), "revisor: codex/gpt-cfg\n")
	// Projeto com .openheinerss/ próprio: aceito como --config também.
	proj := t.TempDir()
	escrever(t, filepath.Join(proj, ".openheinerss", "harnesses", "cfg-projeto.yaml"), "name: cfg-projeto\nbase: codex\n")

	// Pastas de onde se roda: um repositório com configuração diferente e uma pasta vazia.
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	escrever(t, filepath.Join(repo, ".openheinerss", "harnesses", "do-cwd.yaml"), "name: do-cwd\nbase: codex\n")
	escrever(t, filepath.Join(repo, ".openheinerss", "motores.yaml"), "revisor: codex/gpt-cwd\n")
	vazia := t.TempDir()

	rodar := func(dir string, env []string, args ...string) string {
		t.Helper()
		c := exec.Command(bin, args...)
		c.Dir = dir
		c.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, env...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v em %s: %v\n%s", args, dir, err, out)
		}
		return string(out)
	}
	nomesLimites := func(out string) []string {
		var r struct {
			Instancias []struct{ Nome, Base string } `json:"instancias"`
		}
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Fatalf("limites --json: %v\n%s", err, out)
		}
		var nomes []string
		for _, i := range r.Instancias {
			nomes = append(nomes, i.Nome)
		}
		return nomes
	}

	formas := []struct {
		nome string
		env  []string
		pre  []string
	}{
		{"flag", nil, []string{"--config", cfg}},
		{"alias", nil, []string{"--configuracao=" + cfg}},
		{"env", []string{"OPENHEINERSS_CONFIG=" + cfg}, nil},
		// A flag vence a variável.
		{"flag>env", []string{"OPENHEINERSS_CONFIG=" + proj}, []string{"--config", cfg}},
	}
	var refLista, refMotores string
	var refLimites []string
	for _, f := range formas {
		for _, dir := range []string{repo, vazia} {
			lista := rodar(dir, f.env, append(append([]string{}, f.pre...), "harness", "list")...)
			motores := rodar(dir, f.env, append([]string{"motores"}, f.pre...)...)
			lim := nomesLimites(rodar(dir, f.env, append([]string{"limites", "--json"}, f.pre...)...))
			if !strings.Contains(lista, "cfg-externa") || strings.Contains(lista, "do-cwd") || strings.Contains(lista, "cfg-projeto") {
				t.Fatalf("%s em %s: harness list não usou a pasta de configuração:\n%s", f.nome, dir, lista)
			}
			if !strings.Contains(motores, "gpt-cfg") || strings.Contains(motores, "gpt-cwd") {
				t.Fatalf("%s em %s: motores não usou motores.yaml da configuração:\n%s", f.nome, dir, motores)
			}
			if !strings.Contains(strings.Join(lim, ","), "cfg-externa") {
				t.Fatalf("%s em %s: limites não listou a instância da configuração: %v", f.nome, dir, lim)
			}
			if refLista == "" {
				refLista, refMotores, refLimites = lista, motores, lim
				continue
			}
			if lista != refLista || motores != refMotores || !reflect.DeepEqual(lim, refLimites) {
				t.Fatalf("%s em %s: resultado mudou\nlista=%q\nref=%q\nmotores=%q\nref=%q\nlimites=%v ref=%v", f.nome, dir, lista, refLista, motores, refMotores, lim, refLimites)
			}
		}
	}

	// Sem --config vale a busca atual: o repositório enxerga a própria instância.
	if out := rodar(repo, nil, "harness", "list"); !strings.Contains(out, "do-cwd") || strings.Contains(out, "cfg-externa") {
		t.Fatalf("sem --config deveria usar a pasta atual:\n%s", out)
	}
	// Uma pasta de projeto (com .openheinerss/) também serve como --config.
	if out := rodar(vazia, nil, "--config", proj, "harness", "list"); !strings.Contains(out, "cfg-projeto") {
		t.Fatalf("--config com pasta de projeto não usou .openheinerss/:\n%s", out)
	}
	// harness add grava na pasta de configuração, não no cwd.
	novo := filepath.Join(t.TempDir(), "adicionado.yaml")
	escrever(t, novo, "name: adicionado\nbase: codex\n")
	rodar(vazia, nil, "--config", cfg, "harness", "add", novo)
	if _, err := os.Stat(filepath.Join(cfg, "harnesses", "adicionado.yaml")); err != nil {
		t.Fatalf("harness add não gravou em --config: %v", err)
	}
	// Pasta inexistente é erro claro.
	c := exec.Command(bin, "--config", filepath.Join(vazia, "nao-existe"), "harness", "list")
	c.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	if out, err := c.CombinedOutput(); err == nil || !strings.Contains(string(out), "não existe") {
		t.Fatalf("esperado erro para pasta inexistente, veio err=%v\n%s", err, out)
	}
}

func TestFlagConfig(t *testing.T) {
	casos := map[string][]string{
		"":  {"harness", "list"},
		"a": {"--config", "a", "harness", "list"},
		"b": {"serve", "--configuracao=b"},
		"c": {"--config=x", "limites", "--config", "c"},
		"d": {"rodar", "--configuracao", "d", "--", "--config", "z"},
	}
	for want, args := range casos {
		if got := flagConfig(args); got != want {
			t.Errorf("flagConfig(%v) = %q, quer %q", args, got, want)
		}
	}
}

func TestHarnessGlobalEncontradoForaDoRepositorio(t *testing.T) {
	if testing.Short() {
		t.Skip("compila o binário")
	}
	bin := binarioTeste(t)
	home, xdg, repo := t.TempDir(), t.TempDir(), t.TempDir()
	global := filepath.Join(xdg, "openheinerss", "harnesses")
	escrever(t, filepath.Join(global, "codex2.yaml"), "name: codex2\nbase: codex\nenv:\n  CODEX_HOME: ~/.codex-compartilhado\n")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	c := exec.Command(bin, "harness", "list")
	c.Dir = repo
	c.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + xdg, "PATH=" + os.Getenv("PATH")}
	out, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "codex2") {
		t.Fatalf("codex2 global não foi encontrado fora do repositório: %v\n%s", err, out)
	}

	// Uma instância de mesmo nome no projeto vence a global e aparece na identidade.
	escrever(t, filepath.Join(repo, ".openheinerss", "harnesses", "codex2.yaml"), "name: codex2\nbase: codex\nenv:\n  CODEX_HOME: ~/.codex-do-projeto\n")
	c = exec.Command(bin, "identidade", "codex2", "--json")
	c.Dir = repo
	c.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + xdg, "PATH=" + os.Getenv("PATH")}
	out, err = c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), filepath.Join(repo, ".openheinerss", "harnesses", "codex2.yaml")) {
		t.Fatalf("projeto não venceu o global: %v\n%s", err, out)
	}
}
