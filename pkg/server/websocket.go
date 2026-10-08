package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

var upgrader = websocket.Upgrader{CheckOrigin: origemPermitida}

// origemPermitida bloqueia páginas web de terceiros: o servidor executa agentes na máquina,
// então um site aberto no navegador não pode abrir WebSocket para 127.0.0.1.
// Clientes sem cabeçalho Origin (SDKs, scripts) e origens locais passam; outras
// origens precisam estar em OPENHEINERSS_ORIGENS (lista separada por vírgula, ou "*").
func origemPermitida(r *http.Request) bool {
	origem := r.Header.Get("Origin")
	if origem == "" {
		return true
	}
	if u, err := url.Parse(origem); err == nil {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1", "tauri.localhost":
			return true
		}
	}
	for _, permitida := range strings.Split(os.Getenv("OPENHEINERSS_ORIGENS"), ",") {
		permitida = strings.TrimSpace(permitida)
		if permitida == "*" || (permitida != "" && permitida == origem) {
			return true
		}
	}
	return false
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// WSServer provê um servidor WebSocket HTTP para conexão de frontends web (React, Tauri, etc)
type WSServer struct {
	router  *Router
	manager *session.Manager
	clients map[*wsClient]bool
	mu      sync.RWMutex
	server  *http.Server
}

// NewWSServer cria um novo servidor WebSocket
func NewWSServer(m *session.Manager) *WSServer {
	s := &WSServer{
		router:  NewRouter(m),
		manager: m,
		clients: make(map[*wsClient]bool),
	}

	// Repassa eventos assíncronos do agente para todos os clientes conectados
	m.SubscribeEvents(func(notification protocol.Notification) {
		s.broadcast(notification)
	})

	return s
}

func (s *WSServer) broadcast(v interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for client := range s.clients {
		_ = client.writeJSON(v)
	}
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn}
	ctx, c := comConexao(context.Background(), func(v interface{}) { _ = client.writeJSON(v) })
	defer s.router.orq.desconectar(c)
	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, client)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req protocol.Request
		if err := json.Unmarshal(message, &req); err != nil {
			errResp := protocol.NewErrorResponse(nil, protocol.CodeParseError, fmt.Sprintf("JSON inválido: %v", err), nil)
			_ = client.writeJSON(errResp)
			continue
		}

		resp := s.router.HandleRequest(ctx, req)
		if err := client.writeJSON(resp); err != nil {
			break
		}
	}
}

// ListenAndServe inicia o servidor HTTP na porta informada
func (s *WSServer) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	mux.HandleFunc("/ws", s.handleWS)

	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Shutdown desliga o servidor graciosamente
func (s *WSServer) Shutdown(ctx context.Context) error {
	s.router.orq.Close()
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}
