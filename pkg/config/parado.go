package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Ações possíveis quando o agente é dado como parado.
const (
	ParadoAcaoAviso = "aviso"
	ParadoAcaoParar = "parar"
)

// Padrões do detector de agente parado.
const (
	ParadoAvisoPadrao = 10 * time.Minute
	ParadoPararPadrao = 20 * time.Minute
)

// Variáveis de ambiente irmãs de OPENHEINERSS_LOG_PARADO_MIN (minutos; 0 desliga).
const (
	EnvParadoAvisoMin = "OPENHEINERSS_PARADO_AVISO_MIN"
	EnvParadoPararMin = "OPENHEINERSS_PARADO_PARAR_MIN"
	EnvParadoAcao     = "OPENHEINERSS_PARADO_ACAO"
)

// ParadoConfig é a chave `parado:` do config.yaml (global ou de projeto). Campo ausente = herda.
type ParadoConfig struct {
	AvisoMin *int   `json:"aviso_min,omitempty" yaml:"aviso_min"`
	PararMin *int   `json:"parar_min,omitempty" yaml:"parar_min"`
	Acao     string `json:"acao,omitempty" yaml:"acao"`
}

// Parado é a regra efetiva do detector. Aviso 0 desliga o detector; Parar 0 nunca interrompe.
type Parado struct {
	Aviso, Parar                         time.Duration
	Acao                                 string
	OrigemAviso, OrigemParar, OrigemAcao string
}

// Ligado diz se o detector está ligado.
func (p Parado) Ligado() bool { return p.Aviso > 0 }

// PodeParar diz se o detector interrompe o agente (ação parar e limite de parada definido).
func (p Parado) PodeParar() bool { return p.Ligado() && p.Acao == ParadoAcaoParar && p.Parar > 0 }

type arquivoParado struct {
	Parado *ParadoConfig `yaml:"parado"`
}

// ValidarAcaoParado aceita vazio, aviso ou parar.
func ValidarAcaoParado(acao string) error {
	switch acao {
	case "", ParadoAcaoAviso, ParadoAcaoParar:
		return nil
	}
	return fmt.Errorf("acao %q inválida (use %s ou %s)", acao, ParadoAcaoAviso, ParadoAcaoParar)
}

func (c *ParadoConfig) validar() error {
	if c.AvisoMin != nil && *c.AvisoMin < 0 {
		return fmt.Errorf("aviso_min negativo (%d)", *c.AvisoMin)
	}
	if c.PararMin != nil && *c.PararMin < 0 {
		return fmt.Errorf("parar_min negativo (%d)", *c.PararMin)
	}
	return ValidarAcaoParado(c.Acao)
}

func lerParadoArquivo(path string) (*ParadoConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("falha ao ler %s: %w", path, err)
	}
	var f arquivoParado
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("formato inválido em %s: %w", path, err)
	}
	if f.Parado == nil {
		return nil, nil
	}
	if err := f.Parado.validar(); err != nil {
		return nil, fmt.Errorf("%s: parado: %w", path, err)
	}
	return f.Parado, nil
}

// sobrepor põe o que vem de cima sobre c, campo a campo.
func (c ParadoConfig) sobrepor(cima *ParadoConfig) ParadoConfig {
	if cima == nil {
		return c
	}
	if cima.AvisoMin != nil {
		c.AvisoMin = cima.AvisoMin
	}
	if cima.PararMin != nil {
		c.PararMin = cima.PararMin
	}
	if cima.Acao != "" {
		c.Acao = cima.Acao
	}
	return c
}

// ParadoEfetivo resolve a regra, campo a campo: projeto > global > variáveis de ambiente > padrão.
// (As flags do rodar vencem tudo: AplicarFlagsParado.)
func ParadoEfetivo(repo string) (Parado, error) {
	var proj *ParadoConfig
	if repo != "" {
		p, err := lerParadoArquivo(filepath.Join(repo, WorkspaceDirName, ConfigFileName))
		if err != nil {
			return Parado{}, err
		}
		proj = p
	}
	files, err := ArquivosConfigGlobais()
	if err != nil {
		return Parado{}, err
	}
	var glob ParadoConfig
	for _, f := range files {
		c, err := lerParadoArquivo(f)
		if err != nil {
			return Parado{}, err
		}
		glob = glob.sobrepor(c)
	}
	return resolverParado(proj, &glob, ambienteParado()), nil
}

func ambienteParado() *ParadoConfig {
	var c ParadoConfig
	minutos := func(nome string) *int {
		n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(nome)))
		if err != nil || n < 0 {
			return nil
		}
		return &n
	}
	c.AvisoMin, c.PararMin = minutos(EnvParadoAvisoMin), minutos(EnvParadoPararMin)
	if a := strings.TrimSpace(os.Getenv(EnvParadoAcao)); ValidarAcaoParado(a) == nil {
		c.Acao = a
	}
	return &c
}

func resolverParado(proj, glob, env *ParadoConfig) Parado {
	out := Parado{Aviso: ParadoAvisoPadrao, Parar: ParadoPararPadrao, Acao: ParadoAcaoAviso,
		OrigemAviso: OrigemPadrao, OrigemParar: OrigemPadrao, OrigemAcao: OrigemPadrao}
	camadas := []struct {
		c      *ParadoConfig
		escopo string
	}{{env, "env"}, {glob, OrigemGlobal}, {proj, OrigemProjeto}} // da mais fraca para a mais forte
	for _, cam := range camadas {
		if cam.c == nil {
			continue
		}
		if cam.c.AvisoMin != nil {
			out.Aviso, out.OrigemAviso = time.Duration(*cam.c.AvisoMin)*time.Minute, cam.escopo
		}
		if cam.c.PararMin != nil {
			out.Parar, out.OrigemParar = time.Duration(*cam.c.PararMin)*time.Minute, cam.escopo
		}
		if cam.c.Acao != "" {
			out.Acao, out.OrigemAcao = cam.c.Acao, cam.escopo
		}
	}
	return out
}

// AplicarFlagsParado faz --parado-aviso/--parado-parar vencerem a configuração (nil = flag não dada;
// 0 desliga). Dar --parado-parar com valor > 0 liga a ação "parar".
func AplicarFlagsParado(p Parado, aviso, parar *time.Duration) (Parado, error) {
	if aviso != nil {
		if *aviso < 0 {
			return p, fmt.Errorf("--parado-aviso negativo (%s)", *aviso)
		}
		p.Aviso, p.OrigemAviso = *aviso, OrigemFlag
	}
	if parar != nil {
		if *parar < 0 {
			return p, fmt.Errorf("--parado-parar negativo (%s)", *parar)
		}
		p.Parar, p.OrigemParar = *parar, OrigemFlag
		if *parar > 0 {
			p.Acao, p.OrigemAcao = ParadoAcaoParar, OrigemFlag
		}
	}
	return p, nil
}

// Descrever resume a regra em uma linha, com a origem de cada campo.
func (p Parado) Descrever() string {
	if !p.Ligado() {
		return fmt.Sprintf("desligado (aviso 0 vindo de %s)", p.OrigemAviso)
	}
	return fmt.Sprintf("aviso %s (origem: %s); parar %s (origem: %s); acao %s (origem: %s)", p.Aviso, p.OrigemAviso, p.Parar, p.OrigemParar, p.Acao, p.OrigemAcao)
}
