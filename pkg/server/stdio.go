package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

// StdioServer gerencia o transporte JSON-RPC bidirecional via STDIO (linhas NDJSON)
type StdioServer struct {
	router  *Router
	manager *session.Manager
	in      io.Reader
	out     io.Writer
	outMu   sync.Mutex
}

// NewStdioServer cria uma nova instância de StdioServer
func NewStdioServer(m *session.Manager, in io.Reader, out io.Writer) *StdioServer {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	s := &StdioServer{
		router:  NewRouter(m),
		manager: m,
		in:      in,
		out:     out,
	}

	// Inscreve para repassar eventos assíncronos direto para o stdout
	m.SubscribeEvents(func(notification protocol.Notification) {
		s.writeMessage(notification)
	})

	return s
}

// Run inicia o loop de leitura e processamento de linhas do stdin
func (s *StdioServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	// Suporta linhas grandes (ex: buffers de código ou imagens base64)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req protocol.Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			errResp := protocol.NewErrorResponse(nil, protocol.CodeParseError, fmt.Sprintf("JSON inválido: %v", err), nil)
			s.writeMessage(errResp)
			continue
		}

		// Processa o request
		resp := s.router.HandleRequest(ctx, req)
		s.writeMessage(resp)
	}

	return scanner.Err()
}

func (s *StdioServer) writeMessage(v interface{}) {
	s.outMu.Lock()
	defer s.outMu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.out, "%s\n", data)
}
