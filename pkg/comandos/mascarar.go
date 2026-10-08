package comandos

import "regexp"

// Mascarar troca segredos de um texto livre por *** (o mesmo marcador que o --seco usa para
// valores de ambiente). Pega NOME=valor / NOME: valor quando o nome parece credencial,
// "Bearer xxx" e prefixos conhecidos de chaves (sk-, ghp_, github_pat_, glpat-, xox?-, AKIA…).
func Mascarar(s string) string {
	s = reAtribuicao.ReplaceAllString(s, "${1}${2}***")
	s = reBearer.ReplaceAllString(s, "${1}***")
	return reChave.ReplaceAllString(s, "***")
}

var (
	reAtribuicao = regexp.MustCompile(`(?i)(\b[A-Za-z0-9_.-]*(?:token|secret|segredo|senha|passw(?:or)?d|api[_-]?key|access[_-]?key|private[_-]?key|chave|authorization|credential|cookie)[A-Za-z0-9_.-]*)(\s*[=:]\s*["']?)[^\s"']+`)
	reBearer     = regexp.MustCompile(`(?i)(\b(?:bearer|basic)\s+)[A-Za-z0-9._~+/=-]{8,}`)
	reChave      = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{16,}|xox[abposr]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,})`)
)
