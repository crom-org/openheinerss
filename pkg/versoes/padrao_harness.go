package versoes

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// ligarHarness conecta o ambiente real ao catálogo de harnesses (instâncias do usuário).
func ligarHarness(e *Ambiente) {
	e.BaseDe = BaseDe
	e.InstanciaGratis = InstanciaGratis
	e.Testar = func(ctx context.Context, instancia string, timeout time.Duration) (bool, string) {
		r := TesteReal(ctx, instancia, harness.ModeCLI, "responda só OK", timeout)
		return r.Resultado == "OK", r.Resultado + ": " + r.Motivo
	}
}

// BaseDe segue a herança da instância até o harness embutido; "" se o nome é desconhecido.
func BaseDe(nome string) string {
	for i := 0; i < 10; i++ {
		spec, ok := harness.CustomSpecFor(nome)
		if !ok || spec.Base == "" {
			break
		}
		nome = spec.Base
	}
	if _, ok := specs[harness.CanonicalName(nome)]; ok {
		return harness.CanonicalName(nome)
	}
	return ""
}

// InstanciaGratis acha uma instância cujo nome indica uso sem custo (contém "gratis", "grátis" ou
// "free") e cuja base é a pedida. Instâncias pagas/de conta nunca são escolhidas para teste.
func InstanciaGratis(base string) string {
	var achados []string
	for _, item := range harness.ListCatalog() {
		if _, ok := harness.CustomSpecFor(item.ID); !ok {
			continue
		}
		n := strings.ToLower(item.ID)
		if (strings.Contains(n, "gratis") || strings.Contains(n, "grátis") || strings.Contains(n, "free")) && BaseDe(item.ID) == base {
			achados = append(achados, item.ID)
		}
	}
	sort.Strings(achados)
	if len(achados) == 0 {
		return ""
	}
	return achados[0]
}
