package claudecode

import (
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestRateLimitPermitidoComOverageDesligadoNaoECota(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	c.parseCLIEvent([]byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","overageStatus":"rejected","overageDisabledReason":"org_level_disabled"}}`), "s", &cliTurn{})
	select {
	case ev := <-c.Events():
		t.Fatalf("evento inesperado: %+v", ev)
	default:
	}
}
