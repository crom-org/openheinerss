package mock_test

import (
	"context"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/mock"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestMockHarnessAllow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	h := mock.NewMockHarness()
	h.SetStepDelay(1 * time.Millisecond)

	cfg := harness.SessionConfig{
		SessionID: "test_sess_allow",
		CWD:       "/tmp",
	}

	if err := h.Start(ctx, cfg); err != nil {
		t.Fatalf("falha ao iniciar mock harness: %v", err)
	}
	defer h.Stop()

	if err := h.SendPrompt(ctx, "teste de permissao permitida", nil); err != nil {
		t.Fatalf("falha ao enviar prompt: %v", err)
	}

	eventsReceived := make([]harness.EventType, 0)
	permHandled := false

loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout aguardando ciclo do mock harness. Eventos recebidos: %v", eventsReceived)
		case evt := <-h.Events():
			eventsReceived = append(eventsReceived, evt.Type)

			if evt.Type == harness.EventPermission && !permHandled {
				permHandled = true
				req := evt.Payload.(protocol.PermissionRequestParams)
				if err := h.RespondPermission(ctx, req.RequestID, true, "autorizado"); err != nil {
					t.Fatalf("erro ao responder permissao: %v", err)
				}
			}

			if evt.Type == harness.EventComplete {
				break loop
			}
		}
	}

	// Verifica se passou por todos os eventos esperados
	expected := []harness.EventType{
		harness.EventThinking,
		harness.EventText,
		harness.EventPermission,
		harness.EventToolCall,
		harness.EventToolResult,
		harness.EventText,
		harness.EventComplete,
	}

	if len(eventsReceived) != len(expected) {
		t.Fatalf("esperava %d eventos, recebeu %d: %v", len(expected), len(eventsReceived), eventsReceived)
	}
}

func TestMockHarnessDeny(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	h := mock.NewMockHarness()
	h.SetStepDelay(1 * time.Millisecond)

	cfg := harness.SessionConfig{
		SessionID: "test_sess_deny",
		CWD:       "/tmp",
	}

	if err := h.Start(ctx, cfg); err != nil {
		t.Fatalf("falha ao iniciar mock harness: %v", err)
	}
	defer h.Stop()

	if err := h.SendPrompt(ctx, "teste de permissao negada", nil); err != nil {
		t.Fatalf("falha ao enviar prompt: %v", err)
	}

	permHandled := false
loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout aguardando ciclo de negação")
		case evt := <-h.Events():
			if evt.Type == harness.EventPermission && !permHandled {
				permHandled = true
				req := evt.Payload.(protocol.PermissionRequestParams)
				if err := h.RespondPermission(ctx, req.RequestID, false, "negado pelo teste"); err != nil {
					t.Fatalf("erro ao responder negação de permissao: %v", err)
				}
			}

			if evt.Type == harness.EventComplete {
				c := evt.Payload.(protocol.CompleteParams)
				if c.Reason != "permission_denied" {
					t.Errorf("esperava reason 'permission_denied', obteve '%s'", c.Reason)
				}
				break loop
			}
		}
	}
}

func TestMockPonteGravaHarnessArgsEComandoLiteral(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h := mock.NewMockHarness()
	h.SetStepDelay(time.Millisecond)
	cfg := harness.SessionConfig{SessionID: "p1", Options: map[string]interface{}{harness.OptionHarnessArgs: []interface{}{"--a", "b c"}}}
	if err := h.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := h.SendPrompt(ctx, "/x arg", nil); err != nil {
		t.Fatal(err)
	}
	var raws []string
	var text string
	for done := false; !done; {
		select {
		case e := <-h.Events():
			if r, ok := e.Payload.(protocol.RawParams); ok && e.Type == harness.EventRaw {
				raws = append(raws, r.Line)
			}
			if p, ok := e.Payload.(protocol.TextParams); ok {
				text = p.Delta
			}
			done = e.Type == harness.EventComplete
		case <-ctx.Done():
			t.Fatal("timeout")
		}
	}
	if len(raws) != 2 || raws[0] != `harness_args=["--a","b c"]` || raws[1] != "comando /x recebido" || text != "comando /x recebido" {
		t.Fatalf("raws=%q text=%q", raws, text)
	}
	if got := h.ReceivedArgs(); len(got) != 2 || got[1] != "b c" {
		t.Fatalf("args: %q", got)
	}
	if p := h.ReceivedPrompts(); len(p) != 1 || p[0] != "/x arg" {
		t.Fatalf("prompts: %q", p)
	}
}
