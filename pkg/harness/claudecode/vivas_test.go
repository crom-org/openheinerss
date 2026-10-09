package claudecode

import (
	"errors"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

type stdinFalso struct {
	strings.Builder
	fechado bool
}

func (s *stdinFalso) Close() error { s.fechado = true; return nil }

func eventosDe(c *ClaudeCodeHarness) (completes int) {
	for len(c.events) > 0 {
		if (<-c.events).Type == harness.EventComplete {
			completes++
		}
	}
	return
}

func TestArgsVivosUsamStreamJSONSemPromptNoArgv(t *testing.T) {
	cfg := harness.SessionConfig{Options: map[string]interface{}{harness.OptionMensagensVivas: true}}
	args := buildCLIArgs(cfg, "", "", "texto secreto")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--input-format stream-json") || !strings.Contains(joined, "--replay-user-messages") || strings.Contains(joined, "texto secreto") {
		t.Fatalf("args vivos: %v", args)
	}
	if got := buildCLIArgs(harness.SessionConfig{}, "", "", "oi"); got[len(got)-1] != "oi" || strings.Contains(strings.Join(got, " "), "input-format") {
		t.Fatalf("sem vivas o prompt segue no argv: %v", got)
	}
}

func TestEnviarVivoSemTurnoDevolveErro(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	if err := c.EnviarVivo("m", "oi"); !errors.Is(err, harness.ErrSemTurnoVivo) {
		t.Fatalf("erro: %v", err)
	}
}

func TestResultadoComMensagemVivaNaFilaNaoEncerraOTurno(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	in := &stdinFalso{}
	c.vivoStdin, c.vivasPend = in, map[string]bool{}
	if err := c.EnviarVivo("m", "recado"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.String(), `"recado"`) || !strings.Contains(in.String(), `"uuid"`) {
		t.Fatalf("stdin: %q", in.String())
	}
	var uuid string
	for u := range c.vivasPend {
		uuid = u
	}
	turn := &cliTurn{}
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"success","session_id":"s"}`), "f", turn)
	if eventosDe(c) != 0 || in.fechado || !turn.suprimido {
		t.Fatal("o resultado com recado ainda na fila não pode fechar o turno nem o stdin")
	}
	c.parseCLIEvent([]byte(`{"type":"user","uuid":"`+uuid+`","isReplay":true,"message":{"role":"user","content":"recado"},"session_id":"s"}`), "f", turn)
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"success","session_id":"s"}`), "f", turn)
	if eventosDe(c) != 1 || !in.fechado {
		t.Fatal("o segundo resultado fecha o turno e o stdin (o claude sai sozinho)")
	}
	if err := c.EnviarVivo("m2", "tarde"); !errors.Is(err, harness.ErrSemTurnoVivo) {
		t.Fatalf("depois do resultado a entrega vira retomada: %v", err)
	}
}
