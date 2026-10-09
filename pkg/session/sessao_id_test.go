package session_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

// Os eventos agent.* do claude-code devem levar o sessionId do session.create, não o id nativo.
func TestEventosClaudeCodeUsamIdDoSessionCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "..", "harness", "claudecode", "testdata", "fake-claude.sh"), filepath.Join(d, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := session.NewManager()
	defer m.Close()
	ctx := context.Background()
	res, err := m.CreateSession(ctx, protocol.SessionCreateParams{Harness: "claude-code", Mode: "cli", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	type visto struct {
		metodo string
		params map[string]interface{}
	}
	eventos := make(chan visto, 64)
	m.SubscribeEvents(func(n protocol.Notification) {
		raw, _ := json.Marshal(n.Params)
		var p map[string]interface{}
		_ = json.Unmarshal(raw, &p)
		eventos <- visto{n.Method, p}
	})
	if _, err := m.PromptSession(ctx, protocol.SessionPromptParams{SessionID: res.SessionID, Text: "oi"}); err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for {
		select {
		case e := <-eventos:
			vistos++
			if e.params["sessionId"] != res.SessionID {
				t.Fatalf("%s com sessionId %v, esperado %s", e.metodo, e.params["sessionId"], res.SessionID)
			}
			if e.metodo != protocol.EventAgentThinking && e.params["nativeSessionId"] != "sess-fake" && e.metodo != protocol.EventAgentComplete {
				t.Fatalf("%s sem nativeSessionId: %v", e.metodo, e.params)
			}
			if e.metodo == protocol.EventAgentComplete {
				if vistos < 4 {
					t.Fatalf("poucos eventos: %d", vistos)
				}
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("eventos não chegaram")
		}
	}
}
