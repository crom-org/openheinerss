package harness

import "strings"

// Rótulos devolvidos por ClassifyFailure.
const (
	FailureNoCLI   = "sem CLI"
	FailureNoLogin = "sem login"
	FailureNoQuota = "sem cota"
	FailureNoModel = "sem modelo"
	FailureOther   = "falha"
)

// ClassifyFailure separa as falhas conhecidas dos motores (CLI ausente, sem login,
// sem cota, sem modelo) das demais e devolve a correção sugerida quando há uma.
func ClassifyFailure(message string) (kind, suggestedFix string) {
	lower := strings.ToLower(message)
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(lower, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("sem modelo", "no llm model was specified"):
		return FailureNoModel, "Informe um modelo (--model / campo model da instância) e a chave de API do provedor correspondente."
	case has("executable file not found", "command not found", "não está no path"):
		return FailureNoCLI, "Instale o CLI do motor, confirme que está no PATH e rode 'openheinerss doctor'."
	case has("quota", "cota", "rate limit", "rate_limit", "more credits", "insufficient_quota", "usage limit"):
		return FailureNoQuota, "Aguarde a janela de cota renovar, adicione créditos ao provedor ou troque de motor/conta ('openheinerss limites' mostra as janelas)."
	case has("login", "auth", "unauthorized", "não autentic", "not authenticated", "api key", "invalid x-api-key"):
		return FailureNoLogin, "Faça login no CLI do motor (ex.: 'claude /login', 'codex login') ou configure a chave de API."
	}
	return FailureOther, ""
}
