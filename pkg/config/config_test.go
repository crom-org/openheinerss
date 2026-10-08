package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectLerEventosLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, WorkspaceDirName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, WorkspaceDirName, ConfigFileName), []byte("eventos_log: /tmp/eventos.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProject(dir)
	if err != nil || cfg.EventosLog != "/tmp/eventos.log" {
		t.Fatalf("configuração: %+v (%v)", cfg, err)
	}
}

func TestInitWorkspaceCriaEstruturaEEhIdempotente(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := InitWorkspace(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{ConfigFileName, McpFileName, SessionsDirName, CheckpointsDirName} {
		if _, err := os.Stat(filepath.Join(dir, WorkspaceDirName, p)); err != nil {
			t.Errorf("faltou %s: %v", p, err)
		}
	}
}
