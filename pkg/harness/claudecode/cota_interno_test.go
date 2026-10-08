package claudecode

import (
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestRateLimitPermitidoComOverageDesligadoNaoECota(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	c.parseCLIEvent([]byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","overageStatus":"rejected","overageDisabledReason":"org_level_disabled"}}`), "s", &cliTurn{})
	select {
	case ev := <-c.Events():
		if ev.Type != harness.EventRaw {
			t.Fatalf("evento inesperado: %+v", ev)
		}
		select {
		case ev := <-c.Events():
			t.Fatalf("evento inesperado depois do raw: %+v", ev)
		default:
		}
	default:
	}
}

func tiposDoTurno(c *ClaudeCodeHarness) (erros []string, motivo string) {
	for {
		select {
		case ev := <-c.Events():
			if p, ok := ev.Payload.(protocol.ErrorParams); ok && ev.Type == harness.EventError {
				erros = append(erros, p.Message)
			}
			if p, ok := ev.Payload.(protocol.CompleteParams); ok && ev.Type == harness.EventComplete {
				motivo = p.Reason
			}
		default:
			return erros, motivo
		}
	}
}

// Caso real do ponte-a: o resultado final bem-sucedido citava "rate_limit_event" e "cota" no texto.
func TestResultadoComSucessoQueCitaCotaNaoECota(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	turn := &cliTurn{}
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"Viram raw as linhas rate_limit_event; limite de cota e usage limit documentados","session_id":"s","usage":{"input_tokens":1,"output_tokens":2}}`), "s", turn)
	erros, motivo := tiposDoTurno(c)
	if len(erros) != 0 || motivo != "success" {
		t.Fatalf("sucesso virou erro: erros=%v motivo=%q", erros, motivo)
	}
}

func TestRateLimitRejeitadoSeguidoDeResultadoBomNaoECota(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	turn := &cliTurn{}
	c.parseCLIEvent([]byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}`), "s", turn)
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"pronto","session_id":"s"}`), "s", turn)
	if erros, motivo := tiposDoTurno(c); len(erros) != 0 || motivo != "success" {
		t.Fatalf("aviso de cota com resultado bom virou erro: erros=%v motivo=%q", erros, motivo)
	}
}

func TestRateLimitRejeitadoComResultadoDeErroECota(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	turn := &cliTurn{}
	c.parseCLIEvent([]byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}`), "s", turn)
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"falhou","session_id":"s"}`), "s", turn)
	erros, motivo := tiposDoTurno(c)
	if len(erros) == 0 || erros[0] != "limite de cota do Claude Code atingido" || motivo != "process_error" {
		t.Fatalf("cota não detectada: erros=%v motivo=%q", erros, motivo)
	}
}
