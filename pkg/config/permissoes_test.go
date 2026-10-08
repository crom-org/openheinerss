package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPastaPrivadaFechaPastaEArquivosExistentes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões Unix")
	}
	dir := filepath.Join(t.TempDir(), "segredos")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	arq := filepath.Join(dir, "comandos.yaml")
	if err := os.WriteFile(arq, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	var avisos []string
	old := Avisar
	Avisar = func(m string) { avisos = append(avisos, m) }
	defer func() { Avisar = old }()
	if err := PastaPrivada(dir); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, arq: 0o600, filepath.Join(dir, "sub"): 0o755} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: %v (quer %#o)", p, info.Mode().Perm(), want)
		}
	}
	if len(avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", avisos)
	}
	nova := filepath.Join(t.TempDir(), "a", "b")
	if err := PastaPrivada(nova); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(nova); info.Mode().Perm() != 0o700 {
		t.Fatalf("pasta nova: %#o", info.Mode().Perm())
	}
}

func TestPermissaoQueNaoMudaViraAviso(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões Unix")
	}
	dir := filepath.Join(t.TempDir(), "alheia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var avisos []string
	oldAvisar, oldChmod := Avisar, chmod
	Avisar = func(m string) { avisos = append(avisos, m) }
	chmod = func(string, os.FileMode) error { return os.ErrPermission } // pasta de outro dono
	defer func() { Avisar, chmod = oldAvisar, oldChmod }()
	if err := PastaPrivada(dir); err != nil {
		t.Fatalf("falha de chmod não pode virar erro: %v", err)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "não consegui ajustar "+dir) || !strings.Contains(avisos[0], "0700") {
		t.Fatalf("avisos: %v", avisos)
	}
}

func TestArquivoPrivado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões Unix")
	}
	p := filepath.Join(t.TempDir(), "chaves.env")
	if err := os.WriteFile(p, []byte("A=1"), 0o664); err != nil {
		t.Fatal(err)
	}
	ArquivoPrivado(p)
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
		t.Fatalf("%#o", info.Mode().Perm())
	}
}
