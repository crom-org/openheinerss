package harness

import (
	"github.com/crom-org/openheinerss/pkg/protocol"
	"testing"
)

func TestParseJSONEventFormatosCLI(t *testing.T) {
	cases := []struct {
		line string
		typ  EventType
	}{
		{`{"type":"text","delta":"oi"}`, EventText},
		{`{"type":"tool_call","call_id":"c1","tool":"shell","input":{"command":"ls"}}`, EventToolCall},
		{`{"type":"tool_result","call_id":"c1","status":"success","output":"ok"}`, EventToolResult},
		{`{"type":"error","message":"falhou"}`, EventError},
		{`{"type":"usage","input_tokens":2,"output_tokens":3,"total_tokens":5}`, EventUsage},
		{`{"type":"complete","reason":"done"}`, EventComplete},
	}
	for _, tc := range cases {
		got := ParseJSONEvent(tc.line, "s")
		if len(got) != 1 || got[0].Type != tc.typ {
			t.Errorf("%s: %#v", tc.line, got)
		}
	}
	if ParseJSONEvent("texto normal", "s") != nil {
		t.Fatal("texto simples não deveria ser JSON")
	}
	usage := ParseJSONEvent(`{"type":"usage","input_tokens":2,"output_tokens":3,"total_tokens":5}`, "s")[0].Payload.(protocol.UsageParams)
	if usage.TotalTokens != 5 {
		t.Fatalf("uso: %#v", usage)
	}
}
