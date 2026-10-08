package checkpoint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointCopiaERestauraSemGit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arquivo.txt")
	if err := os.WriteFile(path, []byte("antes"), 0644); err != nil {
		t.Fatal(err)
	}
	cp, err := GetManager().CreateCheckpoint(dir, "sess_1", "teste")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(filepath.Join(dir, ".openheinerss", "checkpoints", cp.ID, "files", "arquivo.txt"))
	if err != nil || string(snapshot) != "antes" {
		t.Fatalf("snapshot: %q, err=%v", snapshot, err)
	}
	if err := os.WriteFile(path, []byte("depois"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := GetManager().RestoreCheckpoint(dir, cp.ID); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "antes" {
		t.Fatalf("conteúdo restaurado: %q", b)
	}
}
