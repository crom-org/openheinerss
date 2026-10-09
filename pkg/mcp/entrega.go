package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/crom-org/openheinerss/pkg/config"
)

// Entrega dos servidores MCP a cada harness, por execução, sem editar a config pessoal do usuário.
// Ordem de leitura (o último vence): ~/.openheinerss/mcp.json, ~/.config/openheinerss/mcp.json
// e <projeto>/.openheinerss/mcp.json.

// EnvSelecao permite escolher os servidores sem mexer em options (usado pelo `rodar`):
// "nenhum" desliga; "a,b" entrega só esses.
const EnvSelecao = "OPENHEINERSS_MCP"

// Selecao diz quais servidores entregar.
type Selecao struct {
	Desligado bool
	Nomes     []string // vazio = todos
}

var nomeValido = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ArquivosGlobais devolve os mcp.json globais na ordem de leitura.
func ArquivosGlobais() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, config.WorkspaceDirName, config.McpFileName))
	}
	if dir, err := config.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "openheinerss", config.McpFileName))
	}
	return out
}

func lerArquivo(path string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("falha ao ler %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("formato inválido em %s: %w", path, err)
	}
	return cfg.MCPServers, nil
}

// Efetivos junta os servidores globais e os do projeto (o projeto vence no mesmo nome).
func Efetivos(cwd string) (map[string]ServerConfig, error) {
	files := ArquivosGlobais()
	if cwd != "" {
		files = append(files, filepath.Join(cwd, config.WorkspaceDirName, config.McpFileName))
	}
	out := map[string]ServerConfig{}
	visto := map[string]bool{}
	for _, f := range files {
		if abs, err := filepath.Abs(f); err == nil {
			if visto[abs] {
				continue
			}
			visto[abs] = true
		}
		servs, err := lerArquivo(f)
		if err != nil {
			return nil, err
		}
		for k, v := range servs {
			out[k] = v
		}
	}
	return out, nil
}

// SelecaoDoEnv lê OPENHEINERSS_MCP; ok=false quando a variável não existe.
func SelecaoDoEnv() (Selecao, bool) {
	v, ok := os.LookupEnv(EnvSelecao)
	if !ok {
		return Selecao{}, false
	}
	return SelecaoDeTexto(v), true
}

// SelecaoDeTexto interpreta "nenhum"/"off"/"false"/"0" (desliga), "" ou "todos" (todos) e "a,b" (só esses).
func SelecaoDeTexto(v string) Selecao {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "nenhum", "none", "off", "false", "0", "não", "nao":
		return Selecao{Desligado: true}
	case "", "todos", "all":
		return Selecao{}
	}
	var nomes []string
	for _, n := range strings.Split(v, ",") {
		if n = strings.TrimSpace(n); n != "" {
			nomes = append(nomes, n)
		}
	}
	return Selecao{Nomes: nomes}
}

// Selecionar aplica a seleção. Nome pedido que não existe é erro (evita achar que ligou e não ligou).
func Selecionar(servs map[string]ServerConfig, sel Selecao) (map[string]ServerConfig, error) {
	if sel.Desligado {
		return map[string]ServerConfig{}, nil
	}
	if len(sel.Nomes) == 0 {
		return servs, nil
	}
	out := map[string]ServerConfig{}
	for _, n := range sel.Nomes {
		s, ok := servs[n]
		if !ok {
			return nil, fmt.Errorf("servidor MCP '%s' não está em mcp.json (disponíveis: %s)", n, strings.Join(Nomes(servs), ", "))
		}
		out[n] = s
	}
	return out, nil
}

// Nomes devolve os nomes em ordem estável.
func Nomes(servs map[string]ServerConfig) []string {
	out := make([]string, 0, len(servs))
	for k := range servs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// tipoRemoto decide entre "http" e "sse" para servidores por URL.
func tipoRemoto(s ServerConfig) string {
	switch strings.ToLower(s.Type) {
	case "sse":
		return "sse"
	case "http", "streamable-http", "streamable_http":
		return "http"
	}
	if strings.HasSuffix(strings.TrimRight(s.URL, "/"), "/sse") {
		return "sse"
	}
	return "http"
}

// ParaClaude monta o JSON do --mcp-config do claude.
func ParaClaude(servs map[string]ServerConfig) ([]byte, error) {
	m := map[string]interface{}{}
	for name, s := range servs {
		item := map[string]interface{}{}
		if s.URL != "" {
			item["type"] = tipoRemoto(s)
			item["url"] = s.URL
			if len(s.Headers) > 0 {
				item["headers"] = s.Headers
			}
		} else {
			item["type"] = "stdio"
			item["command"] = s.Command
			item["args"] = append([]string{}, s.Args...)
			if len(s.Env) > 0 {
				item["env"] = s.Env
			}
		}
		m[name] = item
	}
	return json.MarshalIndent(map[string]interface{}{"mcpServers": m}, "", "  ")
}

// ParaOpenCode monta o JSON que vai em OPENCODE_CONFIG_CONTENT (mesclado pelo opencode
// por cima da config do usuário).
func ParaOpenCode(servs map[string]ServerConfig) ([]byte, error) {
	m := map[string]interface{}{}
	for name, s := range servs {
		if s.URL != "" {
			item := map[string]interface{}{"type": "remote", "url": s.URL, "enabled": true}
			if len(s.Headers) > 0 {
				item["headers"] = s.Headers
			}
			m[name] = item
			continue
		}
		item := map[string]interface{}{"type": "local", "command": append([]string{s.Command}, s.Args...), "enabled": true}
		if len(s.Env) > 0 {
			item["environment"] = s.Env
		}
		m[name] = item
	}
	return json.Marshal(map[string]interface{}{"$schema": "https://opencode.ai/config.json", "mcp": m})
}

// ParaCodex monta os -c mcp_servers.<nome>.* do codex. Os valores de env e de headers não vão
// na linha de comando (ps/logs): vão no ambiente do processo e o codex os repassa por
// env_vars / env_http_headers. Devolve os argumentos e as variáveis a acrescentar no ambiente.
func ParaCodex(servs map[string]ServerConfig) ([]string, map[string]string, error) {
	var args []string
	env := map[string]string{}
	dono := map[string]string{}
	for _, name := range Nomes(servs) {
		s := servs[name]
		if !nomeValido.MatchString(name) {
			return nil, nil, fmt.Errorf("nome de servidor MCP '%s' inválido para o codex (use letras, números, _ e -)", name)
		}
		p := "mcp_servers." + name + "."
		if s.URL != "" {
			args = append(args, "-c", p+"url="+tomlString(s.URL))
			if len(s.Headers) > 0 {
				var pares []string
				for _, h := range ordenadas(s.Headers) {
					v := "OPENHEINERSS_MCP_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_" + cabecalhoEnv(h)
					env[v] = s.Headers[h]
					pares = append(pares, tomlString(h)+"="+tomlString(v))
				}
				args = append(args, "-c", p+"env_http_headers={"+strings.Join(pares, ",")+"}")
			}
			continue
		}
		args = append(args, "-c", p+"command="+tomlString(s.Command))
		qargs := make([]string, len(s.Args))
		for i, a := range s.Args {
			qargs[i] = tomlString(a)
		}
		args = append(args, "-c", p+"args=["+strings.Join(qargs, ",")+"]")
		if len(s.Env) > 0 {
			keys := ordenadas(s.Env)
			q := make([]string, len(keys))
			for i, k := range keys {
				if outro, ok := dono[k]; ok && env[k] != s.Env[k] {
					return nil, nil, fmt.Errorf("servidores MCP '%s' e '%s' definem %s com valores diferentes; o codex repassa variáveis do ambiente e não dá para entregar as duas", outro, name, k)
				}
				dono[k], env[k] = name, s.Env[k]
				q[i] = tomlString(k)
			}
			args = append(args, "-c", p+"env_vars=["+strings.Join(q, ",")+"]")
		}
	}
	return args, env, nil
}

func ordenadas(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cabecalhoEnv(h string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 32
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, h)
}

// tomlString escreve uma string básica TOML.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Mascarar troca valores de env/headers por "***" (para mostrar a config sem vazar segredo).
func Mascarar(s ServerConfig) ServerConfig {
	out := s
	if len(s.Env) > 0 {
		out.Env = map[string]string{}
		for k := range s.Env {
			out.Env[k] = "***"
		}
	}
	if len(s.Headers) > 0 {
		out.Headers = map[string]string{}
		for k := range s.Headers {
			out.Headers[k] = "***"
		}
	}
	return out
}
