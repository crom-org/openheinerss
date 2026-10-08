package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredPort(t *testing.T) {
	t.Setenv("OPENHEINERSS_PORTA", "4931")
	if got := configuredPort(); got != 4931 {
		t.Fatalf("porta env: %d", got)
	}
	t.Setenv("OPENHEINERSS_PORTA", "invalida")
	if got := configuredPort(); got != 4820 {
		t.Fatalf("fallback: %d", got)
	}
}

func TestServeFlagsPorta(t *testing.T) {
	t.Setenv("OPENHEINERSS_PORTA", "4820")
	cmd := newServeCmd()
	if got := cmd.Flag("porta").DefValue; got != "4820" {
		t.Fatalf("default --porta: %s", got)
	}
	if got := cmd.Flag("port").DefValue; got != "4820" {
		t.Fatalf("default --port: %s", got)
	}
}

func TestManualCLIAtualizado(t *testing.T) {
	gerado, err := renderCLIDoc(newRootCmd())
	if err != nil {
		t.Fatal(err)
	}
	manual, err := os.ReadFile(filepath.Join("..", "..", "docs", "09-cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gerado, manual) {
		t.Fatal("docs/09-cli.md está desatualizado; execute 'go run ./cmd/openheinerss docs'")
	}
}
