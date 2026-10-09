package limites

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

func TestAtualizarConsultaAtivaCacheiaSemCredencialNoResultado(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, path := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".codex")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"fake-claude"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"tokens":{"access_token":"fake-codex"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "" {
			t.Error("faltou autorização")
		}
		if r.URL.Path == "/claude" {
			w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.25")
			_, _ = w.Write([]byte(`{"rate_limits":{"five_hour":{"used_percentage":25}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"rate_limits":{"primary":{"used_percent":33,"window_minutes":300,"resets_at":4102444800}}}`))
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/claude")
	t.Setenv("OPENHEINERSS_CODEX_USAGE_URL", srv.URL+"/codex")
	cache := filepath.Join(home, "cache")
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("chamadas = %d, esperadas pelo menos 2", calls)
	}
	primeiraChamada := calls
	porNome := map[string]Instancia{}
	for _, i := range r.Instancias {
		porNome[i.Nome] = i
	}
	if porNome["claude-code"].Fonte != "cabeçalhos" || len(porNome["claude-code"].Janelas) == 0 {
		t.Fatalf("claude ativo: %+v", porNome["claude-code"])
	}
	if porNome["codex"].Fonte != "consulta-ativa" && porNome["codex"].Fonte != "cabeçalhos" {
		t.Fatalf("codex ativo: %+v", porNome["codex"])
	}
	b, err := os.ReadFile(filepath.Join(cache, nomeCache("claude-code")))
	if err != nil {
		t.Fatal(err)
	}
	var semSegredo Instancia
	if json.Unmarshal(b, &semSegredo) != nil || strings.Contains(string(b), "fake-") {
		t.Fatal("cache contém credencial")
	}
	r, err = Atualizar(context.Background(), AtualizarOpcoes{CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	if calls != primeiraChamada {
		t.Fatalf("cache não respeitado: chamadas = %d", calls)
	}
}

func TestObterLêCodexEClaudePorInstancia(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := filepath.Join(home, "codex-conta")
	dir := filepath.Join(codexHome, "sessions", "2026", "10", "08")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "codex.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), b, 0644); err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, "claude-conta")
	if err := os.MkdirAll(filepath.Join(home, ".config", "crom-painel"), 0755); err != nil {
		t.Fatal(err)
	}
	status := `{"em":1791457200000,"rate_limits":{"five_hour":{"used_percentage":12,"resets_at":1791459000},"seven_day":{"used_percentage":34,"resets_at":1792058400}}}`
	if err := os.WriteFile(filepath.Join(home, ".config", "crom-painel", "statusline-conta-teste-limites.json"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "codex-teste-limites", Base: "codex", Env: map[string]string{"CODEX_HOME": codexHome}}); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-teste-limites", Base: "claude-code", Env: map[string]string{"CLAUDE_CONFIG_DIR": claudeDir}}); err != nil {
		t.Fatal(err)
	}
	r := Obter()
	var codex, claude *Instancia
	for i := range r.Instancias {
		if r.Instancias[i].Nome == "codex-teste-limites" {
			codex = &r.Instancias[i]
		}
		if r.Instancias[i].Nome == "conta-teste-limites" {
			claude = &r.Instancias[i]
		}
	}
	if codex == nil || len(codex.Janelas) != 1 || codex.Janelas[0].Percentual != 81 {
		t.Fatalf("codex inesperado: %+v", codex)
	}
	if claude == nil || len(claude.Janelas) != 1 || claude.Janelas[0].Percentual != 34 {
		t.Fatalf("claude inesperado: %+v", claude)
	}
	if strings.Contains(status, "token") {
		t.Fatal("fixture de statusline contém token")
	}
}

func TestContaClaudePelaPasta(t *testing.T) {
	casos := map[[2]string]string{
		{"claude-code", ""}:                         "conta1",
		{"x", "/home/u/.claude"}:                    "conta1",
		{"claude-conta2", "/home/u/.claude-conta2"}: "conta2",
		{"outra", "/srv/claude-custom"}:             "outra",
	}
	for in, want := range casos {
		if got := contaClaude(in[0], in[1]); got != want {
			t.Fatalf("contaClaude(%q,%q)=%q, quero %q", in[0], in[1], got, want)
		}
	}
}

func TestMaiorPercentualIgnoraJanelaJaRenovada(t *testing.T) {
	agora := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	i := Instancia{Janelas: []Janela{
		{Nome: "5 h", Percentual: 100, ReiniciaEm: agora.Add(-time.Hour).Format(time.RFC3339)},
		{Nome: "semanal", Percentual: 40, ReiniciaEm: agora.Add(24 * time.Hour).Format(time.RFC3339)},
	}}
	if p, ok := maiorPercentual(i, agora); !ok || p != 40 {
		t.Fatalf("percentual = %v (ok=%v), esperado 40", p, ok)
	}
	if _, ok := maiorPercentual(Instancia{Janelas: i.Janelas[:1]}, agora); ok {
		t.Fatal("só janela vencida não deve contar como leitura")
	}
}

func TestObterRemoveJanelaVencidaDaExibicao(t *testing.T) {
	agora := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	janelas := janelasVigentes([]Janela{
		{Nome: "vencida", Percentual: 99, ReiniciaEm: agora.Add(-time.Minute).Format(time.RFC3339)},
		{Nome: "vigente", Percentual: 12, ReiniciaEm: agora.Add(time.Hour).Format(time.RFC3339)},
	}, agora)
	if len(janelas) != 1 || janelas[0].Nome != "vigente" || janelas[0].Percentual != 12 {
		t.Fatalf("janelas exibidas: %+v", janelas)
	}
}

func TestObterDescobreContasLocaisSemDadoEOrdenaEstavelmente(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, nome := range []string{".claude-conta12", ".claude-conta2", ".claude"} {
		if err := os.Mkdir(filepath.Join(home, nome), 0755); err != nil {
			t.Fatal(err)
		}
	}

	primeira := Obter()
	segunda := Obter()
	if len(primeira.Instancias) != len(segunda.Instancias) {
		t.Fatalf("quantidade instável: %d e %d", len(primeira.Instancias), len(segunda.Instancias))
	}
	for i := range primeira.Instancias {
		if primeira.Instancias[i].Nome != segunda.Instancias[i].Nome {
			t.Fatalf("ordem instável: %+v e %+v", primeira.Instancias, segunda.Instancias)
		}
	}
	porNome := map[string]Instancia{}
	for _, instancia := range primeira.Instancias {
		porNome[instancia.Nome] = instancia
	}
	for _, nome := range []string{"claude-code", "claude-conta2", "claude-conta12"} {
		i, ok := porNome[nome]
		if !ok || i.Nota == "" {
			t.Fatalf("conta %q sem nota de dado ausente: %+v", nome, i)
		}
	}
}
