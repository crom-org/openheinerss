package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/checkpoint"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/storage"
)

// EventHandler é o callback invocado sempre que um evento de sessão é emitido
type EventHandler func(notification protocol.Notification)

// Session representa uma sessão de agente ativa
type Session struct {
	mu                        sync.RWMutex
	ID                        string
	Harness                   harness.Harness
	Config                    harness.SessionConfig
	CreatedAt                 time.Time
	Running                   bool
	startedAt                 time.Time
	inputTokens, outputTokens int64
	ctx                       context.Context
	cancel                    context.CancelFunc
}

// Manager coordena o ciclo de vida de todas as sessões ativas no Openheinerss
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	listeners []EventHandler
	storage   *storage.Storage
}

// NewManager cria uma nova instância de SessionManager
func NewManager() *Manager {
	return &Manager{
		sessions:  make(map[string]*Session),
		listeners: make([]EventHandler, 0),
		storage:   storage.GetStorage(),
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
		Harness:        params.Harness,
		Mode:           string(mode),
		CWD:            params.CWD,
		Provider:       params.Provider,
		Model:          params.Model,
		Env:            params.Env,
		PermissionMode: params.Options.PermissionMode,
		SystemPrompt:   params.Options.SystemPrompt,
		Options:        params.Options.Extra,
	}
	// O snapshot é feito antes de o motor receber o primeiro prompt.
	// Sem checkpoint (pasta somente leitura, por exemplo) a sessão continua, mas o aviso não se perde.
	if _, err := checkpoint.GetManager().CreateCheckpoint(params.CWD, sessID, "antes do primeiro prompt"); err != nil {
		fmt.Fprintf(os.Stderr, "aviso: checkpoint inicial não criado em %s: %v\n", params.CWD, err)
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
		startedAt: time.Now(),
	}

	m.mu.Lock()
	m.sessions[sessID] = s
	m.mu.Unlock()

	// Inicia consumo assíncrono dos eventos do harness
	go m.forwardEvents(s)
	_ = m.storage.Record(params.CWD, sessID, "system", nil, "", cfg)

	return &protocol.SessionCreateResult{
		SessionID: sessID,
		Harness:   params.Harness,
		Mode:      string(mode),
		CWD:       params.CWD,
		Status:    "ready",
	}, nil
}

// PromptSession envia um novo prompt para a sessão ativa
// ResumeSession reidrata os metadados persistidos e entrega o ID nativo ao motor.
func (m *Manager) ResumeSession(ctx context.Context, params protocol.SessionResumeParams) (*protocol.SessionResumeResult, error) {
	if params.SessionID == "" {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "sessionId é obrigatório"}
	}
	if params.CWD == "" {
		params.CWD, _ = os.Getwd()
	}
	entries, err := m.storage.LoadSession(params.CWD, params.SessionID)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeSessionNotFound, Message: err.Error()}
	}
	var cfg harness.SessionConfig
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Config != nil {
			b, _ := json.Marshal(entries[i].Config)
			if json.Unmarshal(b, &cfg) == nil {
				break
			}
		}
	}
	if cfg.Harness == "" {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "configuração da sessão não encontrada"}
	}
	if cfg.CWD == "" {
		cfg.CWD = params.CWD
	}
	cfg.SessionID = params.SessionID
	mode := harness.Mode(cfg.Mode)
	if mode == "" {
		mode = harness.ModeCLI
	}
	h, err := harness.Create(cfg.Harness, mode)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeHarnessNotFound, Message: err.Error()}
	}
	if p := h.ValidatePrerequisites(ctx); !p.Satisfied {
		return nil, &protocol.RPCError{Code: protocol.CodeHarnessDependencyMissing, Message: strings.Join(p.MissingItems, ", "), Data: protocol.ErrorData{SuggestedFix: p.SuggestedFix}}
	}
	// A sessão vive além da requisição que a retomou: o harness usa o contexto da própria sessão.
	sessCtx, cancel := context.WithCancel(context.Background())
	if err := h.Start(sessCtx, cfg); err != nil {
		cancel()
		return nil, &protocol.RPCError{Code: protocol.CodeInternalError, Message: err.Error()}
	}
	s := &Session{ID: params.SessionID, Harness: h, Config: cfg, CreatedAt: time.Now(), ctx: sessCtx, cancel: cancel, startedAt: time.Now()}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	go m.forwardEvents(s)
	return &protocol.SessionResumeResult{SessionID: s.ID, Harness: cfg.Harness, Mode: string(mode), CWD: cfg.CWD, Status: "ready"}, nil
}

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
	_ = m.storage.Record(s.Config.CWD, s.ID, "user_prompt", nil, params.Text, nil)

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

// Close encerra todas as sessões abertas (para os processos dos harnesses ao desligar o servidor).
func (m *Manager) Close() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		s.mu.Lock()
		s.Running = false
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		_ = s.Harness.Stop()
	}
}

func (m *Manager) forwardEvents(s *Session) {
	events := s.Harness.Events()
	for {
		var evt harness.Event
		select {
		case evt = <-events:
		case <-s.ctx.Done():
			return // sessão abortada: não deixa a goroutine presa num canal que ninguém fecha
		}
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

		n := protocol.NewNotification(method, evt.Payload)
		if u, ok := evt.Payload.(protocol.UsageParams); ok {
			s.mu.Lock()
			s.inputTokens += u.InputTokens
			s.outputTokens += u.OutputTokens
			s.mu.Unlock()
		}
		if evt.Type == harness.EventComplete {
			s.mu.Lock()
			if p, ok := evt.Payload.(protocol.CompleteParams); ok {
				p.DurationMs = time.Since(s.startedAt).Milliseconds()
				p.InputTokens = s.inputTokens
				p.OutputTokens = s.outputTokens
				p.TotalTokens = p.InputTokens + p.OutputTokens
				n.Params = p
			}
			s.mu.Unlock()
			if r, ok := s.Harness.(interface{ ResumeID() string }); ok {
				if id := r.ResumeID(); id != "" {
					s.mu.Lock()
					if s.Config.Options == nil {
						s.Config.Options = map[string]interface{}{}
					}
					s.Config.Options["session_id"] = id
					cfg := s.Config
					s.mu.Unlock()
					_ = m.storage.Record(cfg.CWD, s.ID, "system", nil, "", cfg)
				}
			}
		}
		if evt.Type == harness.EventError {
			if p, ok := evt.Payload.(protocol.ErrorParams); ok && p.SuggestedFix == "" {
				_, p.SuggestedFix = harness.ClassifyFailure(p.Message)
				n.Params = p
			}
		}
		_ = m.storage.Record(s.Config.CWD, s.ID, "event", &n, "", nil)
		m.broadcast(n)
	}
}

func generateSessionID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sess_%s", hex.EncodeToString(b))
}
