package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const repoGitHub = "crom-org/openheinerss"

// relatorioAtualizar é o resultado do comando (também a saída de --json para a Central).
type relatorioAtualizar struct {
	Modo         string       `json:"modo"` // fonte | release | voltar
	Seco         bool         `json:"seco"`
	Destino      string       `json:"destino"`
	Antes        string       `json:"antes"`
	Disponivel   string       `json:"disponivel,omitempty"`
	Depois       string       `json:"depois,omitempty"`
	Anterior     string       `json:"anterior,omitempty"`
	Passos       []string     `json:"passos"`
	Serves       []serveRel   `json:"serves"`
	RodarAntigos []processoRe `json:"rodarAntigos"`
	ComoVoltar   string       `json:"comoVoltar,omitempty"`
	Avisos       []string     `json:"avisos,omitempty"`
}

type serveRel struct {
	PID       int    `json:"pid"`
	NovoPID   int    `json:"novoPid,omitempty"`
	Porta     int    `json:"porta,omitempty"`
	Estado    string `json:"estado"` // reiniciar | reiniciado | mantido | falhou
	Detalhe   string `json:"detalhe,omitempty"`
	Comando   string `json:"comando"`
	Diretório string `json:"cwd,omitempty"`
}

type processoRe struct {
	PID     int    `json:"pid"`
	Comando string `json:"comando"`
}

// processo é a fotografia de um processo lida de /proc.
type processo struct {
	PID   int
	Argv  []string
	Env   []string
	Cwd   string
	Saida string // alvo de /proc/<pid>/fd/1 se for arquivo comum
	Erro  string // idem para fd/2
	Exe   string
}

type opcoesAtualizar struct {
	Seco, SemReiniciar, DeFonte, Release, Voltar, JSON bool
	Destino, Repo                                      string
}

// ambienteAtualizar concentra o que os testes precisam trocar.
type ambienteAtualizar struct {
	ProcRoot  string
	Sinalizar func(pid int) error
	Viva      func(pid int) bool
	Iniciar   func(p processo, destino string) (int, error)
	Sonda     func(addr string) bool
	Espera    time.Duration
	Compilar  func(repo, saida string) error
	HTTP      *http.Client
	APIURL    string
	BaseURL   string
	GOOS      string
	GOARCH    string
}

func ambientePadrao() ambienteAtualizar {
	return ambienteAtualizar{
		ProcRoot:  "/proc",
		Sinalizar: sinalizarTerm,
		Viva:      processoVivo,
		Iniciar:   iniciarDesacoplado,
		Sonda:     sondaTCP,
		Espera:    15 * time.Second,
		Compilar:  compilarFonte,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		APIURL:    envOu("OPENHEINERSS_API_URL", "https://api.github.com/repos/"+repoGitHub+"/releases/latest"),
		BaseURL:   os.Getenv("OPENHEINERSS_BASE_URL"),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}

func envOu(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func newAtualizarCmd() *cobra.Command {
	var o opcoesAtualizar
	cmd := &cobra.Command{
		Use:     "atualizar",
		Aliases: []string{"update", "upgrade"},
		Short:   "Atualiza o binário instalado e reinicia os 'serve' em execução (--voltar desfaz)",
		Long: "Instala uma nova versão do Openheinerss de forma atômica (guarda a anterior em <destino>.anterior) " +
			"e reinicia os 'serve' do usuário que usam esse binário, com os mesmos argumentos, ambiente e pasta. " +
			"Agentes 'rodar' em andamento não são tocados: o comando só lista quais continuam na versão antiga.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rel, err := executarAtualizar(o, ambientePadrao())
			if o.JSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if e := enc.Encode(rel); e != nil {
					return e
				}
			} else if rel != nil {
				imprimirAtualizar(cmd.OutOrStdout(), rel)
			}
			return err
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Seco, "seco", false, "Mostra o plano sem alterar nada")
	f.BoolVar(&o.SemReiniciar, "sem-reiniciar", false, "Instala, mas não reinicia os 'serve' em execução")
	f.BoolVar(&o.DeFonte, "de-fonte", false, "Compila do repositório local (padrão se a pasta atual for o repositório)")
	f.BoolVar(&o.Release, "release", false, "Baixa o binário da última release do GitHub e confere o checksum")
	f.BoolVar(&o.Voltar, "voltar", false, "Restaura o binário anterior (<destino>.anterior) e reinicia os 'serve'")
	f.BoolVar(&o.JSON, "json", false, "Saída em JSON (para a Central)")
	f.StringVar(&o.Destino, "destino", "", "Binário a atualizar (padrão ~/.local/bin/openheinerss)")
	f.StringVar(&o.Repo, "repo", "", "Repositório local para --de-fonte (padrão: pasta atual ou $OPENHEINERSS_REPO)")
	return cmd
}

func imprimirAtualizar(w io.Writer, r *relatorioAtualizar) {
	titulo := "Atualização"
	if r.Modo == "voltar" {
		titulo = "Volta ao binário anterior"
	}
	if r.Seco {
		titulo += " (seco: nada foi alterado)"
	}
	fmt.Fprintf(w, "%s — %s\n", titulo, r.Destino)
	fmt.Fprintf(w, "  instalada: %s\n", r.Antes)
	if r.Disponivel != "" {
		fmt.Fprintf(w, "  disponível: %s\n", r.Disponivel)
	}
	for _, p := range r.Passos {
		fmt.Fprintf(w, "  • %s\n", p)
	}
	for _, s := range r.Serves {
		extra := ""
		if s.Detalhe != "" {
			extra = " — " + s.Detalhe
		}
		novo := ""
		if s.NovoPID != 0 {
			novo = fmt.Sprintf(" → pid %d", s.NovoPID)
		}
		fmt.Fprintf(w, "  serve pid %d (porta %d): %s%s%s\n", s.PID, s.Porta, s.Estado, novo, extra)
	}
	for _, p := range r.RodarAntigos {
		fmt.Fprintf(w, "  rodar pid %d continua na versão antiga: %s\n", p.PID, p.Comando)
	}
	for _, a := range r.Avisos {
		fmt.Fprintf(w, "  aviso: %s\n", a)
	}
	if r.Depois != "" {
		fmt.Fprintf(w, "\n%s  →  %s\n", r.Antes, r.Depois)
	}
	if r.ComoVoltar != "" {
		fmt.Fprintf(w, "Para voltar: %s\n", r.ComoVoltar)
	}
}

func destinoPadrao() (string, error) {
	if d := os.Getenv("OPENHEINERSS_INSTALL_DIR"); d != "" {
		return filepath.Join(d, "openheinerss"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "openheinerss"), nil
}

func executarAtualizar(o opcoesAtualizar, env ambienteAtualizar) (*relatorioAtualizar, error) {
	if o.DeFonte && o.Release {
		return nil, errors.New("use --de-fonte ou --release, não os dois")
	}
	if o.Voltar && (o.DeFonte || o.Release) {
		return nil, errors.New("--voltar não combina com --de-fonte/--release")
	}
	destino := o.Destino
	if destino == "" {
		d, err := destinoPadrao()
		if err != nil {
			return nil, err
		}
		destino = d
	}
	destino, err := filepath.Abs(destino)
	if err != nil {
		return nil, err
	}
	rel := &relatorioAtualizar{Seco: o.Seco, Destino: destino, Passos: []string{}, Serves: []serveRel{}, RodarAntigos: []processoRe{}}
	rel.Antes = versaoDoBinario(destino)
	anterior := destino + ".anterior"

	// Fotografia dos processos ANTES de trocar o binário (depois o exe aponta para "(deleted)").
	procs := listarProcessos(env.ProcRoot, destino)
	var serves, rodares []processo
	for _, p := range procs {
		switch classificar(p) {
		case "serve":
			serves = append(serves, p)
		case "rodar":
			rodares = append(rodares, p)
		}
	}
	if env.ProcRoot == "" || !existe(env.ProcRoot) {
		rel.Avisos = append(rel.Avisos, "sem /proc neste sistema: não dá para listar nem reiniciar os 'serve'; reinicie-os manualmente")
	}

	var novo string // arquivo temporário já pronto para instalar
	if o.Voltar {
		rel.Modo = "voltar"
		if _, err := os.Stat(anterior); err != nil {
			return rel, fmt.Errorf("não há %s para restaurar", anterior)
		}
		rel.Anterior = versaoDoBinario(anterior)
		rel.Depois = rel.Anterior
		rel.Passos = append(rel.Passos, fmt.Sprintf("restaurar %s (%s) em %s", anterior, rel.Anterior, destino))
		rel.ComoVoltar = "openheinerss atualizar --voltar  (volta de novo ao binário que está instalado agora)"
	} else {
		fonte := o.DeFonte
		repo := ""
		if !o.Release && !o.DeFonte {
			repo = acharRepo(o.Repo)
			fonte = repo != ""
		} else if fonte {
			repo = acharRepo(o.Repo)
			if repo == "" {
				return rel, errors.New("--de-fonte: repositório não encontrado (use --repo ou rode na raiz do clone)")
			}
		}
		if fonte {
			rel.Modo = "fonte"
			rel.Disponivel = versaoDaFonte(repo)
			rel.Passos = append(rel.Passos, "compilar "+repo+" (go build com ldflags de versão)")
		} else {
			rel.Modo = "release"
			tag, err := ultimaTag(env)
			if err != nil {
				return rel, fmt.Errorf("descobrir a última release: %w", err)
			}
			rel.Disponivel = tag
			rel.Passos = append(rel.Passos, fmt.Sprintf("baixar a release %s (%s/%s) e conferir o checksum", tag, env.GOOS, env.GOARCH))
		}
		rel.Passos = append(rel.Passos, "instalar de forma atômica; o atual vira "+anterior)
		rel.ComoVoltar = "openheinerss atualizar --voltar"
		if o.Destino != "" {
			rel.ComoVoltar += " --destino " + destino
		}
		if !o.Seco {
			if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
				return rel, err
			}
			tmp, err := tempNaPasta(destino)
			if err != nil {
				return rel, err
			}
			novo = tmp
			defer os.Remove(novo)
			if fonte {
				err = env.Compilar(repo, novo)
			} else {
				err = baixarRelease(env, rel.Disponivel, novo)
			}
			if err != nil {
				return rel, err
			}
			if err := os.Chmod(novo, 0o755); err != nil {
				return rel, err
			}
			rel.Depois = versaoDoBinario(novo)
			if rel.Depois == "" {
				return rel, errors.New("o binário novo não executa 'version'; instalação cancelada (nada foi alterado)")
			}
		}
	}

	if !o.Seco {
		if o.Voltar {
			if err := trocarComAnterior(destino, anterior); err != nil {
				return rel, err
			}
		} else {
			if err := instalarAtomico(destino, novo, anterior); err != nil {
				return rel, err
			}
			rel.Depois = versaoDoBinario(destino)
		}
	}

	// Agentes rodar: nunca tocados.
	for _, p := range rodares {
		rel.RodarAntigos = append(rel.RodarAntigos, processoRe{PID: p.PID, Comando: linhaComando(p)})
	}

	// serve: reinicia (ou só planeja).
	for _, p := range serves {
		s := serveRel{PID: p.PID, Porta: portaDoServe(p), Comando: linhaComando(p), Diretório: p.Cwd}
		switch {
		case servidorStdio(p):
			s.Estado, s.Detalhe = "mantido", "serve --stdio depende do processo pai; reinicie-o pelo cliente"
		case o.SemReiniciar:
			s.Estado, s.Detalhe = "mantido", "--sem-reiniciar: continua na versão antiga"
		case o.Seco:
			s.Estado = "reiniciar"
		default:
			s = reiniciarServe(env, p, s, destino)
		}
		rel.Serves = append(rel.Serves, s)
	}
	sort.Slice(rel.Serves, func(i, j int) bool { return rel.Serves[i].PID < rel.Serves[j].PID })
	for _, s := range rel.Serves {
		if s.Estado == "falhou" {
			return rel, fmt.Errorf("serve pid %d não voltou: %s", s.PID, s.Detalhe)
		}
	}
	return rel, nil
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }

func tempNaPasta(destino string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(destino), ".openheinerss-novo-*")
	if err != nil {
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// copiarAtomico grava uma cópia de origem em alvo (temporário na mesma pasta + rename).
func copiarAtomico(origem, alvo string) error {
	src, err := os.Open(origem)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := tempNaPasta(alvo)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if _, err = io.Copy(f, src); err == nil {
		err = f.Close()
	} else {
		f.Close()
	}
	if err == nil {
		err = os.Chmod(tmp, 0o755)
	}
	if err == nil {
		err = os.Rename(tmp, alvo)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// instalarAtomico guarda o destino atual em anterior e coloca novo (arquivo na mesma pasta) no destino.
func instalarAtomico(destino, novo, anterior string) error {
	if existe(destino) {
		if err := copiarAtomico(destino, anterior); err != nil {
			return fmt.Errorf("guardar o binário anterior: %w", err)
		}
	}
	if err := os.Rename(novo, destino); err != nil {
		return fmt.Errorf("instalar: %w", err)
	}
	return nil
}

// trocarComAnterior troca destino e anterior (o que estava instalado vira o novo .anterior).
func trocarComAnterior(destino, anterior string) error {
	var guardado string
	if existe(destino) {
		tmp, err := tempNaPasta(destino)
		if err != nil {
			return err
		}
		os.Remove(tmp)
		if err := copiarAtomico(destino, tmp); err != nil {
			return err
		}
		guardado = tmp
		defer os.Remove(guardado)
	}
	if err := os.Rename(anterior, destino); err != nil {
		return fmt.Errorf("restaurar: %w", err)
	}
	if guardado != "" {
		return os.Rename(guardado, anterior)
	}
	return nil
}

func versaoDoBinario(path string) string {
	if !existe(path) {
		return "(não instalado)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "openheinerss "))
}

func acharRepo(flag string) string {
	cands := []string{flag, os.Getenv("OPENHEINERSS_REPO")}
	if cwd, err := os.Getwd(); err == nil {
		cands = append(cands, cwd)
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(c, "go.mod")); err == nil &&
			bytes.Contains(b, []byte("module github.com/crom-org/openheinerss")) && existe(filepath.Join(c, "cmd", "openheinerss")) {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func gitSaida(repo string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func versaoDaFonte(repo string) string {
	v := gitSaida(repo, "describe", "--tags", "--always", "--dirty")
	if v == "" {
		v = "dev"
	}
	c := gitSaida(repo, "rev-parse", "--short", "HEAD")
	return fmt.Sprintf("%s (main local, commit %s)", v, c)
}

func compilarFonte(repo, saida string) error {
	versao := gitSaida(repo, "describe", "--tags", "--always", "--dirty")
	if versao == "" {
		versao = "dev"
	}
	commit := gitSaida(repo, "rev-parse", "HEAD")
	if commit == "" {
		commit = "desconhecido"
	}
	ld := fmt.Sprintf("-X main.Version=%s -X main.Commit=%s -X main.Date=%s", versao, commit, time.Now().UTC().Format(time.RFC3339))
	cmd := exec.Command("go", "build", "-ldflags", ld, "-o", saida, "./cmd/openheinerss")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ultimaTag(env ambienteAtualizar) (string, error) {
	resp, err := env.HTTP.Get(env.APIURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var v struct {
				Tag string `json:"tag_name"`
			}
			if json.NewDecoder(resp.Body).Decode(&v) == nil && v.Tag != "" {
				return v.Tag, nil
			}
		} else {
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}
	if gh, e := exec.LookPath("gh"); e == nil {
		out, e := exec.Command(gh, "api", "repos/"+repoGitHub+"/releases/latest", "--jq", ".tag_name").Output()
		if t := strings.TrimSpace(string(out)); e == nil && t != "" {
			return t, nil
		}
	}
	if err == nil {
		err = errors.New("resposta sem tag_name")
	}
	return "", err
}

func baixar(env ambienteAtualizar, url string) ([]byte, error) {
	resp, err := env.HTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

// baixarRelease baixa o pacote da tag, confere o sha256 em checksums.txt e extrai o binário em saida.
func baixarRelease(env ambienteAtualizar, tag, saida string) error {
	versao := strings.TrimPrefix(tag, "v")
	ext := ".tar.gz"
	if env.GOOS == "windows" {
		ext = ".zip"
	}
	pacote := fmt.Sprintf("openheinerss_%s_%s_%s%s", versao, env.GOOS, env.GOARCH, ext)
	base := strings.TrimRight(env.BaseURL, "/")
	if base == "" {
		base = "https://github.com/" + repoGitHub + "/releases/download/v" + versao
	}
	dados, err := baixar(env, base+"/"+pacote)
	if err != nil {
		return err
	}
	sums, err := baixar(env, base+"/checksums.txt")
	if err != nil {
		return err
	}
	esperado := ""
	for _, l := range strings.Split(string(sums), "\n") {
		c := strings.Fields(l)
		if len(c) == 2 && strings.TrimPrefix(c[1], "*") == pacote {
			esperado = c[0]
		}
	}
	if esperado == "" {
		return fmt.Errorf("checksum ausente para %s", pacote)
	}
	h := sha256.Sum256(dados)
	if !strings.EqualFold(hex.EncodeToString(h[:]), esperado) {
		return fmt.Errorf("checksum inválido para %s", pacote)
	}
	bin, err := extrairBinario(dados, ext)
	if err != nil {
		return err
	}
	return os.WriteFile(saida, bin, 0o755)
}

func extrairBinario(dados []byte, ext string) ([]byte, error) {
	nomeOK := func(n string) bool {
		b := filepath.Base(n)
		return b == "openheinerss" || b == "openheinerss.exe"
	}
	if ext == ".zip" {
		zr, err := zip.NewReader(bytes.NewReader(dados), int64(len(dados)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if nomeOK(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errors.New("binário não encontrado no pacote")
	}
	gz, err := gzip.NewReader(bytes.NewReader(dados))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("binário não encontrado no pacote")
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && nomeOK(h.Name) {
			return io.ReadAll(tr)
		}
	}
}

// ---- processos ----

func listarProcessos(root, destino string) []processo {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []processo
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !donoSouEu(dir) {
			continue
		}
		exe, err := os.Readlink(filepath.Join(dir, "exe"))
		if err != nil {
			continue
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		if exe != destino {
			continue
		}
		cl, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil || len(cl) == 0 {
			continue
		}
		p := processo{PID: pid, Exe: exe, Argv: partirNul(cl)}
		if ev, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
			p.Env = partirNul(ev)
		}
		p.Cwd, _ = os.Readlink(filepath.Join(dir, "cwd"))
		p.Saida = alvoArquivo(filepath.Join(dir, "fd", "1"))
		p.Erro = alvoArquivo(filepath.Join(dir, "fd", "2"))
		out = append(out, p)
	}
	return out
}

func alvoArquivo(link string) string {
	t, err := os.Readlink(link)
	if err != nil || !filepath.IsAbs(t) || strings.HasSuffix(t, " (deleted)") {
		return ""
	}
	if st, err := os.Stat(t); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return t
}

func partirNul(b []byte) []string {
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}

// subcomando acha o primeiro argumento que não é flag nem valor de flag global.
func subcomando(argv []string) string {
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if a == "--projeto" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

func classificar(p processo) string {
	switch subcomando(p.Argv) {
	case "serve", "servir":
		return "serve"
	case "rodar":
		return "rodar"
	}
	return ""
}

func servidorStdio(p processo) bool {
	for _, a := range p.Argv[1:] {
		if a == "--stdio" {
			return true
		}
	}
	return false
}

func portaDoServe(p processo) int {
	porta := 0
	for _, n := range []string{"OPENHEINERSS_PORT", "OPENHEINERSS_PORTA"} {
		for _, e := range p.Env {
			if v, ok := strings.CutPrefix(e, n+"="); ok {
				if x, err := strconv.Atoi(v); err == nil && x > 0 && x <= 65535 {
					porta = x
				}
			}
		}
	}
	if porta == 0 {
		porta = 4820
	}
	args := p.Argv[1:]
	for i, a := range args {
		for _, pre := range []string{"--porta", "--port", "-p"} {
			var v string
			if a == pre && i+1 < len(args) {
				v = args[i+1]
			} else if s, ok := strings.CutPrefix(a, pre+"="); ok {
				v = s
			}
			if x, err := strconv.Atoi(v); err == nil && x > 0 {
				porta = x
			}
		}
	}
	return porta
}

func hostDoServe(p processo) string {
	args := p.Argv[1:]
	for i, a := range args {
		for _, pre := range []string{"--host", "--hospedeiro"} {
			if a == pre && i+1 < len(args) {
				return args[i+1]
			}
			if s, ok := strings.CutPrefix(a, pre+"="); ok {
				return s
			}
		}
	}
	return "127.0.0.1"
}

func linhaComando(p processo) string {
	a := append([]string{filepath.Base(p.Argv[0])}, p.Argv[1:]...)
	return strings.Join(a, " ")
}

func processoVivo(pid int) bool {
	pr, err := os.FindProcess(pid)
	if err != nil || pr.Signal(syscall.Signal(0)) != nil {
		return false
	}
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if i := bytes.LastIndexByte(b, ')'); i >= 0 && len(b) > i+2 && b[i+2] == 'Z' {
			return false // zumbi: já terminou
		}
	}
	return true
}

func sondaTCP(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func reiniciarServe(env ambienteAtualizar, p processo, s serveRel, destino string) serveRel {
	falha := func(msg string) serveRel { s.Estado, s.Detalhe = "falhou", msg; return s }
	if err := env.Sinalizar(p.PID); err != nil {
		return falha("SIGTERM: " + err.Error())
	}
	fim := time.Now().Add(env.Espera)
	for env.Viva(p.PID) {
		if time.Now().After(fim) {
			return falha("não encerrou em " + env.Espera.String() + " após SIGTERM (não foi forçado)")
		}
		time.Sleep(50 * time.Millisecond)
	}
	pid, err := env.Iniciar(p, destino)
	if err != nil {
		return falha("relançar: " + err.Error())
	}
	s.NovoPID = pid
	addr := net.JoinHostPort(hostDoServe(p), strconv.Itoa(s.Porta))
	fim = time.Now().Add(env.Espera)
	for !env.Sonda(addr) {
		if time.Now().After(fim) {
			return falha("a porta " + addr + " não respondeu após o relançamento")
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.Estado, s.Detalhe = "reiniciado", addr+" respondeu"
	return s
}

func iniciarDesacoplado(p processo, destino string) (int, error) {
	cmd := exec.Command(destino, p.Argv[1:]...)
	cmd.Env = p.Env
	cmd.Dir = p.Cwd
	abrir := func(path string) *os.File {
		if path != "" {
			if f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0); err == nil {
				return f
			}
		}
		f, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		return f
	}
	out, errf := abrir(p.Saida), abrir(p.Erro)
	defer out.Close()
	defer errf.Close()
	if in, err := os.Open(os.DevNull); err == nil {
		defer in.Close()
		cmd.Stdin = in
	}
	cmd.Stdout, cmd.Stderr = out, errf
	cmd.SysProcAttr = atributosDesacoplado()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}
