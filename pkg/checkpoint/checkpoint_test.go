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

func TestCheckpointPulaDependenciasEArquivosGrandes(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "node_modules", "x"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "x", "i.js"), []byte("x"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	big, err := os.Create(filepath.Join(dir, "grande.bin"))
	if err != nil {
		t.Fatal(err)
	}
	_ = big.Truncate(maxArquivo + 1)
	big.Close()
	info, err := GetManager().CreateCheckpoint(dir, "s", "teste")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Parcial {
		t.Error("snapshot deveria ser marcado como parcial")
	}
	root := filepath.Join(dir, ".openheinerss", "checkpoints", info.ID, "files")
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Errorf("a.txt deveria estar no snapshot: %v", err)
	}
	for _, f := range []string{"grande.bin", "node_modules"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			t.Errorf("%s não deveria estar no snapshot", f)
		}
	}
}
