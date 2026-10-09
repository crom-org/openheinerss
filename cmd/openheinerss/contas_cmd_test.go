package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crom-org/openheinerss/pkg/config"
)

func TestContasAdicionarExecutaLoginNativoSemCredencial(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	cfg := t.TempDir()
	marcador := filepath.Join(t.TempDir(), "login.txt")
	login := filepath.Join(bin, "codex")
	if err := os.WriteFile(login, []byte("#!/bin/sh\nprintf login > \"$LOGIN_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LOGIN_MARKER", marcador)
	config.SetConfigDir(cfg)
	t.Cleanup(func() { config.SetConfigDir("") })
	root := newRootCmd()
	root.SetArgs([]string{"contas", "adicionar", "codex", "teste", "--config", cfg})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(marcador); err != nil || string(got) != "login" {
		t.Fatalf("login nativo não executado: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex-teste", "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("o agente não deveria criar credencial: %v", err)
	}
}
