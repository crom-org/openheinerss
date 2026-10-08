// Package motor centraliza os nomes estáveis usados pela Crom para trocar de
// ferramenta sem espalhar detalhes de cada CLI pelo restante do projeto.
package motor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Perfil é a configuração efetiva de um motor.
type Perfil struct {
	Nome, Harness, Mode, Provider, Model, Effort string
	Env                                          map[string]string
}

var perfis = map[string]Perfil{
	"codex":          {Nome: "codex", Harness: "codex", Mode: "cli", Model: "gpt-5-codex", Effort: "medium"},
	"codex2":         {Nome: "codex2", Harness: "codex2", Mode: "cli", Model: "gpt-5-codex", Effort: "medium", Env: map[string]string{"CODEX_HOME": "~/.codex-compartilhado"}},
	"claude":         {Nome: "claude", Harness: "claude", Mode: "cli", Model: "claude-sonnet-5-5"},
	"claude-conta2":  {Nome: "claude-conta2", Harness: "claude", Mode: "cli", Model: "claude-sonnet-5-5", Env: map[string]string{"CLAUDE_CONFIG_DIR": "~/.claude-conta2"}},
	"cco-openrouter": {Nome: "cco-openrouter", Harness: "cco", Mode: "cli", Provider: "openrouter", Model: ""},
	"cco-zen":        {Nome: "cco-zen", Harness: "cco", Mode: "cli", Provider: "opencode-zen", Model: ""},
	"opencode":       {Nome: "opencode", Harness: "opencode", Mode: "cli", Model: "opencode/big-pickle"},
}

func perfisPadrao() []Perfil {
	result := make([]Perfil, 0, len(perfis))
	for _, p := range perfis {
		result = append(result, p)
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

// Resolve resolve um perfil embutido, aplicando modelo e esforço opcionais.
func Resolve(nome, modelo, esforco string) (Perfil, error) {
	p, ok := perfis[nome]
	if !ok {
		return Perfil{}, fmt.Errorf("motor '%s' desconhecido; use motores para listar os perfis", nome)
	}
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

func FindRoles(cwd string) (map[string]Perfil, string, error) {
	path := filepath.Join(cwd, ".openheinerss", "motores.yaml")
	roles, err := LoadRoles(path)
	if err != nil {
		if os.IsNotExist(err) {
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
