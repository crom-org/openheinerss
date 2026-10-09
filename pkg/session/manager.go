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
	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/risco"
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
	// risco fica nil com o classificador desligado (padrão): a ponte é só túnel.
	risco *risco.Classificador
}

// Manager coordena o ciclo de vida de todas as sessões ativas no Openheinerss
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	listeners []EventHandler
	storage   *storage.Storage
	// classificarRisco é o padrão das sessões (serve/run --classificar-risco); a sessão pode mudar.
	classificarRisco bool
}

// SetClassificarRisco liga ou desliga o classificador de risco como padrão das próximas sessões.
func (m *Manager) SetClassificarRisco(on bool) {
	m.mu.Lock()
	m.classificarRisco = on
	m.mu.Unlock()
}

// classificador carrega as regras de risco quando a sessão (ou o padrão do manager) pede.
func (m *Manager) classificador(cwd string, pedido *bool) (*risco.Classificador, error) {
	m.mu.RLock()
	on := m.classificarRisco
	m.mu.RUnlock()
	if pedido != nil {
		on = *pedido
	}
	if !on {
		return nil, nil
	}
	return risco.Carregar(cwd)
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
	if params.CWD == "" {
		params.CWD, _ = os.Getwd()
	}
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
		cfg, cfgErr := config.LoadProject(params.CWD)
		if cfgErr != nil {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: cfgErr.Error()}
		}
		params.Harness, params.Mode = cfg.DefaultHarness, cfg.DefaultMode
		if params.Harness == "" {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "harness obrigatório: informe-o explicitamente ou configure default_harness em .openheinerss/config.yaml"}
		}
	}
	ident, err := identidade.Para(params.Harness, params.Env)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}

	mode := harness.Mode(params.Mode)
	if mode == "" {
		mode = harness.ModeCLI
	}

	h, err := harness.Create(params.Harness, mode)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.CodeHarnessNotFound,
			Message: fmt.Sprintf("Harness '%s' não encontrado: %v", params.Harness, err),
		}
	}
	mode = h.Mode()
	params.Harness = h.Name()

	// Recusa a retomada antes de procurar o binário: alguns harnesses, como o
	// aider, não têm retomada nativa e essa deve ser a causa informada mesmo
	// quando o CLI também não está instalado.
	if params.Retomar != "" {
		nativo := params.Retomar
		if id, ok := m.storage.IDNativo(params.CWD, params.Retomar); ok {
			nativo = id
		}
		if _, rerr := harness.OpcoesRetomada(harness.BaseDe(params.Harness), nativo); rerr != nil {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: rerr.Error()}
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

	// Opções tipadas e o repasse nativo viajam para o adaptador dentro de Options.
	options := make(map[string]interface{}, len(params.Options.Extra)+2)
	for k, v := range params.Options.Extra {
		options[k] = v
	}
	if params.Options.Effort != "" {
		if _, ok := options["effort"]; !ok {
			options["effort"] = params.Options.Effort
		}
	}
	if len(params.Options.HarnessArgs) > 0 {
		options[harness.OptionHarnessArgs] = append(harness.HarnessArgs(options), params.Options.HarnessArgs...)
	}
	if params.Options.SemMCP {
		options[harness.OptionSemMCP] = true
	} else if len(params.Options.MCP) > 0 {
		options[harness.OptionMCP] = append([]string(nil), params.Options.MCP...)
	}
	if params.Retomar != "" {
		// Um id de sessão do openheinerss vale pelo id nativo que ela gravou; qualquer outro é o id nativo.
		nativo := params.Retomar
		if id, ok := m.storage.IDNativo(params.CWD, params.Retomar); ok {
			nativo = id
		}
		ro, rerr := harness.OpcoesRetomada(harness.BaseDe(params.Harness), nativo)
		if rerr != nil {
			return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: rerr.Error()}
		}
		for k, v := range ro {
			options[k] = v
		}
	}
	classif, err := m.classificador(params.CWD, params.Options.ClassificarRisco)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	if classif != nil {
		options["classificar_risco"] = true
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
		Options:        options,
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
		risco:     classif,
	}

	m.mu.Lock()
	m.sessions[sessID] = s
	m.mu.Unlock()

	// Inicia consumo assíncrono dos eventos do harness
	go m.forwardEvents(s)
	_ = m.storage.Record(params.CWD, sessID, "system", nil, "", cfg)

	return &protocol.SessionCreateResult{
		SessionID:  sessID,
		Harness:    params.Harness,
		Mode:       string(mode),
		CWD:        params.CWD,
		Status:     "ready",
		Identidade: ident,
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
	mode = h.Mode()
	ident, err := identidade.Para(cfg.Harness, cfg.Env)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	if p := h.ValidatePrerequisites(ctx); !p.Satisfied {
		return nil, &protocol.RPCError{Code: protocol.CodeHarnessDependencyMissing, Message: strings.Join(p.MissingItems, ", "), Data: protocol.ErrorData{SuggestedFix: p.SuggestedFix}}
	}
	var pedido *bool
	if v, ok := cfg.Options["classificar_risco"].(bool); ok {
		pedido = &v
	}
	classif, err := m.classificador(cfg.CWD, pedido)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}
	// A sessão vive além da requisição que a retomou: o harness usa o contexto da própria sessão.
	sessCtx, cancel := context.WithCancel(context.Background())
	if err := h.Start(sessCtx, cfg); err != nil {
		cancel()
		return nil, &protocol.RPCError{Code: protocol.CodeInternalError, Message: err.Error()}
	}
	s := &Session{ID: params.SessionID, Harness: h, Config: cfg, CreatedAt: time.Now(), ctx: sessCtx, cancel: cancel, startedAt: time.Now(), risco: classif}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	go m.forwardEvents(s)
	return &protocol.SessionResumeResult{SessionID: s.ID, Harness: cfg.Harness, Mode: string(mode), CWD: cfg.CWD, Status: "ready", Identidade: ident}, nil
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
		case harness.EventRaw:
			method = protocol.EventAgentRaw
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
		if s.risco != nil {
			if p, ok := classificarEvento(s.risco, evt.Payload, s.Config.CWD); ok {
				n.Params = p
			}
		}
		if evt.Type == harness.EventError {
			if p, ok := evt.Payload.(protocol.ErrorParams); ok && p.SuggestedFix == "" {
				_, p.SuggestedFix = harness.ClassifyFailure(p.Message)
				n.Params = p
			}
		}
		n.Params = comSessao(n.Params, s.ID)
		_ = m.storage.Record(s.Config.CWD, s.ID, "event", &n, "", nil)
		m.broadcast(n)
	}
}

// comSessao garante que o evento leve o sessionId devolvido por session.create. Motores como o
// claude-code informam o id nativo da conversa; ele segue em nativeSessionId.
func comSessao(params interface{}, id string) interface{} {
	raw, err := json.Marshal(params)
	if err != nil {
		return params
	}
	var campos map[string]interface{}
	if json.Unmarshal(raw, &campos) != nil || campos == nil {
		return params
	}
	atual, _ := campos["sessionId"].(string)
	if atual == id {
		return params
	}
	if atual != "" {
		campos["nativeSessionId"] = atual
	}
	campos["sessionId"] = id
	return campos
}

// classificarEvento acrescenta risco e motivo a tool_call e permission_request (só informa).
func classificarEvento(c *risco.Classificador, payload interface{}, cwd string) (interface{}, bool) {
	switch p := payload.(type) {
	case protocol.ToolCallParams:
		p.Risco, p.MotivoRisco = c.Classificar(p.Tool, "", p.Input, cwd)
		return p, true
	case *protocol.ToolCallParams:
		q := *p
		q.Risco, q.MotivoRisco = c.Classificar(q.Tool, "", q.Input, cwd)
		return q, true
	case protocol.PermissionRequestParams:
		p.Risco, p.MotivoRisco = c.Classificar(p.Tool, p.Command, nil, cwd)
		return p, true
	case *protocol.PermissionRequestParams:
		q := *p
		q.Risco, q.MotivoRisco = c.Classificar(q.Tool, q.Command, nil, cwd)
		return q, true
	}
	return nil, false
}

func generateSessionID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sess_%s", hex.EncodeToString(b))
}
