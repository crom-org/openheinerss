package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gravar(t *testing.T, path, conteudo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEfetivosProjetoVenceGlobal(t *testing.T) {
	home, xdg, proj := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	gravar(t, filepath.Join(home, ".openheinerss", "mcp.json"), `{"mcpServers":{"velho":{"command":"v"},"comum":{"command":"home"}}}`)
	gravar(t, filepath.Join(xdg, "openheinerss", "mcp.json"), `{"mcpServers":{"global":{"command":"g"},"comum":{"command":"global"}}}`)
	gravar(t, filepath.Join(proj, ".openheinerss", "mcp.json"), `{"mcpServers":{"comum":{"command":"projeto"}}}`)
	servs, err := Efetivos(proj)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Nomes(servs), ","); got != "comum,global,velho" {
		t.Fatalf("nomes = %s", got)
	}
	if servs["comum"].Command != "projeto" {
		t.Fatalf("projeto deveria vencer: %+v", servs["comum"])
	}
	gravar(t, filepath.Join(proj, ".openheinerss", "mcp.json"), `{quebrado`)
	if _, err := Efetivos(proj); err == nil || !strings.Contains(err.Error(), "formato inválido") {
		t.Fatalf("esperava erro claro, veio %v", err)
	}
}

func TestSelecao(t *testing.T) {
	servs := map[string]ServerConfig{"a": {Command: "a"}, "b": {Command: "b"}}
	for _, v := range []string{"nenhum", "off", "false", "0"} {
		if !SelecaoDeTexto(v).Desligado {
			t.Fatalf("%s deveria desligar", v)
		}
	}
	got, err := Selecionar(servs, SelecaoDeTexto(" b "))
	if err != nil || len(got) != 1 || got["b"].Command != "b" {
		t.Fatalf("seleção b: %v %v", got, err)
	}
	if got, _ := Selecionar(servs, SelecaoDeTexto("todos")); len(got) != 2 {
		t.Fatalf("todos: %v", got)
	}
	if _, err := Selecionar(servs, SelecaoDeTexto("a,x")); err == nil || !strings.Contains(err.Error(), "'x'") {
		t.Fatalf("nome desconhecido deveria falhar: %v", err)
	}
	t.Setenv(EnvSelecao, "a")
	if sel, ok := SelecaoDoEnv(); !ok || len(sel.Nomes) != 1 {
		t.Fatalf("env: %+v %v", sel, ok)
	}
}

func TestFormatosPorHarness(t *testing.T) {
	servs := map[string]ServerConfig{
		"fs":     {Command: "npx", Args: []string{"-y", "srv \"x\""}, Env: map[string]string{"TOKEN": "segredo-1"}},
		"remoto": {URL: "https://h/mcp", Headers: map[string]string{"Authorization": "Bearer segredo-2"}},
		"antigo": {URL: "https://h/sse"},
	}
	data, err := ParaClaude(servs)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		MCPServers map[string]map[string]interface{} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if c.MCPServers["fs"]["type"] != "stdio" || c.MCPServers["remoto"]["type"] != "http" || c.MCPServers["antigo"]["type"] != "sse" {
		t.Fatalf("claude: %s", data)
	}

	data, err = ParaOpenCode(servs)
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		MCP map[string]map[string]interface{} `json:"mcp"`
	}
	_ = json.Unmarshal(data, &o)
	if o.MCP["fs"]["type"] != "local" || o.MCP["remoto"]["type"] != "remote" || o.MCP["fs"]["environment"] == nil {
		t.Fatalf("opencode: %s", data)
	}
	if cmd, _ := o.MCP["fs"]["command"].([]interface{}); len(cmd) != 3 || cmd[0] != "npx" {
		t.Fatalf("opencode command: %v", o.MCP["fs"]["command"])
	}

	args, env, err := ParaCodex(servs)
	if err != nil {
		t.Fatal(err)
	}
	linha := strings.Join(args, " ")
	if strings.Contains(linha, "segredo") {
		t.Fatalf("segredo na linha de comando: %s", linha)
	}
	for _, want := range []string{
		`mcp_servers.fs.command="npx"`,
		`mcp_servers.fs.args=["-y","srv \"x\""]`,
		`mcp_servers.fs.env_vars=["TOKEN"]`,
		`mcp_servers.remoto.url="https://h/mcp"`,
		`mcp_servers.remoto.env_http_headers={"Authorization"="OPENHEINERSS_MCP_REMOTO_AUTHORIZATION"}`,
	} {
		if !strings.Contains(linha, want) {
			t.Fatalf("faltou %s em %s", want, linha)
		}
	}
	if env["TOKEN"] != "segredo-1" || env["OPENHEINERSS_MCP_REMOTO_AUTHORIZATION"] != "Bearer segredo-2" {
		t.Fatalf("env: %v", env)
	}
	if _, _, err := ParaCodex(map[string]ServerConfig{"a": {Command: "x", Env: map[string]string{"K": "1"}}, "b": {Command: "y", Env: map[string]string{"K": "2"}}}); err == nil {
		t.Fatal("conflito de env deveria falhar")
	}
	if _, _, err := ParaCodex(map[string]ServerConfig{"a.b": {Command: "x"}}); err == nil {
		t.Fatal("nome inválido deveria falhar")
	}
}

func TestMascarar(t *testing.T) {
	m := Mascarar(ServerConfig{Env: map[string]string{"K": "v"}, Headers: map[string]string{"H": "v"}})
	if m.Env["K"] != "***" || m.Headers["H"] != "***" {
		t.Fatalf("%+v", m)
	}
}
