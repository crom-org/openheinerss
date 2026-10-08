package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ambienteContexto isola HOME/XDG e devolve (home, repo) temporários.
func ambienteContexto(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(EnvConfigDir, "")
	SetConfigDir("")
	return home, t.TempDir()
}

func gravar(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestContextoSemConfigEstaDesligado(t *testing.T) {
	_, repo := ambienteContexto(t)
	c, err := ContextoEfetivo(repo, "claude-conta2", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if c.Ligado() || c.LimiteTokens != 0 {
		t.Fatalf("esperava desligado: %+v", c)
	}
}

func TestContextoSeisCamadasEmOrdem(t *testing.T) {
	home, repo := ambienteContexto(t)
	glob := filepath.Join(home, ".openheinerss", ConfigFileName)
	proj := filepath.Join(repo, WorkspaceDirName, ConfigFileName)
	gravar(t, glob, `contexto:
  padrao: {limite_tokens: 1, acao: aviso}
  harnesses:
    claude-code: {limite_tokens: 2}
    inst: {limite_tokens: 3}
`)
	// Remove da camada mais forte para a mais fraca.
	passos := []struct {
		projeto string
		limite  int
		origem  string
		camada  string
	}{
		{`contexto:
  harnesses:
    inst: {limite_tokens: 6}
    claude-code: {limite_tokens: 5}
  padrao: {limite_tokens: 4}
`, 6, OrigemProjeto, "projeto harnesses.inst"},
		{`contexto:
  harnesses:
    claude-code: {limite_tokens: 5}
  padrao: {limite_tokens: 4}
`, 5, OrigemProjeto, "projeto harnesses.claude-code"},
		{`contexto:
  padrao: {limite_tokens: 4}
`, 4, OrigemProjeto, "projeto padrao"},
		{``, 3, OrigemGlobal, "global harnesses.inst"},
	}
	for _, p := range passos {
		gravar(t, proj, p.projeto)
		c, err := ContextoEfetivo(repo, "inst", "claude-code")
		if err != nil {
			t.Fatal(err)
		}
		if c.LimiteTokens != p.limite || c.OrigemLimite != p.origem || c.CamadaLimite != p.camada {
			t.Fatalf("projeto %q: %+v", p.projeto, c)
		}
	}
	// global: sem a instância, vale o harness base e depois o padrão.
	c, _ := ContextoEfetivo(repo, "outra", "claude-code")
	if c.LimiteTokens != 2 || c.CamadaLimite != "global harnesses.claude-code" {
		t.Fatalf("base global: %+v", c)
	}
	c, _ = ContextoEfetivo(repo, "outra", "codex")
	if c.LimiteTokens != 1 || c.CamadaLimite != "global padrao" {
		t.Fatalf("padrão global: %+v", c)
	}
}

func TestContextoResolveCampoACampo(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, ".config", "openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 150000, acao: aviso}\n")
	gravar(t, filepath.Join(repo, WorkspaceDirName, ConfigFileName), `contexto:
  harnesses:
    claude-conta2: {acao: nova-sessao}
    codex: {limite_tokens: 200000}
`)
	c, err := ContextoEfetivo(repo, "claude-conta2", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if c.LimiteTokens != 150000 || c.OrigemLimite != OrigemGlobal || c.Acao != AcaoNovaSessao || c.OrigemAcao != OrigemProjeto {
		t.Fatalf("claude-conta2: %+v", c)
	}
	c, _ = ContextoEfetivo(repo, "codex", "codex")
	if c.LimiteTokens != 200000 || c.OrigemLimite != OrigemProjeto || c.Acao != AcaoAviso || c.OrigemAcao != OrigemGlobal {
		t.Fatalf("codex: %+v", c)
	}
}

func TestContextoLimiteZeroDoProjetoDesligaGlobal(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, ".openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 100}\n")
	gravar(t, filepath.Join(repo, WorkspaceDirName, ConfigFileName), "contexto:\n  padrao: {limite_tokens: 0}\n")
	c, err := ContextoEfetivo(repo, "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Ligado() || c.OrigemLimite != OrigemProjeto {
		t.Fatalf("%+v", c)
	}
}

func TestContextoHomeVenceConfigDoUsuario(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, ".config", "openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 10, acao: nova-sessao}\n")
	gravar(t, filepath.Join(home, ".openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 20}\n")
	c, err := ContextoEfetivo(repo, "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	if c.LimiteTokens != 20 || c.Acao != AcaoNovaSessao {
		t.Fatalf("~/.openheinerss deve vencer campo a campo: %+v", c)
	}
}

func TestContextoConfigDirSubstituiOsGlobaisDoHome(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, ".openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 20}\n")
	pasta := t.TempDir()
	gravar(t, filepath.Join(pasta, ConfigFileName), "contexto:\n  padrao: {limite_tokens: 30}\n")
	SetConfigDir(pasta)
	t.Cleanup(func() { SetConfigDir("") })
	c, err := ContextoEfetivo(repo, "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	if c.LimiteTokens != 30 {
		t.Fatalf("%+v", c)
	}
}

func TestContextoFlagsVencem(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, ".openheinerss", ConfigFileName), "contexto:\n  padrao: {limite_tokens: 100, acao: aviso}\n")
	c, _ := ContextoEfetivo(repo, "x", "x")
	zero := 0
	f, err := AplicarFlagsContexto(c, &zero, AcaoNovaSessao)
	if err != nil {
		t.Fatal(err)
	}
	if f.Ligado() || f.OrigemLimite != OrigemFlag || f.Acao != AcaoNovaSessao || f.OrigemAcao != OrigemFlag {
		t.Fatalf("%+v", f)
	}
	n := 7
	f, _ = AplicarFlagsContexto(c, &n, "")
	if f.LimiteTokens != 7 || f.Acao != AcaoAviso || f.OrigemAcao != OrigemGlobal {
		t.Fatalf("%+v", f)
	}
	if _, err := AplicarFlagsContexto(c, nil, "outra"); err == nil {
		t.Fatal("ação inválida deveria falhar")
	}
}

func TestContextoArquivoInvalidoDaErroClaro(t *testing.T) {
	_, repo := ambienteContexto(t)
	proj := filepath.Join(repo, WorkspaceDirName, ConfigFileName)
	for nome, conteudo := range map[string]string{
		"yaml quebrado":   "contexto: [",
		"acao inválida":   "contexto:\n  padrao: {acao: voar}\n",
		"limite texto":    "contexto:\n  padrao: {limite_tokens: muito}\n",
		"limite negativo": "contexto:\n  harnesses:\n    codex: {limite_tokens: -5}\n",
	} {
		gravar(t, proj, conteudo)
		_, err := ContextoEfetivo(repo, "codex", "codex")
		if err == nil || !strings.Contains(err.Error(), ConfigFileName) {
			t.Fatalf("%s: erro sem o nome do arquivo: %v", nome, err)
		}
	}
}
