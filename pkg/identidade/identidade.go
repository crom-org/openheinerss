package identidade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func Para(nome string, env map[string]string) (protocol.Identity, error) {
	nome = harness.CanonicalName(nome)
	base, configurado, fonte, err := configuracao(nome, map[string]bool{})
	if err != nil {
		return protocol.Identity{}, err
	}
	efetivo := map[string]string{}
	for k, v := range env {
		efetivo[k] = v
	}
	for k, v := range configurado {
		efetivo[k] = v
	}
	key := chaveLogin(base)
	dir := efetivo[key]
	if strings.EqualFold(base, "claude-code") && strings.EqualFold(nome, "claude-code") {
		// A instância canônica não pode ser redirecionada por env do pedido:
		// o filho também ignora esse valor e usa ~/.claude.
		dir = ""
	}
	if dir == "" {
		// claude-code nunca herda CLAUDE_CONFIG_DIR do shell: o adaptador CLI
		// também fixa ~/.claude para a instância principal.
		if strings.EqualFold(base, "claude-code") {
			dir = padraoLogin(base)
		} else if valor, ok := os.LookupEnv(key); ok && valor != "" {
			dir = valor
		} else {
			dir = padraoLogin(base)
		}
	}
	dir, err = canonico(dir)
	if err != nil {
		return protocol.Identity{}, err
	}
	sum := sha256.Sum256([]byte(dir))
	return protocol.Identity{Instancia: nome, Base: base, ContaID: hex.EncodeToString(sum[:])[:16], ConfigFonte: fonte, ContaDir: dir}, nil
}

func configuracao(nome string, seen map[string]bool) (string, map[string]string, string, error) {
	k := strings.ToLower(nome)
	if seen[k] {
		return "", nil, "", fmt.Errorf("herança circular envolvendo %q", nome)
	}
	seen[k] = true
	if s, ok := harness.CustomSpecFor(nome); ok {
		base, env, fonte := s.Name, map[string]string{}, s.Source
		if s.Base != "" {
			var err error
			base, env, _, err = configuracao(s.Base, seen)
			if err != nil {
				return "", nil, "", err
			}
		}
		for k, v := range s.Env {
			env[k] = v
		}
		return base, env, fonte, nil
	}
	if !harness.Exists(nome) {
		return "", nil, "", fmt.Errorf("instância %q não encontrada", nome)
	}
	return nome, map[string]string{}, "", nil
}

func chaveLogin(base string) string {
	switch strings.ToLower(base) {
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
func padraoLogin(base string) string {
	home, _ := os.UserHomeDir()
	switch strings.ToLower(base) {
	case "claude-code":
		return filepath.Join(home, ".claude")
	case "codex":
		return filepath.Join(home, ".codex")
	case "opencode":
		return filepath.Join(home, ".config", "opencode")
	default:
		return filepath.Join(home, ".openheinerss", "accounts", strings.ToLower(base))
	}
}
func canonico(dir string) (string, error) {
	if strings.HasPrefix(dir, "~/") {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(h, dir[2:])
	}
	a, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", err
	}
	if r, e := filepath.EvalSymlinks(a); e == nil {
		a = filepath.Clean(r)
	} else if !os.IsNotExist(e) {
		return "", e
	}
	return a, nil
}
