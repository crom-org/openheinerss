package server_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
	"github.com/gorilla/websocket"
)

func TestWebSocketStreamingPontaAPonta(t *testing.T) {
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
	for i := 0; i < 30; i++ {
		conn, _, err = websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err == nil {
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
	send(1, protocol.MethodSessionCreate, protocol.SessionCreateParams{Harness: "mock", CWD: t.TempDir()})
	var response protocol.Response
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	var created protocol.SessionCreateResult
	resultBytes, _ := json.Marshal(response.Result)
	if err := json.Unmarshal(resultBytes, &created); err != nil {
		t.Fatal(err)
	}
	send(2, protocol.MethodSessionPrompt, protocol.SessionPromptParams{SessionID: created.SessionID, Text: "olá"})
	seenText, seenComplete := false, false
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for !seenComplete {
		var raw json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			t.Fatal(err)
		}
		var n protocol.Notification
		if json.Unmarshal(raw, &n) == nil && n.Method == protocol.EventAgentText {
			seenText = true
		}
		if n.Method == protocol.EventAgentPermissionRequest {
			var p protocol.PermissionRequestParams
			params, _ := json.Marshal(n.Params)
			_ = json.Unmarshal(params, &p)
			send(3, protocol.MethodSessionPermissionRespond, protocol.PermissionRespondParams{SessionID: p.SessionID, RequestID: p.RequestID, Decision: "allow"})
		}
		if n.Method == protocol.EventAgentComplete {
			seenComplete = true
		}
	}
	if !seenText {
		t.Fatal("texto não chegou pelo WebSocket")
	}
}

func mustJSON(v interface{}) json.RawMessage { b, _ := json.Marshal(v); return b }
