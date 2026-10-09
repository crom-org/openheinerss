package capacidades

import (
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
)

func TestTodasAsBasesTemFichaCompleta(t *testing.T) {
	validos := map[string]bool{Sim: true, Nao: true, NaoConfirmado: true}
	todas := Todas()
	if len(todas) != 5 {
		t.Fatalf("bases: %d", len(todas))
	}
	for _, c := range todas {
		if c.ConferidoCom == "" || len(c.Instrucoes) == 0 {
			t.Errorf("%s sem versão/instruções", c.Base)
		}
		itens := append(append([]Item{c.ImportaArquivo, c.AceitaSkills, c.MCP, c.Retomar, c.Permissoes}, c.Instrucoes...), c.Skills...)
		for _, it := range itens {
			if !validos[it.Estado] || it.Nome == "" {
				t.Errorf("%s: célula inválida %+v", c.Base, it)
			}
			// Toda célula afirmativa ou negativa precisa de fonte; só "não confirmado" pode ficar sem.
			if it.Estado != NaoConfirmado && it.Fonte == "" {
				t.Errorf("%s: %q sem fonte", c.Base, it.Nome)
			}
		}
	}
}

func TestPerguntasDaCentral(t *testing.T) {
	achar := func(c Capacidades, arq string) string {
		for _, it := range c.Instrucoes {
			if it.Nome == arq {
				return it.Estado
			}
		}
		return ""
	}
	cc, _ := Para("claude-code")
	cx, _ := Para("codex")
	oc, _ := Para("opencode")
	ag, _ := Para("agy")
	ai, _ := Para("aider")
	if achar(cc, "CLAUDE.md") != Sim || achar(cx, "AGENTS.md") != Sim || achar(ag, "GEMINI.md") != Sim || achar(oc, "AGENTS.md") != Sim {
		t.Fatal("arquivos de instrução principais")
	}
	if achar(cc, "AGENTS.md") == Sim || achar(cx, "CLAUDE.md") == Sim {
		t.Fatal("claude não lê AGENTS.md sozinho; codex não lê CLAUDE.md")
	}
	if ai.MCP.Estado != Nao || ai.AceitaSkills.Estado != Nao || ai.Retomar.Estado == Sim {
		t.Fatalf("aider: %+v", ai)
	}
	for _, c := range []Capacidades{cc, cx, oc, ag} {
		if c.Retomar.Estado != Sim {
			t.Errorf("%s deveria retomar: %+v", c.Base, c.Retomar)
		}
	}
}

func TestInstanciaHerdaDaBase(t *testing.T) {
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cap-conta2", Base: "claude-code", Env: map[string]string{"CLAUDE_CONFIG_DIR": t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	c, err := Para("cap-conta2")
	if err != nil {
		t.Fatal(err)
	}
	if c.Base != "claude-code" || c.Harness != "cap-conta2" || len(c.Cadeia) != 2 || c.ContaDir == "" {
		t.Fatalf("%+v", c)
	}
}

func TestHarnessPorComandoNaoInventaNada(t *testing.T) {
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "cap-cmd", Command: "/bin/true"}); err != nil {
		t.Fatal(err)
	}
	c, err := Para("cap-cmd")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range []Item{c.ImportaArquivo, c.AceitaSkills, c.MCP, c.Retomar, c.Permissoes} {
		if it.Estado != NaoConfirmado {
			t.Errorf("%+v", it)
		}
	}
	if _, err := Para("nao-existe-e7"); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestLinhasMostraEstadoEFonte(t *testing.T) {
	c, _ := Para("codex")
	txt := strings.Join(Linhas(c), "\n")
	for _, parte := range []string{"AGENTS.md", "codex exec resume", "ajuda do CLI", "nao_confirmado"} {
		if !strings.Contains(txt, parte) {
			t.Errorf("faltou %q em:\n%s", parte, txt)
		}
	}
}
