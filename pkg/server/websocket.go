package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Permite conexões de qualquer front-end local ou web
	},
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
