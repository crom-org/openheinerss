package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// espiaHarness guarda o que recebeu e devolve um raw, um texto e o fim.
type espiaHarness struct {
	mu      sync.Mutex
	cfg     harness.SessionConfig
	prompts []string
	ch      chan harness.Event
}

func (e *espiaHarness) Name() string { return "espia-ponte" }
func (e *espiaHarness) Mode() harness.Mode {
	return harness.ModeCLI
}
func (e *espiaHarness) ValidatePrerequisites(context.Context) harness.PrerequisiteResult {
	return harness.PrerequisiteResult{Satisfied: true}
}
func (e *espiaHarness) Start(_ context.Context, cfg harness.SessionConfig) error {
	e.cfg, e.ch = cfg, make(chan harness.Event, 16)
	return nil
}
func (e *espiaHarness) SendPrompt(_ context.Context, text string, _ []protocol.Attachment) error {
	e.mu.Lock()
	e.prompts = append(e.prompts, text)
	e.mu.Unlock()
	e.ch <- harness.RawEvent(e.cfg.SessionID, "espia-ponte", "stdout", "linha-nao-mapeada")
	e.ch <- harness.Event{Type: harness.EventText, Payload: protocol.TextParams{Delta: "ok"}}
	e.ch <- harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{Reason: "completed"}}
	return nil
}
func (e *espiaHarness) RespondPermission(context.Context, string, bool, string) error { return nil }
func (e *espiaHarness) Events() <-chan harness.Event                                  { return e.ch }
func (e *espiaHarness) Stop() error                                                   { return nil }

func TestRodarRepassaHarnessArgsNaOrdemERawNoLog(t *testing.T) {
	spy := &espiaHarness{}
	harness.Register("espia-ponte", protocol.HarnessCatalogItem{ID: "espia-ponte"}, func(harness.Mode) (harness.Harness, error) { return spy, nil })
	root, agents := repoFixture(t)
	args := []string{"--flag", "a,b=c", "/x", "--outra"}
	res, err := Run(context.Background(), root, Options{Name: "ponte", Motor: "espia-ponte", PromptText: "/x faça", SemRegras: true, AgentsDir: agents, MaxAgents: 99, HarnessArgs: args})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	got := harness.HarnessArgs(spy.cfg.Options)
	if strings.Join(got, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("args = %q, quer %q", got, args)
	}
	if len(spy.prompts) != 1 || !strings.HasPrefix(spy.prompts[0], "/x faça") {
		t.Fatalf("prompt não chegou literal: %q", spy.prompts)
	}
	b, _ := os.ReadFile(filepath.Join(agents, "logs", "ponte.log"))
	if !strings.Contains(string(b), "[raw stdout] linha-nao-mapeada") {
		t.Fatalf("log sem o agent.raw: %s", b)
	}
}

func TestSecoMostraHarnessArgs(t *testing.T) {
	got := dryRunCommand(Options{Motor: "codex", HarnessArgs: []string{"--x", "a b", "/y"}})
	if !strings.Contains(got, "[args do harness, na ordem: --x 'a b' /y]") {
		t.Fatalf("seco sem os args: %s", got)
	}
}
