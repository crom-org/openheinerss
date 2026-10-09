package identidade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func Para(nome string, env map[string]string) (protocol.Identity, error) {
	if strings.EqualFold(nome, "codex2") {
		return protocol.Identity{}, fmt.Errorf("codex2 removido: é a mesma conta do codex; use codex")
	}
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
	if dir == "" {
		dir = padraoLogin(base)
	}
	dir, err = canonico(dir)
	if err != nil {
		return protocol.Identity{}, err
	}
	id, idFonte := IDPara(base, dir)
	return protocol.Identity{Instancia: nome, Base: base, ContaID: id, ContaIDFonte: idFonte, ConfigFonte: fonte, ContaDir: dir}, nil
}

// IDPara usa o identificador real da conta quando o CLI o persiste localmente.
// O valor original nunca é retornado; somente seu hash curto sai no contrato.
func IDPara(base, dir string) (string, string) {
	var arquivos []string
	switch strings.ToLower(base) {
	case "codex":
		arquivos = []string{filepath.Join(dir, "auth.json")}
	case "claude-code":
		arquivos = []string{filepath.Join(dir, ".claude.json"), filepath.Join(dir, ".credentials.json")}
	}
	for _, path := range arquivos {
		if id := identificadorArquivo(path); id != "" {
			sum := sha256.Sum256([]byte(id))
			return hex.EncodeToString(sum[:])[:16], "id-real"
		}
	}
	sum := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(sum[:])[:16], "pasta"
}

func identificadorArquivo(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var v interface{}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return procurarIdentificador(v)
}

func procurarIdentificador(v interface{}) string {
	if m, ok := v.(map[string]interface{}); ok {
		for _, k := range []string{"account_id", "accountId", "accountUuid", "email"} {
			if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		for _, x := range m {
			if s := procurarIdentificador(x); s != "" {
				return s
			}
		}
	}
	if a, ok := v.([]interface{}); ok {
		for _, x := range a {
			if s := procurarIdentificador(x); s != "" {
				return s
			}
		}
	}
	return ""
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
