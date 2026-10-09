// Package contas gerencia instâncias de login sem tocar no conteúdo das credenciais.
package contas

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"gopkg.in/yaml.v3"
)

type Conta struct {
	Instancia     string `json:"instancia"`
	Base          string `json:"base"`
	ContaID       string `json:"contaId"`
	ContaIDFonte  string `json:"contaIdFonte,omitempty"`
	ContaDir      string `json:"contaDir"`
	TemLogin      bool   `json:"temLogin"`
	Arquivo       string `json:"arquivo,omitempty"`
	Origem        string `json:"origem"`
	MesmaContaQue string `json:"mesmaContaQue,omitempty"`
}

type Resultado struct {
	Conta
	ComandoLogin []string `json:"comandoLogin,omitempty"`
}

var nomeValido = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func NormalizarNome(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), "-")
	if s == "" || !nomeValido.MatchString(s) {
		return "", fmt.Errorf("nome inválido %q: use letras minúsculas, números, ponto, hífen ou sublinhado", s)
	}
	return s, nil
}

func Base(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case "claude", "claude-code":
		return "claude-code"
	case "codex":
		return "codex"
	case "opencode":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(h))
	}
}

func LoginEnv(base string) string {
	switch Base(base) {
	case "claude-code":
		return "CLAUDE_CONFIG_DIR"
	case "codex":
		return "CODEX_HOME"
	case "opencode":
		return "OPENCODE_CONFIG_DIR"
	default:
		return "LOGIN_DIR"
	}
}

func LoginCommand(base string) []string {
	switch Base(base) {
	case "claude-code":
		return []string{"claude", "auth", "login"}
	case "codex":
		return []string{"codex", "login"}
	case "opencode":
		return []string{"opencode", "auth", "login"}
	default:
		return []string{Base(base), "login"}
	}
}

func ContaDir(base, nome string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	prefix := Base(base)
	if prefix == "claude-code" {
		prefix = "claude"
	}
	if prefix == "opencode" {
		prefix = "opencode"
	}
	return filepath.Join(home, "."+prefix+"-"+nome), nil
}

func arquivoInstancia(dir, instancia string) string { return filepath.Join(dir, instancia+".yaml") }

func hashID(base, dir string) (string, string) {
	return identidade.IDPara(base, dir)
}

func expand(s string) string {
	if strings.HasPrefix(s, "~/") {
		if h, e := os.UserHomeDir(); e == nil {
			s = filepath.Join(h, s[2:])
		}
	}
	a, _ := filepath.Abs(filepath.Clean(s))
	return a
}

func temLogin(base, dir string) bool {
	// Apenas Stat: nunca abrimos nem interpretamos os arquivos de credencial.
	candidatos := []string{}
	switch Base(base) {
	case "claude-code":
		candidatos = []string{".credentials.json", "credentials.json", ".claude.json"}
	case "codex":
		candidatos = []string{"auth.json"}
	case "opencode":
		candidatos = []string{"auth.json", "credentials.json"}
	default:
		candidatos = []string{"credentials", "auth.json", ".credentials.json"}
	}
	for _, n := range candidatos {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

func ler(dir, path string) (Conta, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Conta{}, false, err
	}
	var s harness.CustomSpec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return Conta{}, false, err
	}
	base := s.Base
	if base == "" {
		base = s.Name
	}
	key := LoginEnv(base)
	if s.Name == "" {
		return Conta{}, false, nil
	}
	login := strings.TrimSpace(s.Env[key])
	if login == "" {
		// A listagem não pode transformar env ausente em cwd. Reutilizamos a
		// mesma resolução da identidade, inclusive o padrão do harness.
		id, err := identidade.Para(s.Name, s.Env)
		if err != nil {
			return Conta{}, false, err
		}
		login = id.ContaDir
	} else {
		login = expand(login)
	}
	nome, err := NormalizarNome(s.Name)
	if err != nil || nome != s.Name {
		return Conta{}, false, nil
	}
	id, fonte := hashID(Base(base), login)
	return Conta{Instancia: s.Name, Base: Base(base), ContaID: id, ContaIDFonte: fonte, ContaDir: login, TemLogin: temLogin(base, login), Arquivo: path}, true, nil
}

// ValidarPastaParaApagar limita --apagar-pasta a diretórios de conta
// reconhecíveis e impede apagar o diretório atual ou uma raiz de repositório.
func ValidarPastaParaApagar(base, dir, cwd, repo string) error {
	canon := func(p string) (string, error) {
		p, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			return "", err
		}
		if r, e := filepath.EvalSymlinks(p); e == nil {
			p = filepath.Clean(r)
		} else if !os.IsNotExist(e) {
			return "", e
		}
		return p, nil
	}
	target, err := canon(dir)
	if err != nil {
		return err
	}
	for _, protegido := range []string{cwd, repo} {
		if protegido == "" {
			continue
		}
		p, err := canon(protegido)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(target, p)
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return fmt.Errorf("recusa apagar %s: é o diretório atual ou contém o projeto", dir)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	home, err = canon(home)
	if err != nil {
		return err
	}
	base = Base(base)
	permitidos := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".config", "opencode"),
		filepath.Join(home, ".openheinerss", "accounts", base),
	}
	prefix := "." + base + "-"
	if base == "claude-code" {
		prefix = ".claude-"
	}
	if base == "opencode" {
		prefix = ".opencode-"
	}
	permitidos = append(permitidos, filepath.Join(home, prefix+"*"))
	permitido := false
	for _, p := range permitidos[:len(permitidos)-1] {
		p, _ = canon(p)
		if target == p {
			permitido = true
			break
		}
	}
	if !permitido {
		parent := filepath.Dir(target)
		permitido = parent == home && strings.HasPrefix(filepath.Base(target), prefix)
	}
	if !permitido {
		return fmt.Errorf("recusa apagar %s: a pasta não é um diretório padrão de conta do openheinerss", dir)
	}
	return nil
}

func Listar(dir string) ([]Conta, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Conta{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []Conta{}
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".json")) {
			continue
		}
		c, ok, err := ler(dir, filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, c)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Instancia < items[j].Instancia })
	return items, nil
}

// ListarCamadas mescla instâncias globais e do projeto. O projeto e a pasta
// global principal vencem instâncias antigas com o mesmo nome.
func ListarCamadas(globais []string, projeto string) ([]Conta, error) {
	porNome := map[string]Conta{}
	for i, dir := range globais {
		items, err := Listar(dir)
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			if _, existe := porNome[c.Instancia]; existe {
				continue
			}
			c.Origem = "global"
			if i > 0 {
				c.Origem = "global-legado"
			}
			porNome[c.Instancia] = c
		}
	}
	if projeto != "" {
		items, err := Listar(projeto)
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			c.Origem = "projeto"
			porNome[c.Instancia] = c
		}
	}
	items := make([]Conta, 0, len(porNome))
	for _, c := range porNome {
		items = append(items, c)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Instancia < items[j].Instancia })
	return items, nil
}

// Migrar copia as instâncias do projeto para a pasta global sem apagar as
// originais. Recusa colisões para nunca substituir uma conta existente.
func Migrar(projeto, global string) (int, error) {
	items, err := Listar(projeto)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(global, 0700); err != nil {
		return 0, err
	}
	_ = os.Chmod(global, 0700)
	for _, c := range items {
		target := arquivoInstancia(global, filepath.Base(c.Arquivo))
		if _, err := os.Stat(target); err == nil {
			return 0, fmt.Errorf("a instância %q já existe no global", c.Instancia)
		} else if !os.IsNotExist(err) {
			return 0, err
		}
		b, err := os.ReadFile(c.Arquivo)
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(target, b, 0600); err != nil {
			return 0, err
		}
		_ = os.Chmod(target, 0600)
	}
	return len(items), nil
}

func Adicionar(dir, harnessNome, apelido string) (Resultado, error) {
	base := Base(harnessNome)
	if !harness.Exists(base) {
		return Resultado{}, fmt.Errorf("harness %q não existe", harnessNome)
	}
	nome, err := NormalizarNome(apelido)
	if err != nil {
		return Resultado{}, err
	}
	instancia := base
	if base == "claude-code" {
		instancia = "claude"
	}
	instancia += "-" + nome
	if _, err := NormalizarNome(instancia); err != nil {
		return Resultado{}, err
	}
	if _, err := os.Stat(arquivoInstancia(dir, instancia)); err == nil {
		return Resultado{}, fmt.Errorf("a conta %q já existe", instancia)
	} else if !os.IsNotExist(err) {
		return Resultado{}, err
	}
	login, err := ContaDir(base, nome)
	if err != nil {
		return Resultado{}, err
	}
	if err := os.MkdirAll(login, 0700); err != nil {
		return Resultado{}, err
	}
	if err := os.Chmod(login, 0700); err != nil {
		return Resultado{}, err
	}
	s := harness.CustomSpec{Name: instancia, Base: base, Env: map[string]string{LoginEnv(base): login}}
	b, err := yaml.Marshal(&s)
	if err != nil {
		return Resultado{}, err
	}
	path := arquivoInstancia(dir, instancia)
	if err := os.WriteFile(path, b, 0600); err != nil {
		return Resultado{}, err
	}
	_ = os.Chmod(path, 0600)
	if err := harness.RegisterCustom(s); err != nil {
		return Resultado{}, err
	}
	id, fonte := hashID(base, login)
	return Resultado{Conta: Conta{Instancia: instancia, Base: base, ContaID: id, ContaIDFonte: fonte, ContaDir: login, TemLogin: false, Arquivo: path}, ComandoLogin: LoginCommand(base)}, nil
}

func Renomear(dir, antigo, novo string) (Conta, error) {
	old, err := NormalizarNome(antigo)
	if err != nil {
		return Conta{}, err
	}
	nn, err := NormalizarNome(novo)
	if err != nil {
		return Conta{}, err
	}
	items, err := Listar(dir)
	if err != nil {
		return Conta{}, err
	}
	var c Conta
	for _, item := range items {
		if item.Instancia == old {
			c = item
			break
		}
	}
	if c.Instancia == "" {
		return Conta{}, fmt.Errorf("conta %q não encontrada", old)
	}
	if _, err := os.Stat(arquivoInstancia(dir, nn)); err == nil {
		return Conta{}, fmt.Errorf("a conta %q já existe", nn)
	}
	b, err := os.ReadFile(c.Arquivo)
	if err != nil {
		return Conta{}, err
	}
	var s harness.CustomSpec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return Conta{}, err
	}
	s.Name = nn
	b, err = yaml.Marshal(&s)
	if err != nil {
		return Conta{}, err
	}
	target := arquivoInstancia(dir, nn)
	if err := os.WriteFile(target, b, 0600); err != nil {
		return Conta{}, err
	}
	if err := os.Remove(c.Arquivo); err != nil {
		return Conta{}, err
	}
	if err := harness.RegisterCustom(s); err != nil {
		return Conta{}, err
	}
	harness.UnregisterCustom(old)
	c.Instancia = nn
	c.Arquivo = target
	return c, nil
}

func Remover(dir, nome string) (Conta, error) {
	items, err := Listar(dir)
	if err != nil {
		return Conta{}, err
	}
	for _, c := range items {
		if c.Instancia == nome {
			if err := os.Remove(c.Arquivo); err != nil {
				return Conta{}, err
			}
			harness.UnregisterCustom(c.Instancia)
			return c, nil
		}
	}
	return Conta{}, fmt.Errorf("conta %q não encontrada", nome)
}
