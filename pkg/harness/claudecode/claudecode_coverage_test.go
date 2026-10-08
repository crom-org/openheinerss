package claudecode

import (
	"context"
	"encoding/json"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeCodeStreamNormalizaEventosEGuardaSessao(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	for _, line := range []string{
		`{"type":"system","subtype":"init","session_id":"sess-1"}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"pensando"},{"type":"text","text":"oi"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"pwd"}}]},"session_id":"sess-1"}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]},"session_id":"sess-1"}`,
		`{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.12,"usage":{"input_tokens":2,"output_tokens":3},"session_id":"sess-1"}`,
	} {
		c.parseCLIEvent([]byte(line), "fallback")
	}
	if got := c.ResumeID(); got != "sess-1" {
		t.Fatalf("sessão salva: %q", got)
	}
	var types []harness.EventType
	var cost float64
	for len(c.events) > 0 {
		ev := <-c.events
		types = append(types, ev.Type)
		if usage, ok := ev.Payload.(protocol.UsageParams); ok {
			cost = usage.CostUSD
		}
	}
	want := []harness.EventType{harness.EventThinking, harness.EventText, harness.EventToolCall, harness.EventToolResult, harness.EventUsage, harness.EventComplete}
	if len(types) != len(want) {
		t.Fatalf("tipos: got %v want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("tipos: got %v want %v", types, want)
		}
	}
	if cost != 0.12 {
		t.Fatalf("custo: %v", cost)
	}
}

func TestClaudeCodeCotaViraErro(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	c.parseCLIEvent([]byte(`{"type":"result","subtype":"error","is_error":true,"result":"usage limit reached","session_id":"sess-q"}`), "fallback")
	seenError, seenComplete := false, false
	for len(c.events) > 0 {
		ev := <-c.events
		seenError = seenError || ev.Type == harness.EventError
		seenComplete = seenComplete || ev.Type == harness.EventComplete
	}
	if !seenError || !seenComplete {
		t.Fatal("cota não emitiu erro e fim")
	}
}

func TestClaudeCodePermissoesDoRodar(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeCLI)
	c.cfg.Options = map[string]interface{}{"rodar": true}
	if got := c.cliPermissionMode(); got != "bypassPermissions" {
		t.Fatalf("modo padrão do rodar: %q", got)
	}
	c.cfg.Options["permissoes"] = "perguntar"
	if got := c.cliPermissionMode(); got != "manual" {
		t.Fatalf("modo perguntar: %q", got)
	}
}

func TestClaudeCodeAmostraRealValida(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "claude-stream-ok.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var value map[string]interface{}
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClaudeCodeCLIConfiguraContaEFluxo(t *testing.T) {
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-claude.sh"), filepath.Join(d, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := NewClaudeCodeHarness(harness.ModeCLI)
	if !c.ValidatePrerequisites(context.Background()).Satisfied {
		t.Fatal("claude ausente")
	}
	if err := c.Start(context.Background(), harness.SessionConfig{SessionID: "c1", CWD: t.TempDir(), Provider: "conta2", Model: "sonnet", Env: map[string]string{"CLAUDE_EXTRA": "ok"}}); err != nil {
		t.Fatal(err)
	}
	if !hasEnv(c.env, "CLAUDE_CONFIG_DIR=") {
		t.Fatal("CLAUDE_CONFIG_DIR não propagado")
	}
	if err := c.SendPrompt(context.Background(), "faça", nil); err != nil {
		t.Fatal(err)
	}
	text, done := false, false
	deadline := time.After(2 * time.Second)
	for !done {
		select {
		case ev := <-c.Events():
			text = text || ev.Type == harness.EventText
			done = ev.Type == harness.EventComplete
		case <-deadline:
			t.Fatal("timeout")
		}
	}
	if !text {
		t.Fatal("sem texto")
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeCodeMensagensSDKParser(t *testing.T) {
	c := NewClaudeCodeHarness(harness.ModeSDK)
	c.cfg.SessionID = "s"
	for _, msg := range []string{"agent.thinking", "agent.text", "agent.permission_request", "agent.complete", "agent.error", "desconhecido"} {
		c.handleSDKMessage(msg, map[string]interface{}{"delta": "d", "requestId": "r", "tool": "Bash", "command": "ls", "risk": "high", "reason": "fim", "message": "falha"})
	}
	c.readEvents(strings.NewReader("texto puro\n{\"method\":\"agent.text\",\"params\":{\"delta\":\"json\"}}\n"))
	if len(c.events) < 7 {
		t.Fatalf("eventos parser: %d", len(c.events))
	}
	if err := c.RespondPermission(context.Background(), "r", true, ""); err == nil {
		t.Fatal("SDK sem stdin deveria falhar")
	}
	_ = protocol.EventAgentText
}

func TestClaudeCodeErroDeProcesso(t *testing.T) {
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-claude.sh"), filepath.Join(d, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FAIL", "1")
	c := NewClaudeCodeHarness(harness.ModeCLI)
	if err := c.Start(context.Background(), harness.SessionConfig{SessionID: "e", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := c.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-c.Events():
			if ev.Type == harness.EventComplete {
				return
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}

func hasEnv(env []string, prefix string) bool {
	for _, v := range env {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}
