package harness

import (
	"context"
	"fmt"
	"sync"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

// Mode define o modo de operação do harness
type Mode string

const (
	ModeSDK  Mode = "sdk"
	ModeCLI  Mode = "cli"
	ModeAPI  Mode = "api"
	ModeMock Mode = "mock"
)

// EventType define o tipo normalizado de evento emitido pelo harness
type EventType string

const (
	EventThinking   EventType = "thinking"
	EventText       EventType = "text"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventPermission EventType = "permission"
	EventComplete   EventType = "complete"
	EventError      EventType = "error"
	EventUsage      EventType = "usage"
)

// Event encapsula um evento emitido pelo harness
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

// PrerequisiteResult traz o resultado de verificação de dependências
type PrerequisiteResult struct {
	Satisfied    bool     `json:"satisfied"`
	MissingItems []string `json:"missing_items,omitempty"`
	SuggestedFix string   `json:"suggested_fix,omitempty"`
}

// SessionConfig traz as opções de inicialização de um harness
type SessionConfig struct {
	SessionID      string                 `json:"sessionId"`
	CWD            string                 `json:"cwd"`
	Provider       string                 `json:"provider"`
	Model          string                 `json:"model"`
	Env            map[string]string      `json:"env"`
	PermissionMode string                 `json:"permission_mode"`
	SystemPrompt   string                 `json:"system_prompt,omitempty"`
	Options        map[string]interface{} `json:"options,omitempty"`
}

// Harness é o contrato universal que todo motor deve satisfazer
type Harness interface {
	Name() string
	Mode() Mode
	ValidatePrerequisites(ctx context.Context) PrerequisiteResult
	Start(ctx context.Context, cfg SessionConfig) error
	SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error
	RespondPermission(ctx context.Context, reqID string, allow bool, message string) error
	Events() <-chan Event
	Stop() error
}

// Factory é a função que instancia um Harness dado um Mode
type Factory func(mode Mode) (Harness, error)

// Registry global de adaptadores de harness
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	metadata  map[string]protocol.HarnessCatalogItem
}

var defaultRegistry = &Registry{
	factories: make(map[string]Factory),
	metadata:  make(map[string]protocol.HarnessCatalogItem),
}

// Register registra um harness na fábrica global
func Register(name string, meta protocol.HarnessCatalogItem, factory Factory) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	defaultRegistry.factories[name] = factory
	defaultRegistry.metadata[name] = meta
}

// Create instancia um harness pelo nome e modo
func Create(name string, mode Mode) (Harness, error) {
	defaultRegistry.mu.RLock()
	factory, exists := defaultRegistry.factories[name]
	defaultRegistry.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("harness '%s' não encontrado no registro", name)
	}

	return factory(mode)
}

// ListCatalog retorna os metadados de todos os harnesses registrados
func ListCatalog() []protocol.HarnessCatalogItem {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	items := make([]protocol.HarnessCatalogItem, 0, len(defaultRegistry.metadata))
	for _, meta := range defaultRegistry.metadata {
		items = append(items, meta)
	}
	return items
}
