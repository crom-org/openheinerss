package comandos

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"gopkg.in/yaml.v3"
)

// NomeArquivo é o arquivo de anotações do usuário (na pasta de --config/OPENHEINERSS_CONFIG
// e em ~/.config/openheinerss/).
const NomeArquivo = "comandos.yaml"

// Comando é um comando nativo do harness, já mesclado com a anotação do usuário.
type Comando struct {
	Nome       string `json:"nome"`
	Descricao  string `json:"descricao"`
	Repasse    string `json:"repasse"`
	Detalhe    string `json:"detalhe,omitempty"`
	Origem     string `json:"origem"`
	Anotacao   string `json:"anotacao,omitempty"`
	Confirmado bool   `json:"confirmado"`
}

// Lista é a resposta de `comandos <harness>` e de harness.comandos.
type Lista struct {
	Harness string `json:"harness"`
	Base    string `json:"base"`
	// Cadeia vai da base até a instância pedida (as anotações de cada nível valem para os seguintes).
	Cadeia []string `json:"cadeia"`
	// Desconhecido diz como a ponte repassa um /x que não está na lista.
	Desconhecido string    `json:"desconhecido"`
	Arquivo      string    `json:"arquivo"`
	Comandos     []Comando `json:"comandos"`
}

// anotacao é uma entrada de comandos.yaml.
type anotacao struct {
	Descricao  string `yaml:"descricao,omitempty"`
	Anotacao   string `yaml:"anotacao,omitempty"`
	Confirmado *bool  `yaml:"confirmado,omitempty"`
}

type arquivoAnotacoes map[string]map[string]anotacao

var gravarMu sync.Mutex

// Cadeia devolve a herança de nome (da base embutida até a própria instância).
func Cadeia(nome string) ([]string, error) {
	if !existe(nome) {
		return nil, fmt.Errorf("harness '%s' não existe", nome)
	}
	cadeia := []string{nome}
	visto := map[string]bool{nome: true}
	for atual := nome; ; {
		spec, ok := harness.CustomSpecFor(atual)
		if !ok || spec.Base == "" || visto[spec.Base] {
			break
		}
		atual = spec.Base
		visto[atual] = true
		cadeia = append([]string{atual}, cadeia...)
	}
	return cadeia, nil
}

func existe(nome string) bool {
	if harness.Exists(nome) {
		return true
	}
	if _, ok := harness.CustomSpecFor(nome); ok {
		return true
	}
	_, ok := embutido[nome]
	return ok
}

// NormalizarNome devolve "/nome" e valida que é um comando (caminhos como /home/x não são).
func NormalizarNome(cmd string) (string, error) {
	c := strings.TrimSpace(cmd)
	if !strings.HasPrefix(c, "/") {
		c = "/" + c
	}
	nome, rest, ok := harness.SlashCommand(c)
	if !ok || rest != "" {
		return "", fmt.Errorf("comando inválido %q: use /nome (sem argumentos)", cmd)
	}
	return "/" + nome, nil
}

// ArquivoGlobal é ~/.config/openheinerss/comandos.yaml (respeita XDG_CONFIG_HOME).
func ArquivoGlobal() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "openheinerss", NomeArquivo)
}

// arquivos devolve os arquivos lidos (o último vence) e o arquivo onde se grava.
func arquivos() ([]string, string, error) {
	var lidos []string
	if g := ArquivoGlobal(); g != "" {
		lidos = append(lidos, g)
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return nil, "", err
	}
	if dir != "" {
		lidos = append(lidos, filepath.Join(dir, NomeArquivo))
	}
	if len(lidos) == 0 {
		return nil, "", fmt.Errorf("não achei a pasta de configuração do usuário; use --config")
	}
	return lidos, lidos[len(lidos)-1], nil
}

func lerAnotacoes(path string) (arquivoAnotacoes, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a arquivoAnotacoes
	if err := yaml.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("ler %s: %w", path, err)
	}
	return a, nil
}

// Listar monta a lista de comandos de um harness ou instância: catálogo embutido da base,
// comandos descobertos nos arquivos do harness (cwd = pasta do projeto, pode ser vazia)
// e as anotações do usuário de cada nível da herança.
func Listar(nome, cwd string) (Lista, error) {
	cadeia, err := Cadeia(nome)
	if err != nil {
		return Lista{}, err
	}
	lidos, alvo, err := arquivos()
	if err != nil {
		return Lista{}, err
	}
	base := cadeia[0]
	l := Lista{Harness: nome, Base: base, Cadeia: cadeia, Desconhecido: RepasseLiteral, Arquivo: alvo}
	if p, ok := padraoDesconhecido[base]; ok {
		l.Desconhecido = p
	}
	porNome := map[string]*Comando{}
	var ordem []string
	add := func(c Comando) {
		if _, ok := porNome[c.Nome]; ok {
			return
		}
		cp := c
		porNome[c.Nome] = &cp
		ordem = append(ordem, c.Nome)
	}
	for _, it := range embutido[base] {
		add(Comando{Nome: it.nome, Descricao: it.descricao, Repasse: it.repasse, Detalhe: it.detalhe, Origem: OrigemEmbutido})
	}
	for _, c := range descobrir(base, envDe(nome), cwd) {
		add(c)
	}
	for _, path := range lidos {
		a, err := lerAnotacoes(path)
		if err != nil {
			return Lista{}, err
		}
		for _, h := range cadeia {
			for cmd, an := range a[h] {
				n, err := NormalizarNome(cmd)
				if err != nil {
					continue
				}
				c, ok := porNome[n]
				if !ok {
					add(Comando{Nome: n, Repasse: l.Desconhecido, Origem: OrigemUsuario})
					c = porNome[n]
				}
				if an.Descricao != "" {
					c.Descricao = an.Descricao
				}
				if an.Anotacao != "" {
					c.Anotacao = an.Anotacao
				}
				if an.Confirmado != nil {
					c.Confirmado = *an.Confirmado
				}
			}
		}
	}
	sort.Strings(ordem)
	l.Comandos = make([]Comando, 0, len(ordem))
	for _, n := range ordem {
		l.Comandos = append(l.Comandos, *porNome[n])
	}
	return l, nil
}

// Anotar grava o texto livre de um comando para o harness/instância e devolve o comando mesclado.
func Anotar(nome, cmd, texto, cwd string) (Comando, error) {
	return gravar(nome, cmd, cwd, "anotacao", strings.TrimSpace(texto), "!!str")
}

// Confirmar marca o primeiro uso do comando como já confirmado.
func Confirmar(nome, cmd, cwd string) (Comando, error) {
	return gravar(nome, cmd, cwd, "confirmado", "true", "!!bool")
}

func gravar(nome, cmd, cwd, campo, valor, tag string) (Comando, error) {
	if _, err := Cadeia(nome); err != nil {
		return Comando{}, err
	}
	n, err := NormalizarNome(cmd)
	if err != nil {
		return Comando{}, err
	}
	_, alvo, err := arquivos()
	if err != nil {
		return Comando{}, err
	}
	gravarMu.Lock()
	err = editarYAML(alvo, []string{nome, n, campo}, valor, tag)
	gravarMu.Unlock()
	if err != nil {
		return Comando{}, err
	}
	l, err := Listar(nome, cwd)
	if err != nil {
		return Comando{}, err
	}
	for _, c := range l.Comandos {
		if c.Nome == n {
			return c, nil
		}
	}
	return Comando{}, fmt.Errorf("comando %s sumiu depois de gravar", n)
}

// editarYAML troca um valor no caminho dado preservando comentários e a ordem do arquivo.
func editarYAML(path string, chaves []string, valor, tag string) error {
	var doc yaml.Node
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(strings.TrimSpace(string(b))) > 0 {
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("ler %s: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	no := doc.Content[0]
	if no.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: esperava um mapa harness → comandos", path)
	}
	for i, k := range chaves {
		ultimo := i == len(chaves)-1
		var achado *yaml.Node
		for j := 0; j+1 < len(no.Content); j += 2 {
			if no.Content[j].Value == k {
				achado = no.Content[j+1]
				break
			}
		}
		if ultimo {
			if achado != nil {
				*achado = yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: valor, Style: estilo(tag)}
			} else {
				no.Content = append(no.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: valor, Style: estilo(tag)})
			}
			break
		}
		if achado == nil || achado.Kind != yaml.MappingNode {
			novo := &yaml.Node{Kind: yaml.MappingNode}
			if achado != nil {
				*achado = *novo
				novo = achado
			} else {
				no.Content = append(no.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k, Style: estiloChave(k)}, novo)
			}
			achado = novo
		}
		no = achado
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	out := buf.Bytes()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func estilo(tag string) yaml.Style {
	if tag == "!!str" {
		return yaml.DoubleQuotedStyle
	}
	return 0
}

// estiloChave põe aspas em "/compact" para o arquivo ficar óbvio de editar à mão.
func estiloChave(k string) yaml.Style {
	if strings.HasPrefix(k, "/") {
		return yaml.DoubleQuotedStyle
	}
	return 0
}
