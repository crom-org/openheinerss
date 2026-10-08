package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// mcpProjeto isola HOME/XDG e grava um mcp.json no projeto.
func mcpProjeto(t *testing.T, conteudo string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENHEINERSS_MCP", "todos")
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".openheinerss")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	return cwd
}

// fakeClaudeMCP grava o argv e copia o arquivo de --mcp-config (que some no Stop).
func fakeClaudeMCP(t *testing.T) (argv, copia string) {
	t.Helper()
	dir := t.TempDir()
	argv, copia = filepath.Join(dir, "argv.txt"), filepath.Join(dir, "mcp.txt")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argv + "\"; done\n" +
		"prev=; for a in \"$@\"; do if [ \"$prev\" = --mcp-config ]; then cat \"$a\" >> \"" + copia + "\"; fi; prev=$a; done\n" +
		"echo '{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"s1\"}'\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argv, copia
}

func rodarMCP(t *testing.T, cwd string, opts map[string]interface{}) (*ClaudeCodeHarness, []string) {
	t.Helper()
	argv, _ := fakeClaudeMCP(t)
	h := NewClaudeCodeHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), harness.SessionConfig{SessionID: "s", CWD: cwd, Options: opts}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop() })
	if err := h.SendPrompt(context.Background(), "oi", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h, harness.EventComplete)
	return h, lerArgv(t, argv)
}

func TestMCPEntregueAoClaudePorExecucao(t *testing.T) {
	cwd := mcpProjeto(t, `{"mcpServers":{"fs":{"command":"npx","args":["srv"],"env":{"TOKEN":"segredo-xyz"}},"web":{"url":"https://h/mcp"}}}`)
	argv, copia := fakeClaudeMCP(t)
	h := NewClaudeCodeHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), harness.SessionConfig{SessionID: "s", CWD: cwd, Options: map[string]interface{}{"mcp_config": "meu.json"}}); err != nil {
		t.Fatal(err)
	}
	if err := h.SendPrompt(context.Background(), "oi", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h, harness.EventComplete)
	args := lerArgv(t, argv)
	linha := strings.Join(args, " ")
	if strings.Contains(linha, "segredo") {
		t.Fatalf("segredo no argv: %s", linha)
	}
	// O --mcp-config do usuário continua e o temporário vem depois.
	var tmp string
	for i, a := range args {
		if a == "--mcp-config" && args[i+1] != "meu.json" {
			tmp = args[i+1]
		}
	}
	if !strings.Contains(linha, "--mcp-config meu.json") || tmp == "" {
		t.Fatalf("argv sem os dois --mcp-config: %s", linha)
	}
	data, _ := os.ReadFile(copia)
	if !strings.Contains(string(data), `"TOKEN": "segredo-xyz"`) || !strings.Contains(string(data), `"type": "http"`) {
		t.Fatalf("arquivo entregue: %s", data)
	}
	if st, err := os.Stat(tmp); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("arquivo temporário deveria existir com 0600: %v %v", st, err)
	}
	_ = h.Stop()
	// A remoção é assíncrona (no fim do contexto da sessão).
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(tmp); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("arquivo temporário não foi apagado no Stop: %s", tmp)
}

func TestMCPSemMCPEEscolha(t *testing.T) {
	cwd := mcpProjeto(t, `{"mcpServers":{"a":{"command":"a"},"b":{"command":"b"}}}`)
	_, args := rodarMCP(t, cwd, map[string]interface{}{harness.OptionSemMCP: true})
	if strings.Contains(strings.Join(args, " "), "--mcp-config") {
		t.Fatalf("--sem-mcp não desligou: %v", args)
	}
	_, args = rodarMCP(t, cwd, map[string]interface{}{harness.OptionMCP: []interface{}{"b"}})
	if n := strings.Count(strings.Join(args, " "), "--mcp-config"); n != 1 {
		t.Fatalf("esperava 1 --mcp-config: %v", args)
	}
	h := NewClaudeCodeHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), harness.SessionConfig{CWD: cwd, Options: map[string]interface{}{harness.OptionMCP: "x"}}); err == nil || !strings.Contains(err.Error(), "'x'") {
		t.Fatalf("servidor inexistente deveria falhar no Start: %v", err)
	}
}
