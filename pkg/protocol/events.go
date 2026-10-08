package protocol

// Métodos de notificação / eventos enviados do agente para o cliente
const (
	EventAgentThinking          = "agent.thinking"
	EventAgentText              = "agent.text"
	EventAgentToolCall          = "agent.tool_call"
	EventAgentToolResult        = "agent.tool_result"
	EventAgentPermissionRequest = "agent.permission_request"
	EventAgentComplete          = "agent.complete"
	EventAgentError             = "agent.error"
	EventAgentUsage             = "agent.usage"
)

// ThinkingParams payload para agent.thinking
type ThinkingParams struct {
	SessionID string `json:"sessionId"`
	Delta     string `json:"delta"`
}

// TextParams payload para agent.text
type TextParams struct {
	SessionID string `json:"sessionId"`
	Delta     string `json:"delta"`
}

// ToolCallParams payload para agent.tool_call
type ToolCallParams struct {
	SessionID string      `json:"sessionId"`
	CallID    string      `json:"callId"`
	Tool      string      `json:"tool"`
	Input     interface{} `json:"input"`
}

// ToolResultParams payload para agent.tool_result
type ToolResultParams struct {
	SessionID string `json:"sessionId"`
	CallID    string `json:"callId"`
	Status    string `json:"status"` // "success" ou "error"
	Output    string `json:"output"`
}

// PermissionRequestParams payload para agent.permission_request
type PermissionRequestParams struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Tool      string `json:"tool"`
	Command   string `json:"command,omitempty"`
	Risk      string `json:"risk,omitempty"` // "low", "medium", "high"
}

// CompleteParams payload para agent.complete
type CompleteParams struct {
	SessionID    string `json:"sessionId"`
	Reason       string `json:"reason,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	InputTokens  int64  `json:"input_tokens,omitempty"`
	OutputTokens int64  `json:"output_tokens,omitempty"`
	TotalTokens  int64  `json:"total_tokens,omitempty"`
}

// ErrorParams payload para agent.error
type ErrorParams struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Code      int    `json:"code,omitempty"`
	// SuggestedFix é preenchido nos casos conhecidos (CLI ausente, sem login, sem cota).
	SuggestedFix string `json:"suggestedFix,omitempty"`
}

// UsageParams informa o consumo de tokens quando o motor o fornece.
type UsageParams struct {
	SessionID    string  `json:"sessionId"`
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
	TotalTokens  int64   `json:"totalTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
}
