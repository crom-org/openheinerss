package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// EventHandler é o callback invocado sempre que um evento de sessão é emitido
type EventHandler func(notification protocol.Notification)

// Session representa uma sessão de agente ativa
type Session struct {
	mu        sync.RWMutex
	ID        string
	Harness   harness.Harness
	Config    harness.SessionConfig
	CreatedAt time.Time
	Running   bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// Manager coordena o ciclo de vida de todas as sessões ativas no Openheinerss
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	listeners []EventHandler
}

// NewManager cria uma nova instância de SessionManager
func NewManager() *Manager {
	return &Manager{
		sessions:  make(map[string]*Session),
		listeners: make([]EventHandler, 0),
	}
}

// SubscribeEvents registra um listener global para receber notificações de eventos
func (m *Manager) SubscribeEvents(h EventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, h)
}

func (m *Manager) broadcast(notification protocol.Notification) {
	m.mu.RLock()
	listeners := make([]EventHandler, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, l := range listeners {
		l(notification)
	}
}

// CreateSession inicializa uma nova sessão com o harness e configurações solicitadas
func (m *Manager) CreateSession(ctx context.Context, params protocol.SessionCreateParams) (*protocol.SessionCreateResult, error) {
	if params.Papel != "" {
		roles, path, err := motor.FindRoles(params.CWD)
		if err != nil {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: err.Error()}
		}
		p, ok := roles[params.Papel]
		if !ok {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: fmt.Sprintf("papel '%s' não encontrado em %s", params.Papel, path)}
		}
		params.Harness, params.Mode, params.Provider, params.Model, params.Env = p.Harness, p.Mode, p.Provider, p.Model, p.Env
		if params.Options.Extra == nil {
			params.Options.Extra = map[string]interface{}{}
		}
		if p.Effort != "" {
			params.Options.Extra["effort"] = p.Effort
		}
	}
	if params.Harness == "" {
		params.Harness = "mock"
	}

	mode := harness.Mode(params.Mode)
	if mode == "" {
		mode = harness.ModeMock
	}

	h, err := harness.Create(params.Harness, mode)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeHarnessNotFound,
			Message: fmt.Sprintf("Harness '%s' não encontrado: %v", params.Harness, err),
		}
	}

	// 1. Valida pré-requisitos do harness
	prereq := h.ValidatePrerequisites(ctx)
	if !prereq.Satisfied {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeHarnessDependencyMissing,
			Message: fmt.Sprintf("Pré-requisito ausente para %s (%s)", params.Harness, mode),
			Data: protocol.ErrorData{
				Harness:      params.Harness,
				Mode:         string(mode),
				Missing:      prereq.MissingItems,
				SuggestedFix: prereq.SuggestedFix,
			},
		}
	}

	sessID := generateSessionID()
	sessCtx, cancel := context.WithCancel(context.Background())

	cfg := harness.SessionConfig{
		SessionID:      sessID,
		CWD:            params.CWD,
		Provider:       params.Provider,
		Model:          params.Model,
		Env:            params.Env,
		PermissionMode: params.Options.PermissionMode,
		SystemPrompt:   params.Options.SystemPrompt,
		Options:        params.Options.Extra,
	}

	if err := h.Start(sessCtx, cfg); err != nil {
		cancel()
		return nil, &protocol.RPCError{
			Code:    protocol.CodeInternalError,
			Message: fmt.Sprintf("Falha ao iniciar harness: %v", err),
		}
	}

	s := &Session{
		ID:        sessID,
		Harness:   h,
		Config:    cfg,
		CreatedAt: time.Now(),
		Running:   true,
		ctx:       sessCtx,
		cancel:    cancel,
	}

	m.mu.Lock()
	m.sessions[sessID] = s
	m.mu.Unlock()

	// Inicia consumo assíncrono dos eventos do harness
	go m.forwardEvents(s)

	return &protocol.SessionCreateResult{
		SessionID: sessID,
		Harness:   params.Harness,
		Mode:      string(mode),
		CWD:       params.CWD,
		Status:    "ready",
	}, nil
}

// PromptSession envia um novo prompt para a sessão ativa
func (m *Manager) PromptSession(ctx context.Context, params protocol.SessionPromptParams) (*protocol.SessionPromptResult, error) {
	m.mu.RLock()
	s, exists := m.sessions[params.SessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeSessionNotFound,
			Message: fmt.Sprintf("Sessão '%s' não encontrada", params.SessionID),
		}
	}

	s.mu.Lock()
	s.Running = true
	s.mu.Unlock()

	if err := s.Harness.SendPrompt(ctx, params.Text, params.Attachments); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeInternalError,
			Message: fmt.Sprintf("Erro ao enviar prompt: %v", err),
		}
	}

	return &protocol.SessionPromptResult{
		SessionID: params.SessionID,
		Accepted:  true,
	}, nil
}

// RespondPermission repassa a resposta de autorização da ferramenta ao harness
func (m *Manager) RespondPermission(ctx context.Context, params protocol.PermissionRespondParams) (*protocol.PermissionRespondResult, error) {
	m.mu.RLock()
	s, exists := m.sessions[params.SessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeSessionNotFound,
			Message: fmt.Sprintf("Sessão '%s' não encontrada", params.SessionID),
		}
	}

	allow := params.Decision == "allow"
	if err := s.Harness.RespondPermission(ctx, params.RequestID, allow, params.Message); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeInternalError,
			Message: fmt.Sprintf("Erro ao responder permissão: %v", err),
		}
	}

	return &protocol.PermissionRespondResult{
		SessionID: params.SessionID,
		RequestID: params.RequestID,
		Resolved:  true,
	}, nil
}

// AbortSession interrompe a execução atual da sessão
func (m *Manager) AbortSession(ctx context.Context, sessionID string) (*protocol.SessionAbortResult, error) {
	m.mu.RLock()
	s, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeSessionNotFound,
			Message: fmt.Sprintf("Sessão '%s' não encontrada", sessionID),
		}
	}

	s.mu.Lock()
	s.Running = false
	if s.cancel != nil {
		s.cancel()
	}
	_ = s.Harness.Stop()
	s.mu.Unlock()

	return &protocol.SessionAbortResult{
		SessionID: sessionID,
		Aborted:   true,
	}, nil
}

// ListSessions retorna a listagem de sessões ativas
func (m *Manager) ListSessions() []protocol.SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]protocol.SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.mu.RLock()
		list = append(list, protocol.SessionInfo{
			SessionID: s.ID,
			Harness:   s.Harness.Name(),
			Mode:      string(s.Harness.Mode()),
			CWD:       s.Config.CWD,
			Running:   s.Running,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		})
		s.mu.RUnlock()
	}
	return list
}

func (m *Manager) forwardEvents(s *Session) {
	for evt := range s.Harness.Events() {
		var method string
		switch evt.Type {
		case harness.EventThinking:
			method = protocol.EventAgentThinking
		case harness.EventText:
			method = protocol.EventAgentText
		case harness.EventToolCall:
			method = protocol.EventAgentToolCall
		case harness.EventToolResult:
			method = protocol.EventAgentToolResult
		case harness.EventPermission:
			method = protocol.EventAgentPermissionRequest
		case harness.EventComplete:
			method = protocol.EventAgentComplete
			s.mu.Lock()
			s.Running = false
			s.mu.Unlock()
		case harness.EventError:
			method = protocol.EventAgentError
			s.mu.Lock()
			s.Running = false
			s.mu.Unlock()
		case harness.EventUsage:
			method = protocol.EventAgentUsage
		default:
			method = "agent." + string(evt.Type)
		}

		m.broadcast(protocol.NewNotification(method, evt.Payload))
	}
}

func generateSessionID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sess_%s", hex.EncodeToString(b))
}
