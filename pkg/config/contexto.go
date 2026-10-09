package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Ações possíveis quando o contexto passa do limite.
const (
	AcaoAviso      = "aviso"
	AcaoNovaSessao = "nova-sessao"
)

// Escopos de origem de um valor.
const (
	OrigemProjeto = "projeto"
	OrigemGlobal  = "global"
	OrigemFlag    = "flag"
	OrigemPadrao  = "padrão"
)

// CamadaContexto é um bloco `{limite_tokens, acao}` do config.yaml. Campo ausente = herda da camada de baixo.
type CamadaContexto struct {
	LimiteTokens *int   `json:"limite_tokens,omitempty" yaml:"limite_tokens"`
	Acao         string `json:"acao,omitempty" yaml:"acao"`
}

// ContextoConfig é a chave `contexto:` do config.yaml (global ou de projeto).
type ContextoConfig struct {
	Padrao    CamadaContexto            `json:"padrao" yaml:"padrao"`
	Harnesses map[string]CamadaContexto `json:"harnesses,omitempty" yaml:"harnesses"`
}

// Contexto é a regra efetiva de uma execução. LimiteTokens 0 = desligado.
// Origem* é o escopo (projeto, global, flag, padrão); Camada* diz de qual bloco veio.
type Contexto struct {
	LimiteTokens int    `json:"limite_tokens"`
	Acao         string `json:"acao"`
	OrigemLimite string `json:"origem_limite"`
	OrigemAcao   string `json:"origem_acao"`
	CamadaLimite string `json:"camada_limite,omitempty"`
	CamadaAcao   string `json:"camada_acao,omitempty"`
}

// Ligado diz se há limite configurado.
func (c Contexto) Ligado() bool { return c.LimiteTokens > 0 }

type arquivoContexto struct {
	Contexto *ContextoConfig `yaml:"contexto"`
}

// ArquivosConfigGlobais devolve os config.yaml globais, do que perde para o que vence:
// com --config/OPENHEINERSS_CONFIG só o dessa pasta; senão ~/.config/openheinerss e
// ~/.openheinerss (este vence, como o mcp.json).
func ArquivosConfigGlobais() ([]string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	if dir != "" {
		return []string{filepath.Join(dir, ConfigFileName)}, nil
	}
	var out []string
	if cfg, err := UserConfigDir(); err == nil {
		out = append(out, filepath.Join(cfg, "openheinerss", ConfigFileName))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, WorkspaceDirName, ConfigFileName))
	}
	return out, nil
}

// lerContextoArquivo lê a chave contexto de um config.yaml. Arquivo ou chave ausentes devolvem nil.
func lerContextoArquivo(path string) (*ContextoConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("falha ao ler %s: %w", path, err)
	}
	var f arquivoContexto
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("formato inválido em %s: %w", path, err)
	}
	if f.Contexto == nil {
		return nil, nil
	}
	if err := f.Contexto.validar(); err != nil {
		return nil, fmt.Errorf("%s: contexto: %w", path, err)
	}
	return f.Contexto, nil
}

func (c CamadaContexto) validar() error {
	if c.LimiteTokens != nil && *c.LimiteTokens < 0 {
		return fmt.Errorf("limite_tokens negativo (%d)", *c.LimiteTokens)
	}
	return ValidarAcaoContexto(c.Acao)
}

func (c *ContextoConfig) validar() error {
	if err := c.Padrao.validar(); err != nil {
		return fmt.Errorf("padrao: %w", err)
	}
	for nome, camada := range c.Harnesses {
		if err := camada.validar(); err != nil {
			return fmt.Errorf("harnesses.%s: %w", nome, err)
		}
	}
	return nil
}

// ValidarAcaoContexto aceita vazio, aviso ou nova-sessao.
func ValidarAcaoContexto(acao string) error {
	switch acao {
	case "", AcaoAviso, AcaoNovaSessao:
		return nil
	}
	return fmt.Errorf("acao %q inválida (use %s ou %s)", acao, AcaoAviso, AcaoNovaSessao)
}

// mesclar põe o que vem de cima (outro) sobre c, campo a campo.
func (c *ContextoConfig) mesclar(outro *ContextoConfig) {
	if outro == nil {
		return
	}
	c.Padrao = sobrepor(c.Padrao, outro.Padrao)
	for nome, camada := range outro.Harnesses {
		if c.Harnesses == nil {
			c.Harnesses = map[string]CamadaContexto{}
		}
		c.Harnesses[nome] = sobrepor(c.Harnesses[nome], camada)
	}
}

func sobrepor(base, cima CamadaContexto) CamadaContexto {
	if cima.LimiteTokens != nil {
		base.LimiteTokens = cima.LimiteTokens
	}
	if cima.Acao != "" {
		base.Acao = cima.Acao
	}
	return base
}

// ContextoGlobal junta os config.yaml globais (o mais forte vence campo a campo).
func ContextoGlobal() (*ContextoConfig, error) {
	files, err := ArquivosConfigGlobais()
	if err != nil {
		return nil, err
	}
	out := &ContextoConfig{}
	for _, f := range files {
		c, err := lerContextoArquivo(f)
		if err != nil {
			return nil, err
		}
		out.mesclar(c)
	}
	return out, nil
}

// ContextoProjeto lê a chave contexto de <repo>/.openheinerss/config.yaml (nil se não houver).
func ContextoProjeto(repo string) (*ContextoConfig, error) {
	if repo == "" {
		return nil, nil
	}
	return lerContextoArquivo(filepath.Join(repo, WorkspaceDirName, ConfigFileName))
}

type camadaNomeada struct {
	nome, escopo string
	c            CamadaContexto
}

// ContextoEfetivo resolve a regra de contexto de uma execução da instância `instancia` (harness base
// `base`), campo a campo, na ordem: projeto.harnesses[instância] > projeto.harnesses[base] >
// projeto.padrao > global.harnesses[instância] > global.harnesses[base] > global.padrao.
// Sem nada configurado o limite fica 0 (desligado).
func ContextoEfetivo(repo, instancia, base string) (Contexto, error) {
	proj, err := ContextoProjeto(repo)
	if err != nil {
		return Contexto{}, err
	}
	glob, err := ContextoGlobal()
	if err != nil {
		return Contexto{}, err
	}
	return resolverContexto(proj, glob, instancia, base), nil
}

func resolverContexto(proj, glob *ContextoConfig, instancia, base string) Contexto {
	var camadas []camadaNomeada
	add := func(cfg *ContextoConfig, escopo string) {
		if cfg == nil {
			return
		}
		if instancia != "" {
			if c, ok := cfg.Harnesses[instancia]; ok {
				camadas = append(camadas, camadaNomeada{escopo + " harnesses." + instancia, escopo, c})
			}
		}
		if base != "" && base != instancia {
			if c, ok := cfg.Harnesses[base]; ok {
				camadas = append(camadas, camadaNomeada{escopo + " harnesses." + base, escopo, c})
			}
		}
		camadas = append(camadas, camadaNomeada{escopo + " padrao", escopo, cfg.Padrao})
	}
	add(proj, OrigemProjeto)
	add(glob, OrigemGlobal)

	out := Contexto{Acao: AcaoAviso, OrigemAcao: OrigemPadrao}
	achouLimite, achouAcao := false, false
	for _, c := range camadas {
		if !achouLimite && c.c.LimiteTokens != nil {
			out.LimiteTokens, out.OrigemLimite, out.CamadaLimite = *c.c.LimiteTokens, c.escopo, c.nome
			achouLimite = true
		}
		if !achouAcao && c.c.Acao != "" {
			out.Acao, out.OrigemAcao, out.CamadaAcao = c.c.Acao, c.escopo, c.nome
			achouAcao = true
		}
	}
	return out
}

// AplicarFlagsContexto faz as flags do `rodar` vencerem a configuração. limite nil = flag não dada
// (0 desliga); acao vazia = flag não dada.
func AplicarFlagsContexto(c Contexto, limite *int, acao string) (Contexto, error) {
	if limite != nil {
		if *limite < 0 {
			return c, fmt.Errorf("--limite-contexto negativo (%d)", *limite)
		}
		c.LimiteTokens, c.OrigemLimite, c.CamadaLimite = *limite, OrigemFlag, "--limite-contexto"
	}
	if acao != "" {
		if err := ValidarAcaoContexto(acao); err != nil {
			return c, fmt.Errorf("--acao-contexto: %w", err)
		}
		c.Acao, c.OrigemAcao, c.CamadaAcao = acao, OrigemFlag, "--acao-contexto"
	}
	return c, nil
}

// NomesHarnessContexto lista os harnesses/instâncias citados em `contexto.harnesses` (projeto e global).
func NomesHarnessContexto(repo string) ([]string, error) {
	proj, err := ContextoProjeto(repo)
	if err != nil {
		return nil, err
	}
	glob, err := ContextoGlobal()
	if err != nil {
		return nil, err
	}
	visto := map[string]bool{}
	var nomes []string
	for _, cfg := range []*ContextoConfig{proj, glob} {
		if cfg == nil {
			continue
		}
		for n := range cfg.Harnesses {
			if !visto[n] {
				visto[n] = true
				nomes = append(nomes, n)
			}
		}
	}
	sort.Strings(nomes)
	return nomes, nil
}

// Descrever resume a regra em uma linha, com a origem de cada campo.
func (c Contexto) Descrever() string {
	if !c.Ligado() {
		if c.OrigemLimite == "" {
			return "desligado (nenhum limite configurado)"
		}
		return fmt.Sprintf("desligado (limite 0 vindo de %s)", c.CamadaLimite)
	}
	camada := func(cam, escopo string) string {
		if cam == "" {
			return escopo
		}
		return cam
	}
	return fmt.Sprintf("limite_tokens %d (origem: %s); acao %s (origem: %s)", c.LimiteTokens, camada(c.CamadaLimite, c.OrigemLimite), c.Acao, camada(c.CamadaAcao, c.OrigemAcao))
}
