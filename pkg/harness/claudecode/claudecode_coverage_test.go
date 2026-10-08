package claudecode

import (
	"context"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
