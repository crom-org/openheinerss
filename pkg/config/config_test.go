package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
