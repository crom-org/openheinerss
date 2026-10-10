package versoes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtrairVersaoEComparar(t *testing.T) {
	casos := map[string]string{
		"2.1.295 (Claude Code)": "2.1.295",
		"codex-cli 0.159.2":     "0.159.2",
		"aider 0.86.2":          "0.86.2",
		"1.18.33\n":             "1.18.33",
		"rust-v0.160.0":         "0.160.0",
		"sem versão":            "",
	}
	for in, want := range casos {
		if got := ExtrairVersao(in); got != want {
			t.Errorf("ExtrairVersao(%q) = %q, quero %q", in, got, want)
		}
	}
	if Comparar("0.9.0", "0.10.0") != -1 || Comparar("1.2", "1.2.0") != 0 || Comparar("2.0.0", "1.99.9") != 1 {
		t.Fatal("Comparar numérico incorreto")
	}
}

func TestClassificarCaminho(t *testing.T) {
	casos := []struct{ real, shebang, metodo, pacote string }{
		{"/home/u/.nvm/versions/node/v22/lib/node_modules/@openai/codex/bin/codex.js", "", InstNPM, "@openai/codex"},
		{"/home/u/.nvm/lib/node_modules/opencode-ai/bin/opencode", "", InstNPM, "opencode-ai"},
		{"/home/u/.local/share/uv/tools/aider-chat/bin/aider", "", InstUV, "aider-chat"},
		{"/home/u/.local/pipx/venvs/aider-chat/bin/aider", "", InstPipx, "aider-chat"},
		{"/opt/homebrew/Cellar/codex/0.1.0/bin/codex", "", InstBrew, "codex"},
		{"/home/u/.local/bin/aider", "#!/usr/bin/python3", InstPip, ""},
		{"/home/u/.opencode/bin/opencode", "", InstScript, ""},
		{"/home/u/.local/share/claude/versions/2.1.295", "", InstScript, ""},
		{"/usr/local/bin/qualquer", "", InstBinario, ""},
	}
	for _, c := range casos {
		m, p, _ := ClassificarCaminho(c.real, c.shebang)
		if m != c.metodo || p != c.pacote {
			t.Errorf("%s: %s/%s, quero %s/%s", c.real, m, p, c.metodo, c.pacote)
		}
	}
}

// cli simula um CLI instalado: guarda a versão e a ajuda e registra as chamadas.
type cli struct {
	versao string
	ajuda  string
	quebra bool // --version falha
}

type mundo struct {
	t        *testing.T
	clis     map[string]*cli
	caminhos map[string]string
	chamadas []string
	// instalar roda ao receber um comando de instalação (nome → efeito).
	instalar func(args []string) error
	srv      *httptest.Server
	testar   func(string) (bool, string)
}

func novoMundo(t *testing.T) *mundo {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &mundo{t: t, clis: map[string]*cli{}, caminhos: map[string]string{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/npm/"):
			fmt.Fprint(w, `{"version":"2.0.0"}`)
		case strings.HasPrefix(r.URL.Path, "/pypi/"):
			fmt.Fprint(w, `{"info":{"version":"0.90.0"}}`)
		case strings.HasPrefix(r.URL.Path, "/gh/"):
			fmt.Fprint(w, `{"tag_name":"v2.0.0"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mundo) env() Ambiente {
	return Ambiente{
		LookPath: func(n string) (string, error) {
			if p, ok := m.caminhos[n]; ok {
				return p, nil
			}
			return "", errors.New("não está no PATH")
		},
		Executar: func(_ context.Context, nome string, args ...string) (string, error) {
			m.chamadas = append(m.chamadas, nome+" "+strings.Join(args, " "))
			c := m.clis[nome]
			if len(args) == 1 && args[0] == "--version" {
				if c == nil || c.quebra {
					return "", errors.New("quebrado")
				}
				return nome + " " + c.versao, nil
			}
			if len(args) == 1 && args[0] == "--help" && c != nil {
				return c.ajuda, nil
			}
			if m.instalar != nil {
				return "ok", m.instalar(append([]string{nome}, args...))
			}
			return "", errors.New("comando inesperado: " + nome)
		},
		HTTP:     m.srv.Client(),
		NPMURL:   m.srv.URL + "/npm",
		PyPIURL:  m.srv.URL + "/pypi",
		GitHub:   m.srv.URL + "/gh",
		ProcRoot: m.t.TempDir(),
		Agora:    time.Now,
		Dormir:   func(time.Duration) {},
		PID:      os.Getpid(),
		Testar: func(_ context.Context, inst string, _ time.Duration) (bool, string) {
			if m.testar != nil {
				return m.testar(inst)
			}
			return true, "OK"
		},
		InstanciaGratis: func(b string) string {
			if b == "opencode" {
				return "opencode-gratis"
			}
			return ""
		},
		BaseDe: func(n string) string {
			if n == "opencode-gratis" {
				return "opencode"
			}
			return ""
		},
	}
}

// instalarNPM põe um executável "npm global" falso em dir e devolve o caminho.
func instalarFalso(t *testing.T, sub string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), filepath.FromSlash(sub))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVersoesDetectaGerenciadorEUltima(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0", ajuda: "ajuda"}
	m.caminhos["aider"] = instalarFalso(t, "share/uv/tools/aider-chat/bin/aider")
	m.clis["aider"] = &cli{versao: "0.90.0", ajuda: "ajuda"}
	infos := m.env().Versoes(context.Background())
	por := map[string]Info{}
	for _, i := range infos {
		por[i.Harness] = i
	}
	if i := por["codex"]; i.Instalacao != InstNPM || i.Pacote != "@openai/codex" || i.Instalada != "1.0.0" || i.Disponivel != "2.0.0" || i.Fonte != "npm" || i.Estado != EstadoDesatualizada {
		t.Fatalf("codex: %+v", i)
	}
	if i := por["aider"]; i.Instalacao != InstUV || i.Disponivel != "0.90.0" || i.Fonte != "pypi" || i.Estado != EstadoAtualizada {
		t.Fatalf("aider: %+v", i)
	}
	if i := por["claude-code"]; i.Estado != EstadoAusente || i.Motivo == "" {
		t.Fatalf("claude ausente: %+v", i)
	}
}

func TestVersoesSemRedeViraDesconhecida(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0"}
	m.srv.Close()
	i := m.env().Info(context.Background(), "codex")
	if i.Disponivel != Desconhecida || i.Estado != EstadoDesconhecida || !strings.Contains(i.Motivo, "sem rede") || i.Instalada != "1.0.0" {
		t.Fatalf("sem rede: %+v", i)
	}
}

func TestAtualizarNPMTestaERegistra(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0", ajuda: "ajuda v1"}
	m.instalar = func(a []string) error {
		if strings.Join(a, " ") != "npm install -g @openai/codex@2.0.0" && a[0] == "npm" {
			return fmt.Errorf("comando inesperado %v", a)
		}
		m.clis["codex"].versao, m.clis["codex"].ajuda = "2.0.0", "ajuda v2"
		return nil
	}
	e := m.env()
	m.caminhos["npm"] = "/x/npm"
	// primeira conferência grava a ajuda; a atualização muda → ajudaMudou.
	r, err := e.Atualizar(context.Background(), "codex", Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Resultado != ResAtualizado || r.Antes != "1.0.0" || r.Depois != "2.0.0" || r.Teste == nil || r.Teste.Modo != "fumaca" || !r.Teste.OK {
		t.Fatalf("resultado: %+v", r)
	}
	if r.Comando != "npm install -g @openai/codex@2.0.0" || r.Volta != "automatica" {
		t.Fatalf("comando: %q volta %q", r.Comando, r.Volta)
	}
	h, _ := ArquivoHistorico()
	b, err := os.ReadFile(h)
	if err != nil || !strings.Contains(string(b), `"evento":"harness.atualizado"`) || !strings.Contains(string(b), `"depois":"2.0.0"`) {
		t.Fatalf("histórico: %v %s", err, b)
	}
}

func TestAtualizarTesteFalhaVoltaEConfirma(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["opencode"] = instalarFalso(t, ".opencode/bin/opencode")
	m.clis["opencode"] = &cli{versao: "1.0.0", ajuda: "a"}
	m.instalar = func(a []string) error {
		m.clis["opencode"].versao = strings.TrimPrefix(a[len(a)-1], "v")
		if a[len(a)-1] == "opencode" || a[len(a)-1] == "upgrade" {
			m.clis["opencode"].versao = "2.0.0"
		}
		return nil
	}
	m.testar = func(string) (bool, string) { return false, "falha: modelo caiu" }
	var log = filepath.Join(t.TempDir(), "eventos.log")
	r, err := m.env().Atualizar(context.Background(), "opencode", Opcoes{EventLog: log})
	if err != nil {
		t.Fatal(err)
	}
	if r.Resultado != ResVoltou || r.Depois != "1.0.0" || r.Teste == nil || r.Teste.Modo != "real" || r.Teste.Instancia != "opencode-gratis" {
		t.Fatalf("resultado: %+v", r)
	}
	if got := m.clis["opencode"].versao; got != "1.0.0" {
		t.Fatalf("versão final %s", got)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "ATUALIZADO opencode 1.0.0 → 1.0.0 resultado voltou") {
		t.Fatalf("log: %q", b)
	}
}

func TestAtualizarSimularFalhaTesteForcaVolta(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["opencode"] = instalarFalso(t, ".opencode/bin/opencode")
	m.clis["opencode"] = &cli{versao: "1.0.0", ajuda: "a"}
	m.instalar = func(a []string) error { m.clis["opencode"].versao = "2.0.0"; return nil }
	var volt bool
	e := m.env()
	e.Executar = func(c context.Context, n string, a ...string) (string, error) {
		if n == "opencode" && len(a) == 2 && a[1] == "1.0.0" {
			volt = true
			m.clis["opencode"].versao = "1.0.0"
			return "", nil
		}
		return m.env().Executar(c, n, a...)
	}
	r, _ := e.Atualizar(context.Background(), "opencode", Opcoes{SimularFalhaTeste: true})
	if r.Resultado != ResVoltou || !volt || !r.Teste.Simulado {
		t.Fatalf("resultado: %+v volt=%v", r, volt)
	}
}

func TestAtualizarSecoNaoAltera(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0"}
	m.instalar = func([]string) error { t.Fatal("seco não instala"); return nil }
	r, _ := m.env().Atualizar(context.Background(), "codex", Opcoes{Seco: true})
	if r.Resultado != ResSeco || m.clis["codex"].versao != "1.0.0" || len(r.Passos) == 0 {
		t.Fatalf("seco: %+v", r)
	}
	if h, _ := ArquivoHistorico(); existe(h) {
		t.Fatal("prévia não registra evento")
	}
}

func TestAtualizarJaNaUltima(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "2.0.0"}
	r, _ := m.env().Atualizar(context.Background(), "codex", Opcoes{})
	if r.Resultado != ResJaNaUltima {
		t.Fatalf("%+v", r)
	}
}

func TestAtualizarRecusaQuandoEmUsoEEsperaSolta(t *testing.T) {
	m := novoMundo(t)
	exe := instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.caminhos["codex"] = exe
	m.clis["codex"] = &cli{versao: "1.0.0", ajuda: "a"}
	m.instalar = func([]string) error { m.clis["codex"].versao = "2.0.0"; return nil }
	e := m.env()
	proc := func(pid int, exeLink string, argv ...string) {
		d := filepath.Join(e.ProcRoot, fmt.Sprint(pid))
		os.MkdirAll(d, 0o755)
		os.Symlink(exeLink, filepath.Join(d, "exe"))
		os.WriteFile(filepath.Join(d, "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0o644)
		os.WriteFile(filepath.Join(d, "stat"), []byte(fmt.Sprintf("%d (x y) S 100 1 1", pid)), 0o644)
	}
	os.MkdirAll(filepath.Join(e.ProcRoot, "100"), 0o755)
	os.WriteFile(filepath.Join(e.ProcRoot, "100", "cmdline"), []byte("openheinerss\x00rodar\x00tarefa\x00opencode-gratis\x00"), 0o644)
	os.WriteFile(filepath.Join(e.ProcRoot, "100", "stat"), []byte("100 (openheinerss) S 1 1 1"), 0o644)
	os.Symlink("/bin/sh", filepath.Join(e.ProcRoot, "100", "exe"))
	proc(200, "/usr/bin/node", "node", exe)
	r, _ := e.Atualizar(context.Background(), "codex", Opcoes{})
	if r.Resultado != ResOcupado || len(r.Ocupado) != 1 || r.Ocupado[0].PID != 200 || r.Ocupado[0].Tipo != "rodar" {
		t.Fatalf("ocupado: %+v", r)
	}
	if m.clis["codex"].versao != "1.0.0" {
		t.Fatal("recusa não pode instalar")
	}
	// rodar de instância da base também conta, mesmo sem processo filho.
	m2 := novoMundo(t)
	m2.caminhos["opencode"] = instalarFalso(t, ".opencode/bin/opencode")
	m2.clis["opencode"] = &cli{versao: "1.0.0"}
	e2 := m2.env()
	d := filepath.Join(e2.ProcRoot, "300")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "cmdline"), []byte("openheinerss\x00rodar\x00tarefa\x00opencode-gratis\x00"), 0o644)
	if uso := e2.EmUso("opencode", m2.caminhos["opencode"]); len(uso) != 1 || uso[0].Tipo != "rodar" {
		t.Fatalf("rodar da instância: %+v", uso)
	}
	// --esperar: some o processo durante a espera e a atualização segue.
	dormiu := 0
	e.Dormir = func(time.Duration) {
		dormiu++
		os.RemoveAll(filepath.Join(e.ProcRoot, "200"))
		os.RemoveAll(filepath.Join(e.ProcRoot, "100"))
	}
	r, _ = e.Atualizar(context.Background(), "codex", Opcoes{Esperar: true})
	if dormiu == 0 || r.Resultado != ResAtualizado {
		t.Fatalf("esperar: dormiu=%d %+v", dormiu, r)
	}
}

func TestAtualizarFalhaDeInstalacaoMantemAnterior(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "lib/node_modules/@openai/codex/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0"}
	m.instalar = func([]string) error { return errors.New("EACCES") }
	r, _ := m.env().Atualizar(context.Background(), "codex", Opcoes{})
	if r.Resultado != ResFalhou || r.Depois != "1.0.0" || !strings.Contains(r.Motivo, "EACCES") {
		t.Fatalf("%+v", r)
	}
}

func TestAtualizarBrewSemVoltaExigeFlag(t *testing.T) {
	m := novoMundo(t)
	m.caminhos["codex"] = instalarFalso(t, "Cellar/codex/1.0.0/bin/codex")
	m.clis["codex"] = &cli{versao: "1.0.0"}
	m.instalar = func([]string) error { t.Fatal("não deveria instalar"); return nil }
	r, _ := m.env().Atualizar(context.Background(), "codex", Opcoes{})
	if r.Resultado != ResFalhou || r.Volta != "manual" || !strings.Contains(r.Motivo, "--sem-volta") {
		t.Fatalf("%+v", r)
	}
}
