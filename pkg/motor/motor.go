// Package motor centraliza os nomes estáveis usados pela Crom para trocar de
// ferramenta sem espalhar detalhes de cada CLI pelo restante do projeto.
package motor

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
)

// Perfil é a configuração efetiva de um motor.
type Perfil struct {
	Nome, Harness, Mode, Provider, Model, Effort string
	Env                                          map[string]string
}

var harnessesBase = []string{"aider", "agy", "claude-code", "codex", "mock", "opencode"}

func perfisPadrao() []Perfil {
	result := make([]Perfil, 0, len(harnessesBase))
	for _, name := range harnessesBase {
		result = append(result, Perfil{Nome: name, Harness: name, Mode: "cli"})
	}
	for i := range result {
		for j := i + 1; j < len(result); j++ {
			if result[j].Nome < result[i].Nome {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// Resolve resolve um harness base ou uma instância carregada, aplicando modelo e esforço opcionais.
func Resolve(nome, modelo, esforco string) (Perfil, error) {
	base := false
	for _, name := range harnessesBase {
		if name == nome {
			base = true
			break
		}
	}
	if !base && !harness.Exists(nome) {
		return Perfil{}, fmt.Errorf("motor '%s' desconhecido; defina-o em .openheinerss/harnesses ou use um harness base", nome)
	}
	p := Perfil{Nome: nome, Harness: nome, Mode: "cli"}
	if modelo != "" {
		p.Model = modelo
	}
	if esforco != "" {
		p.Effort = esforco
	}
	p.Env = expandEnv(p.Env)
	return p, nil
}

// LoadRoles lê o formato simples de uma linha: papel: motor/modelo [esforco=high].
// Ele é YAML válido para este caso comum e evita uma dependência pesada.
func LoadRoles(path string) (map[string]Perfil, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir configuração de motores %s: %w", path, err)
	}
	defer f.Close()
	roles := make(map[string]Perfil)
	s := bufio.NewScanner(f)
	linha := 0
	for s.Scan() {
		linha++
		raw := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("configuração %s linha %d: esperado papel: motor/modelo", path, linha)
		}
		role, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return nil, fmt.Errorf("configuração %s linha %d: motor vazio", path, linha)
		}
		effort := ""
		spec := fields[0]
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "esforco=") {
				effort = strings.TrimPrefix(f, "esforco=")
			}
		}
		name, model := spec, ""
		if slash := strings.Index(spec, "/"); slash >= 0 {
			name, model = spec[:slash], spec[slash+1:]
		}
		p, err := Resolve(name, model, effort)
		if err != nil {
			return nil, fmt.Errorf("papel '%s': %w", role, err)
		}
		roles[role] = p
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("ler configuração de motores: %w", err)
	}
	return roles, nil
}

// FindRoles lê motores.yaml da pasta de configuração explícita (--config ou
// OPENHEINERSS_CONFIG); sem ela, de <cwd>/.openheinerss/motores.yaml.
func FindRoles(cwd string) (map[string]Perfil, string, error) {
	path := filepath.Join(cwd, ".openheinerss", "motores.yaml")
	if dir, err := config.ConfigDir(); err != nil {
		return nil, "", err
	} else if dir != "" {
		path = filepath.Join(dir, "motores.yaml")
	}
	roles, err := LoadRoles(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]Perfil{}, path, nil
		}
		return nil, path, err
	}
	return roles, path, nil
}

func Perfis() []Perfil { return perfisPadrao() }

func expandEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if strings.HasPrefix(v, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				v = filepath.Join(home, v[2:])
			}
		}
		out[k] = v
	}
	return out
}
