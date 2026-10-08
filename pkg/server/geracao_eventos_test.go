package server_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
	"github.com/gorilla/websocket"
)

// todosEventos emite um evento de cada tipo a cada prompt.
type todosEventos struct {
	sid string
	ch  chan harness.Event
}

func (e *todosEventos) Name() string       { return "todos-eventos" }
func (e *todosEventos) Mode() harness.Mode { return harness.ModeCLI }
func (e *todosEventos) ValidatePrerequisites(context.Context) harness.PrerequisiteResult {
	return harness.PrerequisiteResult{Satisfied: true}
}
func (e *todosEventos) Start(_ context.Context, cfg harness.SessionConfig) error {
	e.sid, e.ch = cfg.SessionID, make(chan harness.Event, 16)
	return nil
}
func (e *todosEventos) SendPrompt(context.Context, string, []protocol.Attachment) error {
	s := e.sid
	for _, ev := range []harness.Event{
		{Type: harness.EventThinking, Payload: protocol.ThinkingParams{SessionID: s, Delta: "pensa"}},
		{Type: harness.EventText, Payload: protocol.TextParams{SessionID: s, Delta: "oi"}},
		{Type: harness.EventToolCall, Payload: protocol.ToolCallParams{SessionID: s, CallID: "c1", Tool: "shell"}},
		{Type: harness.EventToolResult, Payload: protocol.ToolResultParams{SessionID: s, CallID: "c1", Status: "success"}},
		{Type: harness.EventPermission, Payload: protocol.PermissionRequestParams{SessionID: s, RequestID: "r1", Tool: "shell"}},
		{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: s, InputTokens: 1}},
		harness.RawEvent(s, "todos-eventos", "stdout", "linha crua"),
		{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: s, Message: "falha"}},
		{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: s, Reason: "completed"}},
	} {
		e.ch <- ev
	}
	return nil
}
func (e *todosEventos) RespondPermission(context.Context, string, bool, string) error { return nil }
func (e *todosEventos) Events() <-chan harness.Event                                  { return e.ch }
func (e *todosEventos) Stop() error                                                   { return nil }

// A Central pediu `geracao` em TODOS os eventos do serve, não só em respostas e orq.*.
func TestGeracaoEmTodosOsEventosAgent(t *testing.T) {
	harness.Register("todos-eventos", protocol.HarnessCatalogItem{ID: "todos-eventos"}, func(harness.Mode) (harness.Harness, error) { return &todosEventos{}, nil })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := server.NewWSServer(session.NewManager())
	go func() { _ = s.ListenAndServe(addr) }()
	defer s.Shutdown(context.Background())
	var conn *websocket.Conn
	for i := 0; i < 50; i++ {
		if conn, _, err = websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	send := func(id int, method string, params interface{}) {
		b, _ := json.Marshal(protocol.Request{JSONRPC: "2.0", ID: id, Method: method, Params: mustJSON(params)})
		if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
			t.Fatal(err)
		}
	}
	send(1, protocol.MethodSessionCreate, protocol.SessionCreateParams{Harness: "todos-eventos", CWD: t.TempDir()})
	var resp struct {
		Geracao string `json:"geracao"`
		Result  struct {
			SessionID string `json:"sessionId"`
		} `json:"result"`
		Error interface{} `json:"error"`
	}
	if err := conn.ReadJSON(&resp); err != nil || resp.Error != nil || resp.Geracao == "" {
		t.Fatalf("session.create: %v %+v", err, resp)
	}
	send(2, protocol.MethodSessionPrompt, protocol.SessionPromptParams{SessionID: resp.Result.SessionID, Text: "/x"})
	esperados := []string{protocol.EventAgentThinking, protocol.EventAgentText, protocol.EventAgentToolCall, protocol.EventAgentToolResult,
		protocol.EventAgentPermissionRequest, protocol.EventAgentUsage, protocol.EventAgentRaw, protocol.EventAgentError, protocol.EventAgentComplete}
	vistos := map[string]bool{}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for len(vistos) < len(esperados) {
		var m struct {
			ID      interface{} `json:"id"`
			Method  string      `json:"method"`
			Geracao string      `json:"geracao"`
		}
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatalf("faltaram eventos (vistos %v): %v", vistos, err)
		}
		if m.Method == "" {
			continue
		}
		if m.Geracao != resp.Geracao {
			t.Fatalf("%s sem geracao certa: %q (esperado %q)", m.Method, m.Geracao, resp.Geracao)
		}
		vistos[m.Method] = true
	}
	for _, e := range esperados {
		if !vistos[e] {
			t.Fatalf("evento %s não chegou", e)
		}
	}
}
