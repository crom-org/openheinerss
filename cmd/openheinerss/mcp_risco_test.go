package main

import (
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestCatalogoDizComoMCPChega(t *testing.T) {
	esperado := map[string]string{"claude-code": "--mcp-config", "codex": "mcp_servers", "opencode": "OPENCODE_CONFIG_CONTENT", "aider": "sem suporte", "agy": "sem equivalente"}
	vistos := 0
	for _, item := range harness.ListCatalog() {
		if want, ok := esperado[item.ID]; ok {
			vistos++
			if !strings.Contains(item.MCP, want) {
				t.Errorf("%s: mcp=%q, esperava %q", item.ID, item.MCP, want)
			}
		}
	}
	if vistos < len(esperado) {
		t.Fatalf("só %d dos %d harnesses no catálogo", vistos, len(esperado))
	}
}

func TestFlagsMCPERisco(t *testing.T) {
	for _, c := range []struct {
		cmd   string
		flags []string
	}{
		{"run", []string{"sem-mcp", "mcp", "classificar-risco"}},
		{"serve", []string{"classificar-risco"}},
		{"rodar", []string{"sem-mcp", "mcp"}},
	} {
		sub, _, err := newRootCmd().Find([]string{c.cmd})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range c.flags {
			if sub.Flags().Lookup(f) == nil {
				t.Errorf("%s sem --%s", c.cmd, f)
			}
		}
	}
}
