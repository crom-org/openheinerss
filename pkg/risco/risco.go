// Package risco classifica chamadas de ferramenta e pedidos de permissão em baixo, medio ou alto.
// É opcional e desligado por padrão: a ponte continua só túnel. Quando ligado, só informa;
// nunca bloqueia nada.
package risco

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/crom-org/openheinerss/pkg/config"
	"gopkg.in/yaml.v3"
)

// Níveis de risco.
const (
	Baixo = "baixo"
	Medio = "medio"
	Alto  = "alto"
)

// ArquivoNome é o arquivo de regras por projeto (.openheinerss/risco.yaml) e global.
const ArquivoNome = "risco.yaml"

// Regra casa uma ferramenta e/ou um padrão (regex) no comando e atribui um nível.
type Regra struct {
	Nivel       string   `yaml:"nivel" json:"nivel"`
	Motivo      string   `yaml:"motivo" json:"motivo"`
	Ferramentas []string `yaml:"ferramentas,omitempty" json:"ferramentas,omitempty"` // vazio = qualquer
	Padrao      string   `yaml:"padrao,omitempty" json:"padrao,omitempty"`           // regex no texto do comando
	re          *regexp.Regexp
}

// Arquivo é o formato de risco.yaml.
type Arquivo struct {
	// Padrao é o nível quando nada casa (padrão: medio).
	Padrao string `yaml:"padrao,omitempty"`
	// SemEmbutidas desliga as regras embutidas (só valem as do arquivo).
	SemEmbutidas bool `yaml:"sem_embutidas,omitempty"`
	// ForaDaWorktree é o nível de escrita fora da pasta da sessão (padrão: alto).
	ForaDaWorktree string `yaml:"fora_da_worktree,omitempty"`
	// ForaPermitidos lista pastas fora da worktree onde escrever não sobe o risco (ex.: /tmp).
	ForaPermitidos []string `yaml:"fora_permitidos,omitempty"`
	// Leitura lista ferramentas de leitura (nível baixo); soma-se às embutidas.
	Leitura []string `yaml:"leitura,omitempty"`
	// Escrita lista ferramentas que escrevem arquivo (caminho checado contra a worktree).
	Escrita []string `yaml:"escrita,omitempty"`
	Regras  []Regra  `yaml:"regras,omitempty"`
}

// Classificador aplica as regras. É seguro para uso concorrente (só leitura depois de criado).
type Classificador struct {
	padrao, fora string
	permitidos   []string
	leitura      map[string]bool
	escrita      map[string]bool
	shell        map[string]bool
	regras       []Regra // regras do usuário (projeto antes do global), avaliadas primeiro
	embutidas    []Regra
}

var embutidas = []Regra{
	{Nivel: Alto, Motivo: "remoção recursiva (rm -r/-rf)", Padrao: `\brm\s+(-[a-zA-Z]*[rR][a-zA-Z]*|--recursive)\b`},
	{Nivel: Alto, Motivo: "git push (publica no remoto)", Padrao: `\bgit\s+push\b`},
	{Nivel: Alto, Motivo: "git destrutivo (reset --hard / clean -f)", Padrao: `\bgit\s+(reset\s+--hard|clean\s+-[a-zA-Z]*f)`},
	{Nivel: Alto, Motivo: "baixa e executa script (curl|sh)", Padrao: `\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`},
	{Nivel: Alto, Motivo: "deploy/publicação", Padrao: `(\bdeploy\b|\bkubectl\s+(apply|delete)\b|\bterraform\s+(apply|destroy)\b|\b(npm|cargo|pnpm|yarn)\s+publish\b|\btwine\s+upload\b|\bgh\s+release\s+create\b|\bdocker\s+push\b|\bgit\s+tag\b)`},
	{Nivel: Alto, Motivo: "comando de sistema perigoso (sudo, mkfs, dd, chmod 777, kill em massa)", Padrao: `(\bsudo\b|\bmkfs\b|\bdd\s+if=|\bchmod\s+(-R\s+)?777\b|\bpkill\b|\bkillall\b|\bshutdown\b|\breboot\b)`},
	{Nivel: Medio, Motivo: "rede (curl/wget)", Padrao: `\b(curl|wget)\b`},
	{Nivel: Medio, Motivo: "instala dependências", Padrao: `\b(npm|pnpm|yarn)\s+(install|add|i)\b|\bpip3?\s+install\b|\bgo\s+(get|install)\b|\bapt(-get)?\s+install\b`},
	{Nivel: Medio, Motivo: "altera o histórico git local", Padrao: `\bgit\s+(commit|merge|rebase|checkout|switch|branch\s+-[dD])\b`},
	{Nivel: Medio, Motivo: "remove arquivo", Padrao: `\brm\b`},
}

var (
	leituraPadrao = []string{"read", "glob", "grep", "ls", "list", "view", "search", "websearch", "webfetch", "todowrite", "todoread", "notebookread", "read_file", "list_dir", "find", "codesearch"}
	escritaPadrao = []string{"write", "edit", "multiedit", "notebookedit", "patch", "apply_patch", "write_file", "edit_file", "file_change", "str_replace"}
	shellPadrao   = []string{"bash", "shell", "sh", "exec", "exec_command", "command", "command_execution", "run", "terminal", "local_shell", "run_command"}
)

// Arquivos devolve os risco.yaml na ordem de prioridade (o primeiro vence): projeto, ~/.config/openheinerss, ~/.openheinerss.
func Arquivos(cwd string) []string {
	var out []string
	if cwd != "" {
		out = append(out, filepath.Join(cwd, config.WorkspaceDirName, ArquivoNome))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "openheinerss", ArquivoNome))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, config.WorkspaceDirName, ArquivoNome))
	}
	return out
}

// Carregar monta o classificador com as regras embutidas e as dos risco.yaml encontrados.
func Carregar(cwd string) (*Classificador, error) {
	var arqs []Arquivo
	visto := map[string]bool{}
	for _, f := range Arquivos(cwd) {
		abs, _ := filepath.Abs(f)
		if visto[abs] {
			continue
		}
		visto[abs] = true
		data, err := os.ReadFile(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("falha ao ler %s: %w", f, err)
		}
		var a Arquivo
		if err := yaml.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("formato inválido em %s: %w", f, err)
		}
		arqs = append(arqs, a)
	}
	return Novo(arqs...)
}

// Novo monta o classificador a partir de arquivos já lidos (o primeiro tem prioridade).
func Novo(arqs ...Arquivo) (*Classificador, error) {
	c := &Classificador{padrao: Medio, fora: Alto, leitura: map[string]bool{}, escrita: map[string]bool{}, shell: map[string]bool{}}
	for _, n := range leituraPadrao {
		c.leitura[n] = true
	}
	for _, n := range escritaPadrao {
		c.escrita[n] = true
	}
	for _, n := range shellPadrao {
		c.shell[n] = true
	}
	semEmbutidas := false
	// Do menos prioritário ao mais prioritário: os escalares do primeiro arquivo vencem.
	for i := len(arqs) - 1; i >= 0; i-- {
		a := arqs[i]
		if a.Padrao != "" {
			if !nivelValido(a.Padrao) {
				return nil, fmt.Errorf("risco.yaml: padrao '%s' inválido (use baixo, medio ou alto)", a.Padrao)
			}
			c.padrao = a.Padrao
		}
		if a.ForaDaWorktree != "" {
			if !nivelValido(a.ForaDaWorktree) {
				return nil, fmt.Errorf("risco.yaml: fora_da_worktree '%s' inválido", a.ForaDaWorktree)
			}
			c.fora = a.ForaDaWorktree
		}
		semEmbutidas = semEmbutidas || a.SemEmbutidas
		c.permitidos = append(c.permitidos, a.ForaPermitidos...)
		for _, n := range a.Leitura {
			c.leitura[strings.ToLower(n)] = true
		}
		for _, n := range a.Escrita {
			c.escrita[strings.ToLower(n)] = true
		}
	}
	for _, a := range arqs {
		for _, r := range a.Regras {
			if err := preparar(&r); err != nil {
				return nil, err
			}
			c.regras = append(c.regras, r)
		}
	}
	if !semEmbutidas {
		for _, r := range embutidas {
			if err := preparar(&r); err != nil {
				return nil, err
			}
			c.embutidas = append(c.embutidas, r)
		}
	}
	return c, nil
}

func nivelValido(n string) bool { return n == Baixo || n == Medio || n == Alto }

func preparar(r *Regra) error {
	if !nivelValido(r.Nivel) {
		return fmt.Errorf("risco.yaml: regra '%s' com nível '%s' inválido (use baixo, medio ou alto)", r.Motivo, r.Nivel)
	}
	if r.Padrao == "" && len(r.Ferramentas) == 0 {
		return fmt.Errorf("risco.yaml: regra '%s' precisa de padrao ou ferramentas", r.Motivo)
	}
	if r.Padrao != "" {
		re, err := regexp.Compile(r.Padrao)
		if err != nil {
			return fmt.Errorf("risco.yaml: padrao inválido na regra '%s': %w", r.Motivo, err)
		}
		r.re = re
	}
	if r.Motivo == "" {
		r.Motivo = "regra " + r.Padrao
	}
	return nil
}

func (r Regra) casa(tool, texto string) bool {
	if len(r.Ferramentas) > 0 {
		ok := false
		for _, f := range r.Ferramentas {
			if strings.EqualFold(f, tool) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return r.re == nil || r.re.MatchString(texto)
}

// Classificar devolve o nível e o motivo de uma chamada. command é o texto do comando quando
// já se sabe (permission_request); input é o input da ferramenta (tool_call).
func (c *Classificador) Classificar(tool, command string, input interface{}, cwd string) (nivel, motivo string) {
	campos := extrair(input)
	if command == "" {
		command = primeiro(campos, "command", "cmd", "script", "commandLine")
	}
	texto := command
	if texto == "" && input != nil {
		if b, err := json.Marshal(input); err == nil {
			texto = string(b)
		}
	}
	nome := normalizar(tool)
	for _, r := range c.regras {
		if r.casa(tool, texto) {
			return r.Nivel, r.Motivo
		}
	}
	// Escrita fora da worktree: caminho de ferramenta de escrita ou redirecionamento no shell.
	if c.escrita[nome] {
		if p := primeiro(campos, "file_path", "filePath", "path", "notebook_path", "file"); p != "" && c.foraDaPasta(p, cwd) {
			return c.fora, "escrita fora da worktree (" + p + ")"
		}
	}
	if command != "" {
		for _, alvo := range redirecionamentos(command) {
			if c.foraDaPasta(alvo, cwd) {
				return c.fora, "escrita fora da worktree (" + alvo + ")"
			}
		}
	}
	for _, r := range c.embutidas {
		if r.casa(tool, texto) && (command != "" || c.shell[nome]) {
			return r.Nivel, r.Motivo
		}
	}
	switch {
	case c.leitura[nome]:
		return Baixo, "leitura"
	case c.escrita[nome]:
		return Medio, "escreve arquivo na worktree"
	case c.shell[nome] || command != "":
		return Medio, "comando de shell sem regra específica"
	}
	return c.padrao, "ferramenta sem regra específica"
}

func normalizar(tool string) string {
	t := strings.ToLower(strings.TrimSpace(tool))
	// Namespaces ("functions.shell", "tools:bash") ficam pelo último pedaço.
	if i := strings.LastIndexAny(t, ".:"); i >= 0 {
		t = t[i+1:]
	}
	return t
}

func extrair(input interface{}) map[string]interface{} {
	switch v := input.(type) {
	case map[string]interface{}:
		return v
	case string:
		var m map[string]interface{}
		if json.Unmarshal([]byte(v), &m) == nil {
			return m
		}
		return map[string]interface{}{"command": v}
	case nil:
		return nil
	}
	var m map[string]interface{}
	if b, err := json.Marshal(input); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func primeiro(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case []interface{}:
			parts := make([]string, 0, len(v))
			for _, p := range v {
				parts = append(parts, fmt.Sprint(p))
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
	}
	return ""
}

var reRedir = regexp.MustCompile(`(?:^|[^0-9&<>])>{1,2}\s*([^\s;|&<>]+)|\btee\s+(?:-a\s+)?([^\s;|&<>]+)`)

func redirecionamentos(cmd string) []string {
	var out []string
	for _, m := range reRedir.FindAllStringSubmatch(cmd, -1) {
		for _, g := range m[1:] {
			if g != "" {
				out = append(out, strings.Trim(g, `"'`))
			}
		}
	}
	return out
}

// foraDaPasta diz se o caminho (absoluto ou com ~) cai fora de cwd. Relativos contam como dentro,
// exceto quando sobem com "..".
func (c *Classificador) foraDaPasta(p, cwd string) bool {
	if cwd == "" || p == "" {
		return false
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if strings.HasPrefix(p, "/dev/") {
		return false
	}
	for _, perm := range append([]string{cwd}, c.permitidos...) {
		perm = filepath.Clean(perm)
		if p == perm || strings.HasPrefix(p, perm+string(filepath.Separator)) {
			return false
		}
	}
	return true
}
