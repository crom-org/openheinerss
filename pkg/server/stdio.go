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
	return NewStdioServerWithMaxAgents(m, in, out, 0)
}

// NewStdioServerWithMaxAgents cria o servidor STDIO com limite de agentes opcional.
func NewStdioServerWithMaxAgents(m *session.Manager, in io.Reader, out io.Writer, maxAgents int) *StdioServer {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	s := &StdioServer{
		router:  NewRouterWithMaxAgents(m, maxAgents),
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
	ctx, c := comConexao(ctx, func(v interface{}) { s.writeMessage(v) })
	defer s.router.orq.desconectar(c)
	defer s.router.orq.Close()
	scanner := bufio.NewScanner(s.in)
	// Suporta linhas grandes (ex: buffers de código ou imagens base64)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)

	// A leitura fica numa goroutine para que SIGTERM/Ctrl-C encerrem o servidor mesmo com o stdin parado.
	linhas := make(chan string)
	go func() {
		defer close(linhas)
		for scanner.Scan() {
			select {
			case linhas <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		var bruta string
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l, ok := <-linhas:
			if !ok {
				return scanner.Err()
			}
			bruta = l
		}

		line := strings.TrimSpace(bruta)
		if line == "" {
			continue
		}

		var req protocol.Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			errResp := protocol.NewErrorResponse(nil, protocol.CodeParseError, fmt.Sprintf("JSON inválido: %v", err), nil)
			errResp.Geracao = s.router.geracao
			s.writeMessage(errResp)
			continue
		}

		// Processa o request
		resp := s.router.HandleRequest(ctx, req)
		s.writeMessage(resp)
	}
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
