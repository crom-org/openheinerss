package agy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// rodarAgyFalso executa um agy falso (script) e devolve o motivo do fim e as mensagens de erro.
func rodarAgyFalso(t *testing.T, corpo string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agy"), []byte("#!/bin/sh\n"+corpo), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := NewAGYHarness(harness.ModeCLI)
	if err := a.Start(context.Background(), harness.SessionConfig{SessionID: "f", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	var erros []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-a.Events():
			if p, ok := ev.Payload.(protocol.ErrorParams); ok {
				erros = append(erros, p.Message)
			}
			if p, ok := ev.Payload.(protocol.CompleteParams); ok {
				return p.Reason, erros
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}

func TestAgyTextoLivreComPermissionDeniedNaoFalha(t *testing.T) {
	for _, linha := range []string{
		"Corrigi permission denied no README",
		`{"type":"text","text":"Corrigi permission denied no README"}`,
		`{"type":"tool_result","status":"success","text":"permission denied (esperado no teste)"}`,
	} {
		reason, erros := rodarAgyFalso(t, "printf '%s\\n' '"+linha+"'\n")
		if reason != "completed" || len(erros) != 0 {
			t.Errorf("%q: reason=%s erros=%v", linha, reason, erros)
		}
	}
}

func TestAgyFalhaSoEmCanalDeErro(t *testing.T) {
	casos := map[string]string{
		"evento-de-erro": `printf '%s\n' '{"type":"error","message":"permission denied: tool required"}'`,
		"stderr":         `echo 'Error: no output produced' >&2`,
		"linha-inicial":  `echo 'tool required but unavailable'`,
	}
	for nome, corpo := range casos {
		reason, erros := rodarAgyFalso(t, corpo+"\n")
		if reason != "process_error" || len(erros) == 0 {
			t.Errorf("%s: reason=%s erros=%v", nome, reason, erros)
		}
	}
}
