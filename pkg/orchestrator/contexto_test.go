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

// fakeCtx é um harness que, a cada prompt, informa o uso (tokens de entrada) e termina o turno.
type fakeCtx struct {
	mu      sync.Mutex
	eventos chan harness.Event
	opts    map[string]interface{}
	tokens  int64
	reg     *regCtx
}

type regCtx struct {
	mu       sync.Mutex
	prompts  []string
	opcoes   []map[string]interface{}
	sessoes  int
	tokens   int64
	vezesUso int // quantos eventos de uso por turno
}

func (r *regCtx) snap() ([]string, []map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.prompts...), append([]map[string]interface{}(nil), r.opcoes...)
}

func registrarFakeCtx(t *testing.T, nome string, tokens int64, usos int) *regCtx {
	t.Helper()
	r := &regCtx{tokens: tokens, vezesUso: usos}
	harness.Register(nome, protocol.HarnessCatalogItem{ID: nome, DisplayName: nome}, func(harness.Mode) (harness.Harness, error) {
		r.mu.Lock()
		r.sessoes++
		r.mu.Unlock()
		return &fakeCtx{eventos: make(chan harness.Event, 16), reg: r, tokens: tokens}, nil
	})
	return r
}

func (f *fakeCtx) Name() string       { return "fake" }
func (f *fakeCtx) Mode() harness.Mode { return harness.ModeMock }
func (f *fakeCtx) ValidatePrerequisites(context.Context) harness.PrerequisiteResult {
	return harness.PrerequisiteResult{Satisfied: true}
}
func (f *fakeCtx) Start(_ context.Context, cfg harness.SessionConfig) error {
	f.opts = cfg.Options
	return nil
}
func (f *fakeCtx) SendPrompt(_ context.Context, text string, _ []protocol.Attachment) error {
	f.reg.mu.Lock()
	f.reg.prompts = append(f.reg.prompts, text)
	f.reg.opcoes = append(f.reg.opcoes, f.opts)
	f.reg.mu.Unlock()
	for i := 0; i < f.reg.vezesUso; i++ {
		f.eventos <- harness.Event{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: "s", InputTokens: f.tokens, OutputTokens: 10}}
	}
	f.eventos <- harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: "s", Reason: "finished"}}
	return nil
}
func (f *fakeCtx) RespondPermission(context.Context, string, bool, string) error { return nil }
func (f *fakeCtx) Events() <-chan harness.Event                                  { return f.eventos }
func (f *fakeCtx) Stop() error                                                   { return nil }
func (f *fakeCtx) ResumeID() string                                              { return "sessao-nativa-1" }

func ambienteCtx(t *testing.T, yamlProjeto string) (root, agents string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENHEINERSS_CONFIG", "")
	root, agents = repoFixture(t)
	if yamlProjeto != "" {
		dir := filepath.Join(root, ".openheinerss")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlProjeto), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, agents
}

func TestContextoAvisoUmaVezPorSessao(t *testing.T) {
	root, agents := ambienteCtx(t, "contexto:\n  padrao: {limite_tokens: 1000, acao: aviso}\n")
	reg := registrarFakeCtx(t, "ctx-fake-aviso", 5000, 3) // 3 eventos de uso acima do limite no mesmo turno
	evlog := filepath.Join(t.TempDir(), "eventos.log")
	var eventos []Evento
	res, err := Run(context.Background(), root, Options{Name: "a1", Motor: "ctx-fake-aviso", AgentsDir: agents, MaxAgents: 99, PromptText: "faça", EventLog: evlog,
		OnEvent: func(e Evento) { eventos = append(eventos, e) }})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	log := mustRead(t, res.LogFile)
	if n := strings.Count(log, "[contexto] 5000 tokens >= limite 1000 (origem: projeto)"); n != 1 {
		t.Fatalf("aviso %d vezes:\n%s", n, log)
	}
	prompts, _ := reg.snap()
	if len(prompts) != 1 || reg.sessoes != 1 {
		t.Fatalf("aviso não reinicia: %d prompts, %d sessões", len(prompts), reg.sessoes)
	}
	n := 0
	for _, e := range eventos {
		if e.Tipo == EvContexto {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("eventos de contexto: %d", n)
	}
	if ev := mustRead(t, evlog); strings.Count(ev, "orq.contexto a1 tokens=5000 limite=1000 acao=aviso origem=projeto") != 1 {
		t.Fatalf("log de eventos:\n%s", ev)
	}
	if strings.Contains(mustRead(t, res.MetaFile), "reinicios_contexto") {
		t.Fatalf("meta com reinícios: %s", mustRead(t, res.MetaFile))
	}
}

func TestContextoNovaSessaoReiniciaSemSessaoNativaEParaEm3(t *testing.T) {
	root, agents := ambienteCtx(t, "contexto:\n  harnesses:\n    ctx-fake-nova: {limite_tokens: 1000, acao: nova-sessao}\n")
	reg := registrarFakeCtx(t, "ctx-fake-nova", 5000, 1)
	res, err := Run(context.Background(), root, Options{Name: "a2", Motor: "ctx-fake-nova", AgentsDir: agents, MaxAgents: 99, PromptText: "tarefa original", Attempts: 1})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	prompts, opcoes := reg.snap()
	if len(prompts) != 4 || reg.sessoes != 4 { // 1 inicial + 3 reinícios
		t.Fatalf("esperava 4 sessões, vieram %d prompts / %d sessões", len(prompts), reg.sessoes)
	}
	for i, p := range prompts {
		if !strings.Contains(p, "tarefa original") {
			t.Fatalf("prompt %d sem o original: %q", i, p)
		}
		if (i > 0) != strings.Contains(p, "CONTINUAÇÃO") {
			t.Fatalf("prompt %d: continuação inesperada/ausente: %q", i, p)
		}
		if _, ok := opcoes[i]["claude_session_id"]; ok {
			t.Fatalf("reinício %d reaproveitou sessão nativa: %v", i, opcoes[i])
		}
		if _, ok := opcoes[i]["codex_session_id"]; ok {
			t.Fatalf("reinício %d reaproveitou sessão nativa: %v", i, opcoes[i])
		}
	}
	log := mustRead(t, res.LogFile)
	if n := strings.Count(log, "[contexto] 5000 tokens >= limite 1000 (origem: projeto)"); n != 4 {
		t.Fatalf("um aviso por sessão (4), vieram %d:\n%s", n, log)
	}
	if !strings.Contains(log, "máximo de 3 reinícios atingido; só avisando") {
		t.Fatalf("sem aviso do máximo:\n%s", log)
	}
	if !strings.Contains(mustRead(t, res.MetaFile), `"reinicios_contexto":3`) {
		t.Fatalf("meta: %s", mustRead(t, res.MetaFile))
	}
	if res.Attempts != 1 {
		t.Fatalf("reinício não conta como tentativa: %d", res.Attempts)
	}
}

func TestContextoFlagVenceEDesliga(t *testing.T) {
	root, agents := ambienteCtx(t, "contexto:\n  padrao: {limite_tokens: 1000, acao: nova-sessao}\n")
	reg := registrarFakeCtx(t, "ctx-fake-flag", 5000, 1)
	zero := 0
	res, err := Run(context.Background(), root, Options{Name: "a3", Motor: "ctx-fake-flag", AgentsDir: agents, MaxAgents: 99, PromptText: "x", LimiteContexto: &zero})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if prompts, _ := reg.snap(); len(prompts) != 1 || strings.Contains(mustRead(t, res.LogFile), "[contexto]") {
		t.Fatalf("--limite-contexto 0 deveria desligar")
	}
	limite := 4000
	res, err = Run(context.Background(), root, Options{Name: "a4", Motor: "ctx-fake-flag", AgentsDir: agents, MaxAgents: 99, PromptText: "x", LimiteContexto: &limite, AcaoContexto: "aviso"})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if log := mustRead(t, res.LogFile); !strings.Contains(log, "limite 4000 (origem: flag)") || reg.sessoes != 2 {
		t.Fatalf("flag deveria vencer (aviso, sem reinício): sessões=%d\n%s", reg.sessoes, log)
	}
}

func TestContextoArquivoInvalidoFalhaComErroClaro(t *testing.T) {
	root, agents := ambienteCtx(t, "contexto:\n  padrao: {acao: voar}\n")
	res, err := Run(context.Background(), root, Options{Name: "a5", Motor: "mock", AgentsDir: agents, MaxAgents: 99, PromptText: "x"})
	if err == nil || !strings.Contains(err.Error(), "acao") || !strings.Contains(err.Error(), "config.yaml") {
		t.Fatalf("erro: %v", err)
	}
	if res.Code == 0 {
		t.Fatalf("código %d", res.Code)
	}
}
