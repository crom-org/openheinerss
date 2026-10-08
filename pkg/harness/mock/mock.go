package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	harness.Register("mock", protocol.HarnessCatalogItem{
		ID:                 "mock",
		DisplayName:        "Mock Harness (Test Engine)",
		SupportedModes:     []string{"mock"},
		SupportedProtocols: []string{"mock"},
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "mock-local",
				Name:        "Simulador Local",
				Models:      []string{"mock-v1", "mock-fast"},
				RequiresKey: false,
			},
		},
	}, func(mode harness.Mode) (harness.Harness, error) {
		return NewMockHarness(), nil
	})
}

// permDecision representa a resposta recebida para uma permissão pendente
type permDecision struct {
	reqID   string
	allow   bool
	message string
}

// MockHarness simula um agente de código com streaming, chamadas de tools e pedidos de permissão
type MockHarness struct {
	mu        sync.Mutex
	cfg       harness.SessionConfig
	events    chan harness.Event
	permCh    chan permDecision
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   bool
	stepDelay time.Duration
	args      []string
	prompts   []string
}

// ReceivedArgs devolve os harness_args recebidos em Start (para testes de ponta a ponta).
func (m *MockHarness) ReceivedArgs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.args...)
}

// ReceivedPrompts devolve os prompts recebidos por SendPrompt, na ordem.
func (m *MockHarness) ReceivedPrompts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.prompts...)
}

// NewMockHarness cria uma nova instância de MockHarness
func NewMockHarness() *MockHarness {
	return &MockHarness{
		events:    make(chan harness.Event, 100),
		permCh:    make(chan permDecision, 1),
		stepDelay: 10 * time.Millisecond, // delay pequeno para testes rápidos
	}
}

// SetStepDelay permite customizar o delay entre etapas (ex: para visualização em CLI)
func (m *MockHarness) SetStepDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stepDelay = d
}

func (m *MockHarness) Name() string {
	return "mock"
}

func (m *MockHarness) Mode() harness.Mode {
	return harness.ModeMock
}

func (m *MockHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	return harness.PrerequisiteResult{
		Satisfied: true,
	}
}

func (m *MockHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg = cfg
	m.args = harness.HarnessArgs(cfg.Options)
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.stopped = false

	return nil
}

func (m *MockHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	m.mu.Lock()
	if m.stopped || m.ctx == nil {
		m.mu.Unlock()
		return fmt.Errorf("mock harness não está iniciado ou foi interrompido")
	}
	m.prompts = append(m.prompts, text)
	m.mu.Unlock()

	// Executa a simulação em uma goroutine assíncrona
	go m.runSimulation(text)

	return nil
}

func (m *MockHarness) runSimulation(text string) {
	sessID := m.cfg.SessionID

	// Ponte: os harness_args recebidos voltam num evento raw (harness_args=<json>).
	if args := m.ReceivedArgs(); len(args) > 0 {
		b, _ := json.Marshal(args)
		m.emit(harness.RawEvent(sessID, "mock", "stdout", "harness_args="+string(b)))
	}

	// Um /comando chega literal: o mock confirma e encerra o turno.
	if name, _, ok := harness.SlashCommand(text); ok {
		line := "comando /" + name + " recebido"
		m.emit(harness.RawEvent(sessID, "mock", "stdout", line))
		m.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: line}})
		m.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "finished"}})
		return
	}

	// 1. Thinking
	m.emit(harness.Event{
		Type: harness.EventThinking,
		Payload: protocol.ThinkingParams{
			SessionID: sessID,
			Delta:     fmt.Sprintf("Analisando solicitação: '%s'...", text),
		},
	})
	m.sleep()

	if m.isCancelled() {
		return
	}

	// 2. Text inicial
	m.emit(harness.Event{
		Type: harness.EventText,
		Payload: protocol.TextParams{
			SessionID: sessID,
			Delta:     "Entendido. Para atender ao seu pedido, preciso executar uma checagem no ambiente.",
		},
	})
	m.sleep()

	if m.isCancelled() {
		return
	}

	// 3. Permission Request
	reqID := "perm_mock_1"
	m.emit(harness.Event{
		Type: harness.EventPermission,
		Payload: protocol.PermissionRequestParams{
			SessionID: sessID,
			RequestID: reqID,
			Tool:      "Bash",
			Command:   "git status",
			Risk:      "medium",
		},
	})

	// 4. Aguarda resposta de permissão ou cancelamento
	var decision permDecision
	select {
	case <-m.ctx.Done():
		return
	case decision = <-m.permCh:
	}

	if !decision.allow {
		m.emit(harness.Event{
			Type: harness.EventText,
			Payload: protocol.TextParams{
				SessionID: sessID,
				Delta:     "A execução foi cancelada pois a permissão da ferramenta foi negada.",
			},
		})
		m.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    "permission_denied",
			},
		})
		return
	}

	// 5. Tool Call
	callID := "call_mock_1"
	m.emit(harness.Event{
		Type: harness.EventToolCall,
		Payload: protocol.ToolCallParams{
			SessionID: sessID,
			CallID:    callID,
			Tool:      "Bash",
			Input:     map[string]interface{}{"command": "git status"},
		},
	})
	m.sleep()

	if m.isCancelled() {
		return
	}

	// 6. Tool Result
	m.emit(harness.Event{
		Type: harness.EventToolResult,
		Payload: protocol.ToolResultParams{
			SessionID: sessID,
			CallID:    callID,
			Status:    "success",
			Output:    "On branch main\nnothing to commit, working tree clean",
		},
	})
	m.sleep()

	if m.isCancelled() {
		return
	}

	// 7. Text final
	m.emit(harness.Event{
		Type: harness.EventText,
		Payload: protocol.TextParams{
			SessionID: sessID,
			Delta:     "Tarefa concluída com sucesso! O repositório está limpo e atualizado.",
		},
	})
	m.sleep()

	// 8. Complete
	m.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "finished",
		},
	})
}

func (m *MockHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return fmt.Errorf("harness já foi finalizado")
	}

	select {
	case m.permCh <- permDecision{reqID: reqID, allow: allow, message: message}:
		return nil
	default:
		return fmt.Errorf("nenhuma solicitação de permissão pendente para responder")
	}
}

func (m *MockHarness) Events() <-chan harness.Event {
	return m.events
}

func (m *MockHarness) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}
	m.stopped = true
	if m.cancel != nil {
		m.cancel()
	}

	return nil
}

func (m *MockHarness) emit(evt harness.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	select {
	case m.events <- evt:
	default:
	}
}

func (m *MockHarness) isCancelled() bool {
	if m.ctx == nil {
		return true
	}
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

func (m *MockHarness) sleep() {
	m.mu.Lock()
	d := m.stepDelay
	m.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
}
