package harness

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

// ParseJSONEvent converte o NDJSON comum dos CLIs em eventos do protocolo.
// Linhas que não são JSON retornam nil para que o adaptador possa tratá-las como texto.
func ParseJSONEvent(line, sessionID string) []Event {
	var raw map[string]interface{}
	if json.Unmarshal([]byte(line), &raw) != nil {
		return nil
	}
	typ, _ := raw["type"].(string)
	if typ == "" {
		return nil
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	text := str("delta", "text", "content", "message")
	result := make([]Event, 0, 2)
	switch typ {
	case "text", "message", "assistant", "content_block_delta", "agent_message":
		if text != "" {
			result = append(result, Event{Type: EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: text}})
		}
	case "tool_call", "tool_use", "command_started":
		result = append(result, Event{Type: EventToolCall, Payload: protocol.ToolCallParams{SessionID: sessionID, CallID: str("call_id", "id"), Tool: str("tool", "name"), Input: raw["input"]}})
	case "tool_result", "command_finished":
		status := str("status")
		if status == "" {
			status = "success"
		}
		result = append(result, Event{Type: EventToolResult, Payload: protocol.ToolResultParams{SessionID: sessionID, CallID: str("call_id", "id"), Status: status, Output: text}})
	case "error", "failed":
		result = append(result, Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: str("message", "error")}})
	case "usage", "turn.completed":
		u := protocol.UsageParams{SessionID: sessionID}
		if v, ok := raw["input_tokens"].(float64); ok {
			u.InputTokens = int64(v)
		}
		if v, ok := raw["output_tokens"].(float64); ok {
			u.OutputTokens = int64(v)
		}
		if v, ok := raw["total_tokens"].(float64); ok {
			u.TotalTokens = int64(v)
		}
		result = append(result, Event{Type: EventUsage, Payload: u})
	case "complete", "done", "turn_end":
		result = append(result, Event{Type: EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: str("reason")}})
	default:
		return nil
	}
	return result
}

func JSONEventError(line string) Event {
	return Event{Type: EventError, Payload: protocol.ErrorParams{Message: fmt.Sprintf("JSON inválido: %s", strings.TrimSpace(line))}}
}
