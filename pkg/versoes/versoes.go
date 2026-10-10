// Package versoes descobre a versão instalada de cada harness base, como ela foi instalada e
// qual é a última publicada pela fonte oficial; e atualiza (com teste e volta automática).
package versoes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Métodos de instalação reconhecidos pelo caminho real do executável.
const (
	InstNPM     = "npm"
	InstUV      = "uv"
	InstPipx    = "pipx"
	InstPip     = "pip"
	InstBrew    = "brew"
	InstScript  = "script" // instalador oficial/binário próprio: o CLI se atualiza sozinho
	InstBinario = "binario"
	InstNenhum  = "nenhum"
)

// Estados de Info.Estado.
const (
	EstadoAtualizada    = "atualizada"
	EstadoDesatualizada = "desatualizada"
	EstadoDesconhecida  = "desconhecida"
	EstadoAusente       = "ausente"
)

// Desconhecida é o valor de Info.Disponivel quando a fonte oficial não respondeu.
const Desconhecida = "desconhecida"

// Info é uma linha de `harness versoes` e do RPC harness.versoes.
type Info struct {
	Harness    string `json:"harness"`
	CLI        string `json:"cli"`
	Caminho    string `json:"caminho,omitempty"`
	Instalada  string `json:"instalada,omitempty"`
	Instalacao string `json:"instalacao"`
	Pacote     string `json:"pacote,omitempty"`
	Disponivel string `json:"disponivel"`
	Fonte      string `json:"fonte,omitempty"`
	Estado     string `json:"estado"`
	// Motivo explica "desconhecida"/"ausente" (sem rede, sem fonte oficial, CLI fora do PATH...).
	Motivo string `json:"motivo,omitempty"`
	// Comando é o que `harness atualizar` executaria; Volta é "automatica" ou "manual".
	Comando string `json:"comando,omitempty"`
	Volta   string `json:"volta,omitempty"`
}

type baseSpec struct {
	cli, npm, pypi, github string
	// atualiza/alvo: subcomando de autoatualização do CLI e se ele aceita a versão como argumento.
	atualiza   string
	aceitaAlvo bool
}

var specs = map[string]baseSpec{
	"claude-code": {cli: "claude", npm: "@anthropic-ai/claude-code", atualiza: "update"},
	"codex":       {cli: "codex", npm: "@openai/codex", github: "openai/codex", atualiza: "update"},
	"opencode":    {cli: "opencode", npm: "opencode-ai", github: "sst/opencode", atualiza: "upgrade", aceitaAlvo: true},
	"aider":       {cli: "aider", pypi: "aider-chat"},
	"agy":         {cli: "agy", atualiza: "update"},
}

// Bases lista as bases tratadas, na mesma ordem de `capacidades`.
func Bases() []string { return []string{"claude-code", "codex", "opencode", "aider", "agy"} }

// Ambiente reúne o que os testes precisam trocar.
type Ambiente struct {
	LookPath func(string) (string, error)
	// Executar roda o comando e devolve stdout+stderr juntos.
	Executar func(ctx context.Context, nome string, args ...string) (string, error)
	HTTP     *http.Client
	NPMURL   string // padrão https://registry.npmjs.org
	PyPIURL  string // padrão https://pypi.org/pypi
	GitHub   string // padrão https://api.github.com/repos
	ProcRoot string
	Agora    func() time.Time
	Dormir   func(time.Duration)
	// Testar roda o `harness test` real da instância; ok=false é falha do teste.
	Testar func(ctx context.Context, instancia string, timeout time.Duration) (ok bool, detalhe string)
	// InstanciaGratis devolve a instância grátis da base ("" se não existir).
	InstanciaGratis func(base string) string
	// BaseDe devolve a base embutida do nome (instância ou harness); "" se não conhecer.
	BaseDe func(nome string) string
	// PID do processo atual (excluído da busca de uso).
	PID int
}

func envOu(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Padrao é o ambiente real. Testar/InstanciaGratis/BaseDe vêm de padrao_harness.go.
func Padrao() Ambiente {
	e := Ambiente{
		LookPath: exec.LookPath,
		Executar: func(ctx context.Context, nome string, args ...string) (string, error) {
			out, err := exec.CommandContext(ctx, nome, args...).CombinedOutput()
			return string(out), err
		},
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		NPMURL:   envOu("OPENHEINERSS_NPM_URL", "https://registry.npmjs.org"),
		PyPIURL:  envOu("OPENHEINERSS_PYPI_URL", "https://pypi.org/pypi"),
		GitHub:   envOu("OPENHEINERSS_GITHUB_URL", "https://api.github.com/repos"),
		ProcRoot: "/proc",
		Agora:    time.Now,
		Dormir:   time.Sleep,
		PID:      os.Getpid(),
	}
	ligarHarness(&e)
	return e
}

var reVersao = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)?(?:[-+.][0-9A-Za-z]+)*)`)

// ExtrairVersao tira o primeiro número de versão de uma saída de `--version`.
func ExtrairVersao(s string) string {
	m := reVersao.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return strings.TrimRight(m[1], ".-")
}

// Comparar devolve -1, 0 ou 1 comparando versões numéricas (a <, = ou > b).
func Comparar(a, b string) int {
	pa, pb := partesNumericas(a), partesNumericas(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func partesNumericas(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out []int
	for _, p := range strings.Split(v, ".") {
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}

// detectado é o resultado de olhar o caminho real do executável.
type detectado struct {
	caminho, metodo, pacote string
	// pyvenv é o python do ambiente (uv/pipx/pip) quando conhecido.
	python string
}

// ClassificarCaminho descobre como o executável foi instalado a partir do caminho real
// (já com symlinks resolvidos) e, quando o caminho não basta, da primeira linha do script.
func ClassificarCaminho(real string, shebang string) (metodo, pacote, python string) {
	p := filepath.ToSlash(real)
	segmento := func(depois string) string {
		i := strings.Index(p, depois)
		if i < 0 {
			return ""
		}
		resto := p[i+len(depois):]
		if strings.HasPrefix(resto, "@") { // pacote com escopo
			partes := strings.SplitN(resto, "/", 3)
			if len(partes) >= 2 {
				return partes[0] + "/" + partes[1]
			}
		}
		return strings.SplitN(resto, "/", 2)[0]
	}
	switch {
	case strings.Contains(p, "/node_modules/"):
		return InstNPM, segmento("/node_modules/"), ""
	case strings.Contains(p, "/uv/tools/"):
		nome := segmento("/uv/tools/")
		return InstUV, nome, filepath.Join(p[:strings.Index(p, "/uv/tools/")], "uv", "tools", nome, "bin", "python")
	case strings.Contains(p, "/pipx/venvs/"):
		nome := segmento("/pipx/venvs/")
		return InstPipx, nome, filepath.Join(p[:strings.Index(p, "/pipx/venvs/")], "pipx", "venvs", nome, "bin", "python")
	case strings.Contains(p, "/Cellar/"):
		return InstBrew, segmento("/Cellar/"), ""
	case strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/"):
		return InstBrew, filepath.Base(p), ""
	}
	if strings.HasPrefix(shebang, "#!") && strings.Contains(shebang, "python") {
		f := strings.Fields(strings.TrimPrefix(shebang, "#!"))
		py := ""
		if len(f) > 0 {
			py = f[len(f)-1]
			if filepath.Base(f[0]) == "env" && len(f) > 1 {
				py = f[1]
			}
		}
		return InstPip, "", py
	}
	for _, marca := range []string{"/.opencode/bin/", "/.codex/", "/.local/share/claude/", "/.claude/", "/.antigravity", "/.gemini/"} {
		if strings.Contains(p, marca) {
			return InstScript, "", ""
		}
	}
	return InstBinario, "", ""
}

func primeiraLinha(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	s := string(buf[:n])
	if !strings.HasPrefix(s, "#!") {
		return ""
	}
	return strings.SplitN(s, "\n", 2)[0]
}

func (e Ambiente) detectar(base string) (detectado, error) {
	sp, ok := specs[base]
	if !ok {
		return detectado{}, fmt.Errorf("harness %q não é uma base embutida (use: %s)", base, strings.Join(Bases(), ", "))
	}
	p, err := e.LookPath(sp.cli)
	if err != nil {
		return detectado{}, err
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		real = p
	}
	m, pacote, py := ClassificarCaminho(real, primeiraLinha(real))
	if (m == InstUV || m == InstPipx) && py != "" {
		// O python do venv some quando o ambiente é recriado: usa o interpretador base.
		if base, err := filepath.EvalSymlinks(py); err == nil {
			py = base
		}
	}
	d := detectado{caminho: real, metodo: m, pacote: pacote, python: py}
	if d.pacote == "" || m == InstPip {
		switch m {
		case InstPip, InstUV, InstPipx:
			d.pacote = sp.pypi
		case InstNPM:
			d.pacote = sp.npm
		}
	}
	return d, nil
}

// versaoCLI roda `<cli> --version`.
func (e Ambiente) versaoCLI(ctx context.Context, cli string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := e.Executar(ctx, cli, "--version")
	if err != nil {
		return "", fmt.Errorf("%s --version: %v", cli, err)
	}
	v := ExtrairVersao(out)
	if v == "" {
		return "", fmt.Errorf("%s --version não trouxe uma versão (saída: %q)", cli, strings.TrimSpace(out))
	}
	return v, nil
}

func (e Ambiente) getJSON(ctx context.Context, url string, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (e Ambiente) ultimaNPM(ctx context.Context, pkg string) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	err := e.getJSON(ctx, strings.TrimRight(e.NPMURL, "/")+"/"+pkg+"/latest", &v)
	if err == nil && v.Version != "" {
		return v.Version, nil
	}
	if err == nil {
		err = errors.New("resposta sem version")
	}
	// Sem HTTP direto (proxy, registry privado): tenta o npm do usuário.
	if _, e2 := e.LookPath("npm"); e2 == nil {
		out, e3 := e.Executar(ctx, "npm", "view", pkg, "version")
		if v := ExtrairVersao(out); e3 == nil && v != "" {
			return v, nil
		}
	}
	return "", err
}

func (e Ambiente) ultimaPyPI(ctx context.Context, pkg string) (string, error) {
	var v struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := e.getJSON(ctx, strings.TrimRight(e.PyPIURL, "/")+"/"+pkg+"/json", &v); err != nil {
		return "", err
	}
	if v.Info.Version == "" {
		return "", errors.New("resposta sem info.version")
	}
	return v.Info.Version, nil
}

func (e Ambiente) ultimaGitHub(ctx context.Context, repo string) (string, error) {
	var v struct {
		Tag string `json:"tag_name"`
	}
	if err := e.getJSON(ctx, strings.TrimRight(e.GitHub, "/")+"/"+repo+"/releases/latest", &v); err != nil {
		return "", err
	}
	if t := ExtrairVersao(v.Tag); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("tag %q sem versão", v.Tag)
}

// Ultima consulta as fontes oficiais da base na ordem adequada ao método de instalação.
func (e Ambiente) Ultima(ctx context.Context, base, metodo, pacote string) (versao, fonte string, err error) {
	sp := specs[base]
	type tent struct {
		nome string
		f    func() (string, error)
	}
	pkgNPM := sp.npm
	if metodo == InstNPM && pacote != "" {
		pkgNPM = pacote
	}
	npm := func() (string, error) { return e.ultimaNPM(ctx, pkgNPM) }
	pypi := func() (string, error) { return e.ultimaPyPI(ctx, sp.pypi) }
	gh := func() (string, error) { return e.ultimaGitHub(ctx, sp.github) }
	var ordem []tent
	add := func(ok bool, n string, f func() (string, error)) {
		if ok {
			ordem = append(ordem, tent{n, f})
		}
	}
	switch metodo {
	case InstNPM:
		add(sp.npm != "", "npm", npm)
		add(sp.github != "", "github", gh)
	case InstUV, InstPipx, InstPip:
		add(sp.pypi != "", "pypi", pypi)
	default: // script, brew, binário: releases do GitHub e, depois, o npm do mesmo produto
		add(sp.github != "", "github", gh)
		add(sp.npm != "", "npm", npm)
		add(sp.pypi != "", "pypi", pypi)
	}
	if len(ordem) == 0 {
		return "", "", fmt.Errorf("sem fonte oficial pública conhecida para %s (o CLI se atualiza sozinho: %s)", base, sp.cli+" "+sp.atualiza)
	}
	var erros []string
	for _, t := range ordem {
		v, err := t.f()
		if err == nil {
			return v, t.nome, nil
		}
		erros = append(erros, t.nome+": "+err.Error())
	}
	return "", "", fmt.Errorf("sem rede ou fonte fora do ar (%s)", strings.Join(erros, "; "))
}

// Versoes devolve uma linha por base (ou só a pedida).
func (e Ambiente) Versoes(ctx context.Context, bases ...string) []Info {
	if len(bases) == 0 {
		bases = Bases()
	}
	out := make([]Info, 0, len(bases))
	for _, b := range bases {
		out = append(out, e.Info(ctx, b))
	}
	return out
}

// Info descreve uma base.
func (e Ambiente) Info(ctx context.Context, base string) Info {
	sp, ok := specs[base]
	i := Info{Harness: base, Instalacao: InstNenhum, Disponivel: Desconhecida}
	if !ok {
		i.Estado, i.Motivo = EstadoDesconhecida, "harness não é uma base embutida"
		return i
	}
	i.CLI = sp.cli
	d, err := e.detectar(base)
	if err != nil {
		i.Estado, i.Motivo = EstadoAusente, sp.cli+" não está no PATH"
		return i
	}
	i.Caminho, i.Instalacao, i.Pacote = d.caminho, d.metodo, d.pacote
	v, verr := e.versaoCLI(ctx, sp.cli)
	i.Instalada = v
	ult, fonte, uerr := e.Ultima(ctx, base, d.metodo, d.pacote)
	if uerr == nil {
		i.Disponivel, i.Fonte = ult, fonte
	}
	switch {
	case verr != nil:
		i.Estado, i.Motivo = EstadoDesconhecida, verr.Error()
	case uerr != nil:
		i.Estado, i.Motivo = EstadoDesconhecida, "última versão desconhecida: "+uerr.Error()
	case Comparar(v, ult) < 0:
		i.Estado = EstadoDesatualizada
	default:
		i.Estado = EstadoAtualizada
	}
	if cmd, _, volta, err := e.comandos(d, sp, "", ult, ""); err == nil {
		i.Comando, i.Volta = strings.Join(cmd, " "), volta
	} else {
		i.Volta = "manual"
		i.Comando = ""
	}
	return i
}
