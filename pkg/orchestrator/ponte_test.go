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

// Auditoria 26: com regras padrão o stdin começava com "--- REGRAS PADRÃO", e o /comando deixava de ser comando.
func TestRodarComRegrasPadraoMantemSlashNoComeco(t *testing.T) {
	spy := &espiaHarness{}
	harness.Register("espia-ponte", protocol.HarnessCatalogItem{ID: "espia-ponte"}, func(harness.Mode) (harness.Harness, error) { return spy, nil })
	root, agents := repoFixture(t)
	res, err := Run(context.Background(), root, Options{Name: "slash-padrao", Motor: "espia-ponte", PromptText: "/x literal", AgentsDir: agents, MaxAgents: 99})
	if err != nil || res.Code != 0 {
		t.Fatalf("err=%v código=%d", err, res.Code)
	}
	if len(spy.prompts) != 1 || !strings.HasPrefix(spy.prompts[0], "/x literal\n\n") || !strings.Contains(spy.prompts[0], defaultPromptRules) {
		t.Fatalf("o /comando precisa vir primeiro e as regras depois: %q", spy.prompts)
	}
	if strings.Contains(spy.prompts[0], "\x00") {
		t.Fatalf("separador interno vazou: %q", spy.prompts[0])
	}
	// Prompt comum continua com as regras antes.
	spy.prompts = nil
	if _, err := Run(context.Background(), root, Options{Name: "texto-padrao", Motor: "espia-ponte", PromptText: "faça x", AgentsDir: agents, MaxAgents: 99}); err != nil {
		t.Fatal(err)
	}
	if len(spy.prompts) != 1 || !strings.HasPrefix(spy.prompts[0], defaultPromptRules) {
		t.Fatalf("prompt comum mudou: %q", spy.prompts)
	}
}

func TestRegrasDeSlashPorHarness(t *testing.T) {
	_, agents := repoFixture(t)
	p, err := readPromptOptions(agents, "missao-x", "", "/review agora", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	envio, regras := separarRegras(p)
	if envio != "/review agora" || !strings.Contains(regras, defaultPromptRules) || !strings.Contains(regras, "MISSÃO SOMENTE LEITURA") {
		t.Fatalf("envio=%q regras=%q", envio, regras)
	}
	novo := func() *harness.SessionConfig { return &harness.SessionConfig{Options: map[string]interface{}{}} }

	cfg := novo()
	if got, _ := entregarRegras("claude-code", envio, regras, "", cfg); got != envio || cfg.SystemPrompt != regras {
		t.Fatalf("claude: envio=%q system=%q", got, cfg.SystemPrompt)
	}
	cfg = novo()
	cfg.Options["config"] = []string{"a=1"}
	if got, _ := entregarRegras("codex", envio, "linha \"1\"\n<b>", "", cfg); got != envio {
		t.Fatalf("codex: envio=%q", got)
	}
	if c := harness.OptionStrings(cfg.Options, "config"); len(c) != 2 || c[0] != "a=1" || c[1] != `developer_instructions="linha \"1\"\n<b>"` {
		t.Fatalf("codex: config=%q", c)
	}
	cfg = novo()
	arq := filepath.Join(t.TempDir(), "r.md")
	if got, _ := entregarRegras("aider", envio, regras, arq, cfg); got != envio {
		t.Fatalf("aider: envio=%q", got)
	}
	if b, _ := os.ReadFile(arq); !strings.Contains(string(b), defaultPromptRules) || harness.OpcaoLista(cfg.Options, "read_files")[0] != arq {
		t.Fatalf("aider: arquivo %q opções %v", b, cfg.Options)
	}
	cfg = novo()
	if got, _ := entregarRegras("opencode", envio, regras, "", cfg); got != envio+"\n\n"+regras {
		t.Fatalf("opencode: %q", got)
	}
	if got, _ := entregarRegras("claude-code", "faça", "", "", novo()); got != "faça" {
		t.Fatalf("sem regras: %q", got)
	}
}
