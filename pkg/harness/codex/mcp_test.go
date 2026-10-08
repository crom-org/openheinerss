package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestMCPEntregueAoCodexSemSegredoNoArgv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENHEINERSS_MCP", "todos")
	cwd := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cwd, ".openheinerss"), 0o755)
	_ = os.WriteFile(filepath.Join(cwd, ".openheinerss", "mcp.json"), []byte(`{"mcpServers":{"fs":{"command":"npx","args":["srv"],"env":{"TOKEN_FS":"segredo-abc"}}}}`), 0o600)

	dir := t.TempDir()
	argvFile, envFile := filepath.Join(dir, "argv.txt"), filepath.Join(dir, "env.txt")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"printf '%s' \"$TOKEN_FS\" > \"" + envFile + "\"\n" +
		"echo '{\"type\":\"thread.started\",\"thread_id\":\"th-1\"}'\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	h := NewCodexHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), harness.SessionConfig{SessionID: "s", CWD: cwd, Options: map[string]interface{}{"config": "x=1"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop() })
	if err := h.SendPrompt(context.Background(), "oi", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h)
	data, _ := os.ReadFile(argvFile)
	linha := strings.ReplaceAll(string(data), "\n", " ")
	if strings.Contains(linha, "segredo") {
		t.Fatalf("segredo no argv: %s", linha)
	}
	for _, want := range []string{`-c mcp_servers.fs.command="npx"`, `-c mcp_servers.fs.args=["srv"]`, `-c mcp_servers.fs.env_vars=["TOKEN_FS"]`, "-c x=1"} {
		if !strings.Contains(linha, want) {
			t.Fatalf("faltou %q em %s", want, linha)
		}
	}
	if env, _ := os.ReadFile(envFile); string(env) != "segredo-abc" {
		t.Fatalf("o valor deveria chegar pelo ambiente do codex, veio %q", env)
	}
}

func TestMCPCodexDesligadoPeloAmbiente(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENHEINERSS_MCP", "nenhum")
	cwd := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cwd, ".openheinerss"), 0o755)
	_ = os.WriteFile(filepath.Join(cwd, ".openheinerss", "mcp.json"), []byte(`{"mcpServers":{"fs":{"command":"npx"}}}`), 0o600)
	h := NewCodexHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), harness.SessionConfig{CWD: cwd}); err != nil {
		t.Fatal(err)
	}
	if args := strings.Join(buildExecArgs(h.cfg, "", "oi"), " "); strings.Contains(args, "mcp_servers") {
		t.Fatalf("OPENHEINERSS_MCP=nenhum não desligou: %s", args)
	}
}
