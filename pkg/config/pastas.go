package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NormalizarPasta expande ~ e variáveis e usa o caminho canônico quando ele existe.
func NormalizarPasta(p string) (string, error) {
	p = strings.TrimSpace(os.ExpandEnv(p))
	if p == "" {
		return "", fmt.Errorf("pasta permitida vazia")
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(h, strings.TrimPrefix(p, "~/"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs), nil
}

func normalizarLista(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range in {
		n, err := NormalizarPasta(p)
		if err != nil {
			return nil, err
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

// PastasPermitidasEfetivas mescla o global, o projeto e a instância. A camada
// posterior só acrescenta caminhos: uma configuração não revoga outra.
func PastasPermitidasEfetivas(repo string, instancia []string) ([]string, error) {
	var all []string
	for _, path := range append(arquivosConfigGlobais(), filepath.Join(repo, WorkspaceDirName, ConfigFileName)) {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var f struct {
			Pastas []string `yaml:"pastas_permitidas"`
		}
		if err := unmarshalYAML(b, &f); err != nil {
			return nil, fmt.Errorf("ler %s: %w", ConfigFileName, err)
		}
		all = append(all, f.Pastas...)
	}
	all = append(all, instancia...)
	return normalizarLista(all)
}

func arquivosConfigGlobais() []string {
	var out []string
	if dir, err := ConfigDir(); err == nil && dir != "" {
		out = append(out, filepath.Join(dir, ConfigFileName))
	}
	if dir, err := UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "openheinerss", ConfigFileName))
	}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(h, WorkspaceDirName, ConfigFileName))
	}
	return out
}

// separado para manter o parser YAML e o arquivo de resolução pequenos.
func unmarshalYAML(b []byte, v interface{}) error { return yaml.Unmarshal(b, v) }
