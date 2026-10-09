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

func TestAtualizarNaoReutilizaCacheCodexSemJanelas(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codex, "auth.json"), []byte(`{"tokens":{"access_token":"token-anonimo"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"rate_limits":{"primary":{"used_percent":39,"window_minutes":300,"resets_at":4102444800}}}`))
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CODEX_USAGE_URL", srv.URL)
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := gravarCache(filepath.Join(cache, nomeCache("codex")), Instancia{
		Nome: "codex", Base: "codex", DadoEm: time.Now().Format(time.RFC3339), Fonte: "consulta-ativa",
	}); err != nil {
		t.Fatal(err)
	}
	r, err := Atualizar(context.Background(), AtualizarOpcoes{CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cache sem janelas foi reutilizado: chamadas=%d", calls)
	}
	for _, i := range r.Instancias {
		if i.Nome == "codex" && (i.Fonte != "consulta-ativa" || len(i.Janelas) != 1 || i.Janelas[0].Percentual != 39) {
			t.Fatalf("consulta ativa não atualizou Codex: %+v", i)
		}
	}
}

func TestConsultaCodexWhamLêFixtureEEnviaAccountID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "wham-usage-real.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codex, "auth.json"), []byte(`{"account_id":"acct-anonimo","tokens":{"access_token":"token-anonimo"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("ChatGPT-Account-Id")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CODEX_USAGE_URL", srv.URL)
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: filepath.Join(t.TempDir(), "cache"), HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var codexResult Instancia
	for _, i := range r.Instancias {
		if i.Nome == "codex" {
			codexResult = i
		}
	}
	if gotID != "acct-anonimo" {
		t.Fatalf("account id não enviado: %q", gotID)
	}
	porJanela := map[string]Janela{}
	for _, janela := range codexResult.Janelas {
		porJanela[janela.Nome] = janela
	}
	if len(porJanela) != 2 || porJanela["5h"].Percentual != 37 || porJanela["semana"].Percentual != 70 || porJanela["5h"].VoltaEm == "" || porJanela["semana"].VoltaEm == "" {
		t.Fatalf("fixture wham não interpretado: %+v", codexResult)
	}
	if codexResult.ContaIDFonte != "id-real" {
		t.Fatalf("fonte da identidade: %+v", codexResult)
	}
}

func prepararClaudeAtivo(t *testing.T, cred string) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte(cred), 0600); err != nil {
		t.Fatal(err)
	}
	return home, path
}

func TestAtualizarClaudeRenovaOAuth401TokenNaoExpirado(t *testing.T) {
	prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"velho","refreshToken":"renovar","expiresAt":4102444800000}}`)
	var usageCalls, refreshCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			usageCalls++
			if r.Header.Get("Authorization") == "Bearer velho" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.12")
			_, _ = w.Write([]byte(`{}`))
		case "/token":
			refreshCalls++
			_, _ = w.Write([]byte(`{"access_token":"novo","expires_in":3600}`))
		}
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/usage")
	t.Setenv("OPENHEINERSS_CLAUDE_OAUTH_TOKEN_URL", srv.URL+"/token")
	if _, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: filepath.Join(t.TempDir(), "cache"), HTTPClient: srv.Client()}); err != nil {
		t.Fatal(err)
	}
	if usageCalls != 2 || refreshCalls != 1 {
		t.Fatalf("chamadas usage=%d refresh=%d", usageCalls, refreshCalls)
	}
}

func TestNomesCabecalhosAnthropicSoNomes(t *testing.T) {
	h := http.Header{}
	h.Set("Anthropic-Ratelimit-Foo", "segredo-valor")
	h.Set("Content-Type", "x")
	got := nomesCabecalhosAnthropic(h)
	if got != "anthropic-ratelimit-foo" || strings.Contains(got, "segredo") {
		t.Fatalf("got %q", got)
	}
}

func TestAtualizarClaudeRenovaOAuth401EGravaMesmoFormato(t *testing.T) {
	_, credPath := prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"velho","refreshToken":"renovar","expiresAt":1,"subscriptionType":"pro"}}`)
	var usageCalls, refreshCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			usageCalls++
			if r.Header.Get("Authorization") == "Bearer velho" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.12")
			_, _ = w.Write([]byte(`{"rate_limits":{"five_hour":{"used_percentage":12,"resets_at":4102444800}}}`))
		case "/token":
			refreshCalls++
			if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") == "" {
				t.Fatalf("formulário OAuth inválido")
			}
			_, _ = w.Write([]byte(`{"access_token":"novo","refresh_token":"novo-refresh","expires_in":3600}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/usage")
	t.Setenv("OPENHEINERSS_CLAUDE_OAUTH_TOKEN_URL", srv.URL+"/token")
	resultado, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: filepath.Join(t.TempDir(), "cache"), HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if usageCalls != 1 || refreshCalls != 1 { // expirado: renova antes, sem gastar chamada com 401
		t.Fatalf("chamadas usage=%d refresh=%d", usageCalls, refreshCalls)
	}
	if resultado.Instancias[0].Fonte != "cabeçalhos" {
		t.Fatalf("fonte: %+v", resultado.Instancias[0])
	}
	b, err := os.ReadFile(credPath)
	if err != nil || !strings.Contains(string(b), "novo") || strings.Contains(string(b), "velho") {
		t.Fatalf("credencial não foi renovada: %v", err)
	}
	if st, err := os.Stat(credPath); err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("modo da credencial: %v", err)
	}
}

func TestAtualizarClaude401FalhaMantemFontePassiva(t *testing.T) {
	home, _ := prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"velho","refreshToken":"inválido"}}`)
	if err := os.MkdirAll(filepath.Join(home, ".config", "crom-painel"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "crom-painel", "statusline-conta1.json"), []byte(`{"em":4102444000000,"rate_limits":{"five_hour":{"used_percentage":31}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usage" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/usage")
	t.Setenv("OPENHEINERSS_CLAUDE_OAUTH_TOKEN_URL", srv.URL+"/token")
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: filepath.Join(t.TempDir(), "cache"), HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if r.Instancias[0].Fonte != "statusline" || !strings.Contains(r.Instancias[0].Nota, "renovação OAuth") {
		t.Fatalf("fonte passiva esperada: %+v", r.Instancias)
	}
}

func TestAtualizar429RespeitaRetryAfterEUsaCache(t *testing.T) {
	_, _ = prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"bom"}}`)
	var calls int
	got429 := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got429 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"rate_limits":{"five_hour":{"used_percentage":22}}}`))
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL)
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()}); err != nil {
		t.Fatal(err)
	}
	got429 = true
	t.Setenv("OPENHEINERSS_CLAUDE_MESSAGES_URL", srv.URL)
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	chamadasDepoisDo429 := calls
	if calls < 2 || r.Instancias[0].Fonte != "cache (429)" || len(r.Instancias[0].Janelas) != 1 {
		t.Fatalf("429 não usou cache: chamadas=%d resultado=%+v", calls, r.Instancias)
	}
	_, err = Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil || calls != chamadasDepoisDo429 {
		t.Fatalf("repetiu antes do retry-after: chamadas=%d erro=%v", calls, err)
	}
}

func TestAtualizar429ReservaClaudePelosCabecalhosUmaVez(t *testing.T) {
	_, _ = prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"bom"}}`)
	var usageCalls, messageCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			usageCalls++
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/messages":
			messageCalls++
			if r.Method != http.MethodPost || r.Header.Get("Authorization") == "" {
				t.Fatalf("reserva não foi POST autenticado")
			}
			w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.27")
			w.Header().Set("anthropic-ratelimit-unified-7d-utilization", "0.41")
			w.Header().Set("anthropic-ratelimit-unified-5h-reset", "4102444800")
			_, _ = w.Write([]byte(`{"id":"msg-anonimo","content":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/usage")
	t.Setenv("OPENHEINERSS_CLAUDE_MESSAGES_URL", srv.URL+"/messages")
	cache := filepath.Join(t.TempDir(), "cache")
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var got Instancia
	for _, i := range r.Instancias {
		if i.Nome == "claude-code" {
			got = i
		}
	}
	if got.Fonte != "cabeçalhos" || len(got.Janelas) != 2 || got.Janelas[0].Percentual != 27 || got.Janelas[1].Percentual != 41 {
		t.Fatalf("reserva não virou limites: %+v", got)
	}
	if got.Janelas[0].VoltaEm == "" || !strings.Contains(got.Nota, "max_tokens=1") {
		t.Fatalf("reserva sem data/custo documentado: %+v", got)
	}
	if _, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()}); err != nil {
		t.Fatal(err)
	}
	if usageCalls != 1 || messageCalls != 1 {
		t.Fatalf("reserva repetida durante Retry-After: usage=%d messages=%d", usageCalls, messageCalls)
	}
}

func TestAtualizar429EmBloqueioTentaReservaClaudeSeCacheDeCabecalhosVenceu(t *testing.T) {
	_, _ = prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"bom"}}`)
	var usageCalls, messageCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			usageCalls++
			t.Fatalf("não deveria consultar usage durante Retry-After")
		case "/messages":
			messageCalls++
			w.Header().Set("anthropic-ratelimit-unified-5h-utilization", "0.39")
			_, _ = w.Write([]byte(`{"id":"msg-anonimo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL+"/usage")
	t.Setenv("OPENHEINERSS_CLAUDE_MESSAGES_URL", srv.URL+"/messages")
	cache := filepath.Join(t.TempDir(), "cache")
	cachePath := filepath.Join(cache, nomeCache("claude-code"))
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := gravarCache(cachePath, Instancia{
		Nome: "claude-code", Base: "claude-code", DadoEm: time.Now().Add(-6 * time.Minute).Format(time.RFC3339),
		Fonte: "cabeçalhos", Janelas: []Janela{{Nome: "5h", Percentual: 12}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := gravarRetryAte(retryPath(cachePath), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if usageCalls != 0 || messageCalls != 1 {
		t.Fatalf("reserva durante bloqueio: usage=%d messages=%d", usageCalls, messageCalls)
	}
	for _, i := range r.Instancias {
		if i.Nome == "claude-code" && (i.Fonte != "cabeçalhos" || len(i.Janelas) != 1 || i.Janelas[0].Percentual != 39) {
			t.Fatalf("reserva não substituiu cache vencido: %+v", i)
		}
	}
}

func TestAtualizar429RecalculaIdadeDoCache(t *testing.T) {
	_, _ = prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"bom"}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL)
	cache := filepath.Join(t.TempDir(), "cache")
	cachePath := filepath.Join(cache, nomeCache("claude-code"))
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	dadoEm := time.Now().Add(-36 * time.Minute).Truncate(time.Second)
	if err := gravarCache(cachePath, Instancia{Nome: "claude-code", Base: "claude-code", DadoEm: dadoEm.Format(time.RFC3339), Fonte: "consulta-ativa", Janelas: []Janela{{Nome: "5h", Percentual: 22}}}); err != nil {
		t.Fatal(err)
	}
	if err := gravarRetryAte(retryPath(cachePath), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: cache, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if r.Instancias[0].IdadeSegundos < 35*60 || r.Instancias[0].IdadeSegundos > 37*60 {
		t.Fatalf("idade do cache não recalculada: %+v", r.Instancias[0])
	}
}

func TestAtualizarNomesEVoltaEm(t *testing.T) {
	_, _ = prepararClaudeAtivo(t, `{"claudeAiOauth":{"accessToken":"bom"}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rate_limits":{"five_hour":{"used_percentage":22,"resets_at":4102444800},"seven_day":{"used_percentage":44,"resets_at":4102448400}}}`))
	}))
	defer srv.Close()
	t.Setenv("OPENHEINERSS_CLAUDE_USAGE_URL", srv.URL)
	r, err := Atualizar(context.Background(), AtualizarOpcoes{Forcar: true, CacheDir: filepath.Join(t.TempDir(), "cache"), HTTPClient: srv.Client()})
	if err != nil || len(r.Instancias[0].Janelas) != 2 {
		t.Fatalf("resultado: %+v erro=%v", r, err)
	}
	got := map[string]Janela{}
	for _, j := range r.Instancias[0].Janelas {
		got[j.Nome] = j
	}
	for _, nome := range []string{"5h", "semana"} {
		j, ok := got[nome]
		if !ok || j.VoltaEm == "" {
			t.Fatalf("janela sem nome/voltaEm: %q %+v", nome, got)
		}
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"voltaEm"`) || strings.Contains(string(b), `"nome":"limite"`) {
		t.Fatalf("JSON de janelas: %s", b)
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

func TestAvaliarCotaEscolheOutraContaDaMesmaBaseComDadoFresco(t *testing.T) {
	agora := time.Now()
	fresco := agora.Add(-time.Minute).Format(time.RFC3339)
	velho := agora.Add(-time.Hour).Format(time.RFC3339)
	jan := func(p float64) []Janela { return []Janela{{Nome: "5h", Percentual: p}} }
	r := Resultado{Instancias: []Instancia{
		{Nome: "a", Base: "claude-code", ContaID: "1", DadoEm: fresco, Janelas: jan(92)},
		{Nome: "b", Base: "claude-code", ContaID: "2", DadoEm: fresco, Janelas: jan(30)},
		{Nome: "c", Base: "claude-code", ContaID: "3", DadoEm: fresco, Janelas: jan(10)},
		{Nome: "mesma", Base: "claude-code", ContaID: "1", DadoEm: fresco, Janelas: jan(1)},
		{Nome: "velha", Base: "claude-code", ContaID: "4", DadoEm: velho, Janelas: jan(1)},
		{Nome: "cheia", Base: "claude-code", ContaID: "5", DadoEm: fresco, Janelas: jan(85)},
		{Nome: "semid", Base: "claude-code", DadoEm: fresco, Janelas: jan(1)},
		{Nome: "outra-base", Base: "codex", ContaID: "6", DadoEm: fresco, Janelas: jan(1)},
	}}
	d := avaliarCota(r, agora, "a", 80, 5*time.Minute, nil)
	if !d.Conhecida || !d.Acima || d.Alternativa != "c" || d.AlternativaPercentual != 10 {
		t.Fatalf("%+v", d)
	}
	d = avaliarCota(r, agora, "a", 80, 5*time.Minute, func(n string) bool { return n != "c" })
	if d.Alternativa != "b" {
		t.Fatalf("validação ignorada: %+v", d)
	}
	if d := avaliarCota(r, agora, "b", 80, 5*time.Minute, nil); !d.Conhecida || d.Acima || d.Alternativa != "" {
		t.Fatalf("abaixo do limiar não troca: %+v", d)
	}
	if d := avaliarCota(r, agora, "velha", 80, 5*time.Minute, nil); d.Conhecida {
		t.Fatalf("dado velho não decide: %+v", d)
	}
	if d := avaliarCota(r, agora, "inexistente", 80, 5*time.Minute, nil); d.Conhecida {
		t.Fatalf("%+v", d)
	}
}
