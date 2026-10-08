package comandos

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLinhasAlinhaPeloMaiorNome(t *testing.T) {
	cs := []Comando{
		{Nome: "/compact", Repasse: "literal", Descricao: "compacta"},
		{Nome: "/crom-tv-agentes-externos", Repasse: "literal", Descricao: "skill"},
		{Nome: "/ação", Repasse: "sem_equivalente", Descricao: "acento", Confirmado: true, Anotacao: "ok"},
	}
	linhas := Linhas(cs)
	col := -1
	for _, l := range linhas {
		i := strings.Index(l, "literal")
		if i < 0 {
			i = strings.Index(l, "sem_equivalente")
		}
		// posição em runes: "/ação" e "✓" têm mais bytes que runes
		pos := utf8.RuneCountInString(l[:i])
		if col >= 0 && pos != col {
			t.Fatalf("coluna de repasse desalinhada (%d != %d):\n%s", pos, col, strings.Join(linhas, "\n"))
		}
		col = pos
	}
	if col != 2+len("/crom-tv-agentes-externos")+1 {
		t.Fatalf("coluna deveria seguir o maior nome, veio %d", col)
	}
	if !strings.HasSuffix(linhas[2], "acento — nota: ok") || !strings.HasPrefix(linhas[2], "✓ ") {
		t.Fatalf("linha com anotação: %q", linhas[2])
	}
}

func TestLinhasTetoDoNome(t *testing.T) {
	longo := "/" + strings.Repeat("x", 60)
	linhas := Linhas([]Comando{{Nome: "/a", Repasse: "literal"}, {Nome: longo, Repasse: "literal"}})
	if got := strings.Index(linhas[0], "literal"); got != 2+tetoNome+1 {
		t.Fatalf("nome longo deveria parar no teto, coluna %d", got)
	}
	if !strings.Contains(linhas[1], longo+" literal") {
		t.Fatalf("nome acima do teto empurra só a própria linha: %q", linhas[1])
	}
}
