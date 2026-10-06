package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

func TestStdioServerProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	manager := session.NewManager()

	// Cria pipes para simular stdin e stdout
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	stdioServer := server.NewStdioServer(manager, inR, outW)

	go func() {
		_ = stdioServer.Run(ctx)
		_ = outW.Close()
	}()

	scanner := bufio.NewScanner(outR)

	// 1. Enviar catalog.list
	req1 := `{"jsonrpc":"2.0","id":1,"method":"catalog.list"}` + "\n"
	if _, err := inW.Write([]byte(req1)); err != nil {
		t.Fatalf("falha ao escrever no pipe: %v", err)
	}

	if !scanner.Scan() {
		t.Fatalf("não recebeu resposta para catalog.list: %v", scanner.Err())
	}

	var resp1 protocol.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp1); err != nil {
		t.Fatalf("falha ao parsear resposta: %v", err)
	}
	if resp1.Error != nil {
		t.Fatalf("erro inesperado na resposta 1: %+v", resp1.Error)
	}

	// 2. Enviar doctor.check
	req2 := `{"jsonrpc":"2.0","id":2,"method":"doctor.check"}` + "\n"
	if _, err := inW.Write([]byte(req2)); err != nil {
		t.Fatalf("falha ao escrever no pipe: %v", err)
	}

	if !scanner.Scan() {
		t.Fatalf("não recebeu resposta para doctor.check: %v", scanner.Err())
	}

	var resp2 protocol.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp2); err != nil {
		t.Fatalf("falha ao parsear resposta 2: %v", err)
	}
	if resp2.Error != nil {
		t.Fatalf("erro inesperado na resposta 2: %+v", resp2.Error)
	}

	// 3. Enviar mcp.list
	req3 := `{"jsonrpc":"2.0","id":3,"method":"mcp.list"}` + "\n"
	if _, err := inW.Write([]byte(req3)); err != nil {
		t.Fatalf("falha ao escrever no pipe: %v", err)
	}

	if !scanner.Scan() {
		t.Fatalf("não recebeu resposta para mcp.list: %v", scanner.Err())
	}

	var resp3 protocol.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp3); err != nil {
		t.Fatalf("falha ao parsear resposta 3: %v", err)
	}
	if resp3.Error != nil {
		t.Fatalf("erro inesperado na resposta 3: %+v", resp3.Error)
	}

	_ = inW.Close()
}
