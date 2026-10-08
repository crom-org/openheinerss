package comandos

import (
	"strings"
	"unicode/utf8"
)

// Tetos das colunas de `comandos <harness>`: um nome maior que isso só empurra a própria linha.
const (
	tetoNome    = 40
	tetoRepasse = 16
)

// Linhas formata a lista em colunas alinhadas pela largura de exibição (runes, não bytes):
// a coluna do nome tem a largura do maior nome, até tetoNome.
func Linhas(cs []Comando) []string {
	larguraNome, larguraRepasse := 0, 0
	for _, c := range cs {
		larguraNome = max(larguraNome, min(utf8.RuneCountInString(c.Nome), tetoNome))
		larguraRepasse = max(larguraRepasse, min(utf8.RuneCountInString(c.Repasse), tetoRepasse))
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		marca := " "
		if c.Confirmado {
			marca = "✓"
		}
		linha := marca + " " + preencher(c.Nome, larguraNome) + " " + preencher(c.Repasse, larguraRepasse) + " " + c.Descricao
		if c.Anotacao != "" {
			linha += " — nota: " + c.Anotacao
		}
		out = append(out, strings.TrimRight(linha, " "))
	}
	return out
}

func preencher(s string, largura int) string {
	if n := utf8.RuneCountInString(s); n < largura {
		return s + strings.Repeat(" ", largura-n)
	}
	return s
}
