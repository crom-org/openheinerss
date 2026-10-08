package storage_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/crom-org/openheinerss/pkg/storage"
)

func TestTranscriptEmPastaPrivadaMesmoSeJaExistia(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões Unix")
	}
	dir := t.TempDir()
	sessoes := filepath.Join(dir, ".openheinerss", "sessions")
	if err := os.MkdirAll(sessoes, 0o755); err != nil {
		t.Fatal(err)
	}
	velho := filepath.Join(sessoes, "antiga.jsonl")
	if err := os.WriteFile(velho, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := storage.GetStorage().RecordEvent(dir, "nova", "user_prompt", nil, "segredo"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{sessoes: 0o700, velho: 0o600, filepath.Join(sessoes, "nova.jsonl"): 0o600} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: %v %v (quer %#o)", p, info.Mode().Perm(), err, want)
		}
	}
}
