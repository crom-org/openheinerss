package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestMCPEntregueAoOpenCodePorConfigContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENHEINERSS_MCP", "todos")
	os.Unsetenv("OPENCODE_CONFIG_CONTENT")
	cwd := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cwd, ".openheinerss"), 0o755)
	_ = os.WriteFile(filepath.Join(cwd, ".openheinerss", "mcp.json"), []byte(`{"mcpServers":{"fs":{"command":"npx","args":["srv"],"env":{"TOKEN":"segredo"}}}}`), 0o600)

	dir := t.TempDir()
	argvFile, cfgFile := filepath.Join(dir, "argv.txt"), filepath.Join(dir, "cfg.json")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argvFile + "\"\nprintf '%s' \"$OPENCODE_CONFIG_CONTENT\" > \"" + cfgFile + "\"\n" +
		`echo '{"type":"text","sessionID":"ses_1","part":{"text":"oi"}}'` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	o := NewOpenCodeHarness(harness.ModeCLI)
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "s1", CWD: cwd, Model: "p/m"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Stop() })
	rodar(t, o, "oi")
	if argv, _ := os.ReadFile(argvFile); strings.Contains(string(argv), "segredo") {
		t.Fatalf("segredo no argv: %s", argv)
	}
	data, _ := os.ReadFile(cfgFile)
	var c struct {
		MCP map[string]struct {
			Type        string            `json:"type"`
			Command     []string          `json:"command"`
			Environment map[string]string `json:"environment"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("OPENCODE_CONFIG_CONTENT inválido: %q", data)
	}
	if fs := c.MCP["fs"]; fs.Type != "local" || strings.Join(fs.Command, " ") != "npx srv" || fs.Environment["TOKEN"] != "segredo" {
		t.Fatalf("config entregue: %s", data)
	}

	// Quem chama já definiu OPENCODE_CONFIG_CONTENT: a ponte respeita.
	o2 := NewOpenCodeHarness(harness.ModeCLI)
	_ = o2.Start(context.Background(), harness.SessionConfig{CWD: cwd, Env: map[string]string{"OPENCODE_CONFIG_CONTENT": "{}"}})
	for _, kv := range o2.env {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG_CONTENT=") && kv != "OPENCODE_CONFIG_CONTENT={}" {
			t.Fatalf("sobrescreveu a config de quem chamou: %s", kv)
		}
	}
}
