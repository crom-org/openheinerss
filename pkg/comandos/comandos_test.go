package comandos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

// ambiente isola HOME e a pasta de config do usuário numa pasta temporária.
func ambiente(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv(config.EnvConfigDir, "")
	config.SetConfigDir("")
	t.Cleanup(func() { config.SetConfigDir("") })
	return tmp
}

func achar(t *testing.T, l Lista, nome string) Comando {
	t.Helper()
	for _, c := range l.Comandos {
		if c.Nome == nome {
			return c
		}
	}
	t.Fatalf("%s não está na lista de %s", nome, l.Harness)
	return Comando{}
}

func TestCatalogoEmbutidoPorBase(t *testing.T) {
	ambiente(t)
	for _, base := range []string{"claude-code", "codex", "opencode", "aider", "agy"} {
		if len(embutido[base]) == 0 {
			t.Fatalf("catálogo vazio para %s", base)
		}
		for _, it := range embutido[base] {
			if _, err := NormalizarNome(it.nome); err != nil || it.descricao == "" {
				t.Fatalf("%s %s: item inválido (%v)", base, it.nome, err)
			}
			switch it.repasse {
			case RepasseLiteral, RepasseTraduzido, RepasseSemEquivalente:
			default:
				t.Fatalf("%s %s: repasse %q", base, it.nome, it.repasse)
			}
		}
	}
	l, err := Listar("claude-code", "")
	if err != nil {
		t.Fatal(err)
	}
	if c := achar(t, l, "/compact"); c.Repasse != RepasseLiteral || c.Origem != OrigemEmbutido || c.Confirmado {
		t.Fatalf("/compact claude: %+v", c)
	}
	if c := achar(t, l, "/model"); c.Repasse != RepasseTraduzido {
		t.Fatalf("/model claude: %+v", c)
	}
	lc, err := Listar("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	if c := achar(t, lc, "/compact"); c.Repasse != RepasseSemEquivalente {
		t.Fatalf("/compact codex: %+v", c)
	}
	if lc.Desconhecido != RepasseSemEquivalente || l.Desconhecido != RepasseLiteral {
		t.Fatalf("desconhecido: codex=%s claude=%s", lc.Desconhecido, l.Desconhecido)
	}
	if _, err := Listar("nao-existe", ""); err == nil {
		t.Fatal("harness inexistente deveria falhar")
	}
}

func TestInstanciaHerdaDaBaseEAnotacoesMesclam(t *testing.T) {
	tmp := ambiente(t)
	conta := filepath.Join(tmp, ".claude-conta2")
	if err := os.MkdirAll(filepath.Join(conta, "commands", "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(conta, "commands", "git", "pr.md"), []byte("---\ndescription: Abre um PR\n---\ncorpo"), 0o644)
	_ = os.MkdirAll(filepath.Join(conta, "skills", "dataviz"), 0o755)
	_ = os.WriteFile(filepath.Join(conta, "skills", "dataviz", "SKILL.md"), []byte("---\nname: dataviz\ndescription: \"Gráficos\"\n---\n"), 0o644)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cmd-teste-conta2", Base: "claude-code", Env: map[string]string{"CLAUDE_CONFIG_DIR": "~/.claude-conta2"}}); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cmd-teste-neta", Base: "cmd-teste-conta2"}); err != nil {
		t.Fatal(err)
	}

	// Anotação na base vale para a instância; a da instância vence.
	if _, err := Anotar("claude-code", "/compact", "compacta o claude code; o central não usa", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Anotar("cmd-teste-conta2", "compact", "só na conta2", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirmar("claude-code", "/clear", ""); err != nil {
		t.Fatal(err)
	}
	l, err := Listar("cmd-teste-neta", "")
	if err != nil {
		t.Fatal(err)
	}
	if l.Base != "claude-code" || strings.Join(l.Cadeia, ">") != "claude-code>cmd-teste-conta2>cmd-teste-neta" {
		t.Fatalf("herança: base=%s cadeia=%v", l.Base, l.Cadeia)
	}
	if c := achar(t, l, "/compact"); c.Anotacao != "só na conta2" || c.Confirmado {
		t.Fatalf("/compact: %+v", c)
	}
	if c := achar(t, l, "/clear"); !c.Confirmado {
		t.Fatalf("/clear deveria vir confirmado da base: %+v", c)
	}
	if c := achar(t, l, "/git:pr"); c.Origem != OrigemDescoberto || c.Descricao != "Abre um PR" {
		t.Fatalf("/git:pr: %+v", c)
	}
	if c := achar(t, l, "/dataviz"); c.Descricao != "Gráficos" {
		t.Fatalf("/dataviz: %+v", c)
	}
	// A base pura não enxerga a anotação da instância nem os comandos da conta2.
	lb, _ := Listar("claude-code", "")
	if c := achar(t, lb, "/compact"); c.Anotacao != "compacta o claude code; o central não usa" {
		t.Fatalf("base: %+v", c)
	}
	for _, c := range lb.Comandos {
		if c.Nome == "/git:pr" {
			t.Fatal("base não deveria ver comandos da conta2")
		}
	}
	// Comando só do usuário entra com o repasse padrão do harness.
	if _, err := Anotar("codex", "/meu", "x", ""); err != nil {
		t.Fatal(err)
	}
	lc, _ := Listar("codex", "")
	if c := achar(t, lc, "/meu"); c.Origem != OrigemUsuario || c.Repasse != RepasseSemEquivalente {
		t.Fatalf("/meu: %+v", c)
	}
	if _, err := Anotar("codex", "/home/x y", "x", ""); err == nil {
		t.Fatal("caminho com argumento não é comando")
	}
}

func TestConfirmarGravaPreservandoComentarioEConfig(t *testing.T) {
	tmp := ambiente(t)
	global := filepath.Join(tmp, ".config", "openheinerss", NomeArquivo)
	if ArquivoGlobal() != global {
		t.Fatalf("global = %s", ArquivoGlobal())
	}
	if _, err := Confirmar("codex", "/model", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(global)
	if !strings.Contains(string(b), "confirmado: true") {
		t.Fatalf("global não gravou:\n%s", b)
	}

	// Com --config, grava lá; o global continua sendo lido por baixo.
	cfg := filepath.Join(tmp, "cfg")
	_ = os.MkdirAll(cfg, 0o755)
	_ = os.WriteFile(filepath.Join(cfg, NomeArquivo), []byte("# meu comentário\ncodex:\n  \"/new\":\n    anotacao: \"zera\"\n"), 0o644)
	config.SetConfigDir(cfg)
	l, err := Listar("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	if l.Arquivo != filepath.Join(cfg, NomeArquivo) {
		t.Fatalf("arquivo = %s", l.Arquivo)
	}
	if c := achar(t, l, "/model"); !c.Confirmado {
		t.Fatal("confirmação do global sumiu com --config")
	}
	c, err := Confirmar("codex", "/new", "")
	if err != nil || !c.Confirmado || c.Anotacao != "zera" {
		t.Fatalf("confirmar /new: %+v %v", c, err)
	}
	b, _ = os.ReadFile(filepath.Join(cfg, NomeArquivo))
	if !strings.Contains(string(b), "# meu comentário") || !strings.Contains(string(b), "confirmado: true") {
		t.Fatalf("config perdeu comentário ou não gravou:\n%s", b)
	}
	// OPENHEINERSS_CONFIG também vale.
	config.SetConfigDir("")
	t.Setenv(config.EnvConfigDir, cfg)
	l, _ = Listar("codex", "")
	if c := achar(t, l, "/new"); !c.Confirmado {
		t.Fatal("OPENHEINERSS_CONFIG não foi lido")
	}
}
