package session_test

import (
	"context"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

func TestSessionManagerLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m := session.NewManager()

	// 1. Criar sessão
	res, err := m.CreateSession(ctx, protocol.SessionCreateParams{
		Harness: "mock",
		CWD:     "/tmp",
	})
	if err != nil {
		t.Fatalf("falha ao criar sessão: %v", err)
	}
	if res.SessionID == "" {
		t.Fatalf("SessionID vazia")
	}

	// 2. Verificar listagem
	list := m.ListSessions()
	if len(list) != 1 || list[0].SessionID != res.SessionID {
		t.Fatalf("sessão criada não aparece na listagem: %+v", list)
	}

	doneChan := make(chan bool, 1)

	// 3. Ouvir eventos
	m.SubscribeEvents(func(notification protocol.Notification) {
		if notification.Method == protocol.EventAgentPermissionRequest {
			p, ok := notification.Params.(protocol.PermissionRequestParams)
			if ok {
				go func() {
					_, _ = m.RespondPermission(ctx, protocol.PermissionRespondParams{
						SessionID: p.SessionID,
						RequestID: p.RequestID,
						Decision:  "allow",
					})
				}()
			}
		}

		if notification.Method == protocol.EventAgentComplete {
			select {
			case doneChan <- true:
			default:
			}
		}
	})

	// 4. Enviar prompt
	promptRes, err := m.PromptSession(ctx, protocol.SessionPromptParams{
		SessionID: res.SessionID,
		Text:      "Olá mock agent",
	})
	if err != nil {
		t.Fatalf("erro ao enviar prompt: %v", err)
	}
	if !promptRes.Accepted {
		t.Fatalf("prompt não foi aceito")
	}

	select {
	case <-ctx.Done():
		t.Fatalf("timeout aguardando conclusão do ciclo da sessão")
	case <-doneChan:
	}

	// 5. Testar Abort
	abortRes, err := m.AbortSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("erro ao abortar sessão: %v", err)
	}
	if !abortRes.Aborted {
		t.Fatalf("sessão não foi abortada")
	}
}
