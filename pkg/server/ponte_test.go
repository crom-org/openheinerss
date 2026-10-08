package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

type espiaSrv struct {
	mu  sync.Mutex
	cfg harness.SessionConfig
	ch  chan harness.Event
}

func (e *espiaSrv) Name() string       { return "espia-srv" }
func (e *espiaSrv) Mode() harness.Mode { return harness.ModeCLI }
func (e *espiaSrv) ValidatePrerequisites(context.Context) harness.PrerequisiteResult {
	return harness.PrerequisiteResult{Satisfied: true}
}
func (e *espiaSrv) Start(_ context.Context, cfg harness.SessionConfig) error {
	e.cfg, e.ch = cfg, make(chan harness.Event, 8)
	return nil
}
func (e *espiaSrv) SendPrompt(_ context.Context, text string, _ []protocol.Attachment) error {
	if strings.HasPrefix(text, "/nope") {
		return harness.NoEquivalent("espia-srv", "nope", "use outro comando")
	}
	e.ch <- harness.RawEvent(e.cfg.SessionID, "espia-srv", "stderr", "recebi:"+text)
	e.ch <- harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{Reason: "completed"}}
	return nil
}
func (e *espiaSrv) RespondPermission(context.Context, string, bool, string) error { return nil }
func (e *espiaSrv) Events() <-chan harness.Event                                  { return e.ch }
func (e *espiaSrv) Stop() error                                                   { return nil }

func TestStdioHarnessArgsAgentRawEErroSemEquivalente(t *testing.T) {
	spy := &espiaSrv{}
	harness.Register("espia-srv", protocol.HarnessCatalogItem{ID: "espia-srv"}, func(harness.Mode) (harness.Harness, error) { return spy, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go func() { _ = server.NewStdioServer(session.NewManager(), inR, outW).Run(ctx); _ = outW.Close() }()
	sc := bufio.NewScanner(outR)
	send := func(line string) { _, _ = inW.Write([]byte(line + "\n")) }
	next := func() map[string]interface{} {
		if !sc.Scan() {
			t.Fatalf("sem resposta: %v", sc.Err())
		}
		var m map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	send(`{"jsonrpc":"2.0","id":1,"method":"session.create","params":{"harness":"espia-srv","mode":"cli","cwd":"` + t.TempDir() + `","options":{"harnessArgs":["--a","x,y=z","/q"]}}}`)
	resp := next()
	if resp["error"] != nil {
		t.Fatalf("session.create: %v", resp["error"])
	}
	sid := resp["result"].(map[string]interface{})["sessionId"].(string)
	if got := harness.HarnessArgs(spy.cfg.Options); strings.Join(got, "|") != "--a|x,y=z|/q" {
		t.Fatalf("harnessArgs chegou como %q", got)
	}

	send(`{"jsonrpc":"2.0","id":2,"method":"session.prompt","params":{"sessionId":"` + sid + `","text":"/x arg"}}`)
	var raw map[string]interface{}
	for i := 0; i < 4 && raw == nil; i++ {
		if m := next(); m["method"] == "agent.raw" {
			raw = m["params"].(map[string]interface{})
		}
	}
	if raw == nil || raw["line"] != "recebi:/x arg" || raw["stream"] != "stderr" || raw["harness"] != "espia-srv" {
		t.Fatalf("agent.raw não chegou: %v", raw)
	}

	send(`{"jsonrpc":"2.0","id":3,"method":"session.prompt","params":{"sessionId":"` + sid + `","text":"/nope"}}`)
	for i := 0; i < 4; i++ {
		m := next()
		if m["id"] == float64(3) {
			e, _ := m["error"].(map[string]interface{})
			if e == nil || !strings.Contains(e["message"].(string), "não aceita /nope") {
				t.Fatalf("erro sem equivalente não chegou claro: %v", m)
			}
			return
		}
	}
	t.Fatal("sem resposta ao /nope")
}
