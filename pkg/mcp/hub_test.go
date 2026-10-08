package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHubRegistraELista(t *testing.T) {
	dir := t.TempDir()
	h := GetHub()
	servers, err := h.ListServers(dir)
	if err != nil || len(servers) != 0 {
		t.Fatalf("sem mcp.json esperava lista vazia: %v %v", servers, err)
	}
	if err := h.RegisterServer(dir, "fs", ServerConfig{Command: "npx", Args: []string{"-y", "servidor-fs"}}); err != nil {
		t.Fatal(err)
	}
	servers, err = h.ListServers(dir)
	if err != nil || len(servers) != 1 || servers[0].Name != "fs" || servers[0].Command != "npx" {
		t.Fatalf("servidores = %+v, err = %v", servers, err)
	}
}

func TestHubArquivoCorrompidoNaoQuebra(t *testing.T) {
	dir := t.TempDir()
	h := GetHub()
	if err := h.RegisterServer(dir, "a", ServerConfig{Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".openheinerss", "mcp.json"), []byte("{ não é json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterServer(dir, "b", ServerConfig{Command: "y"}); err == nil {
		t.Fatal("esperava erro com mcp.json corrompido (e não pânico)")
	}
}
