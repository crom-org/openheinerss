package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

// riscoDaPermissao roda o mock e devolve a permission_request e o tool_call vistos.
func riscoDaPermissao(t *testing.T, m *session.Manager, cwd string, pedido *bool) (protocol.PermissionRequestParams, protocol.ToolCallParams) {
	t.Helper()
	ctx := context.Background()
	res, err := m.CreateSession(ctx, protocol.SessionCreateParams{Harness: "mock", CWD: cwd, Options: protocol.SessionOptions{ClassificarRisco: pedido}})
	if err != nil {
		t.Fatal(err)
	}
	perm := make(chan protocol.PermissionRequestParams, 1)
	tool := make(chan protocol.ToolCallParams, 1)
	m.SubscribeEvents(func(n protocol.Notification) {
		switch p := n.Params.(type) {
		case protocol.PermissionRequestParams:
			if p.SessionID == res.SessionID {
				perm <- p
				go func() {
					_, _ = m.RespondPermission(ctx, protocol.PermissionRespondParams{SessionID: p.SessionID, RequestID: p.RequestID, Decision: "allow"})
				}()
			}
		case protocol.ToolCallParams:
			if p.SessionID == res.SessionID {
				select {
				case tool <- p:
				default:
				}
			}
		}
	})
	if _, err := m.PromptSession(ctx, protocol.SessionPromptParams{SessionID: res.SessionID, Text: "oi"}); err != nil {
		t.Fatal(err)
	}
	var p protocol.PermissionRequestParams
	var tc protocol.ToolCallParams
	for i := 0; i < 2; i++ {
		select {
		case p = <-perm:
		case tc = <-tool:
		case <-time.After(20 * time.Second):
			t.Fatal("eventos do mock não chegaram")
		}
	}
	return p, tc
}

func TestRiscoDesligadoPorPadrao(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := session.NewManager()
	defer m.Close()
	p, tc := riscoDaPermissao(t, m, t.TempDir(), nil)
	if p.Risco != "" || tc.Risco != "" {
		t.Fatalf("a ponte deveria ser só túnel: %+v %+v", p, tc)
	}
}

func TestRiscoLigadoNaSessaoENoServidor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cwd := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cwd, ".openheinerss"), 0o755)
	_ = os.WriteFile(filepath.Join(cwd, ".openheinerss", "risco.yaml"), []byte("regras:\n  - nivel: baixo\n    motivo: status é leitura\n    padrao: 'git status'\n"), 0o644)
	on := true
	m := session.NewManager()
	defer m.Close()
	p, tc := riscoDaPermissao(t, m, cwd, &on)
	if tc.Risco != "baixo" {
		t.Fatalf("tool_call sem risco: %+v", tc)
	}
	if p.Risco != "baixo" || p.MotivoRisco != "status é leitura" || p.Risk != "medium" {
		t.Fatalf("permissão sem risco classificado (ou perdeu o risk do harness): %+v", p)
	}

	m2 := session.NewManager()
	defer m2.Close()
	m2.SetClassificarRisco(true)
	p, _ = riscoDaPermissao(t, m2, cwd, nil)
	if p.Risco != "baixo" {
		t.Fatalf("--classificar-risco do servidor não ligou: %+v", p)
	}
	off := false
	p, _ = riscoDaPermissao(t, m2, cwd, &off)
	if p.Risco != "" {
		t.Fatalf("a sessão deveria poder desligar: %+v", p)
	}
}
