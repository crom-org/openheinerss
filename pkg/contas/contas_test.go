package contas

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

func TestCicloContaSemLerCredencial(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(t.TempDir(), "harnesses")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	res, err := Adicionar(dir, "codex", "Conta 2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Instancia != "codex-conta-2" || res.ContaDir != filepath.Join(home, ".codex-conta-2") {
		t.Fatalf("conta inesperada: %+v", res)
	}
	items, err := Listar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TemLogin {
		t.Fatalf("listagem inesperada: %+v", items)
	}
	if err := os.WriteFile(filepath.Join(res.ContaDir, "auth.json"), []byte("segredo que o teste não lê"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err = Listar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].TemLogin {
		t.Fatal("deveria detectar somente a existência da credencial")
	}
	ren, err := Renomear(dir, "codex-conta-2", "segunda")
	if err != nil {
		t.Fatal(err)
	}
	if ren.ContaID != res.ContaID || ren.ContaDir != res.ContaDir {
		t.Fatalf("renomeação moveu/alterou a conta: %+v", ren)
	}
	if _, err := Remover(dir, "segunda"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.ContaDir); err != nil {
		t.Fatalf("remoção deveria preservar pasta: %v", err)
	}
}

func TestCamadasProjetoVenceEMigraSemApagar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	global, projeto, destino := filepath.Join(t.TempDir(), "global"), filepath.Join(t.TempDir(), "projeto"), filepath.Join(t.TempDir(), "novo-global")
	for _, dir := range []string{global, projeto} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Adicionar(global, "codex", "um"); err != nil {
		t.Fatal(err)
	}
	if _, err := Adicionar(projeto, "codex", "dois"); err != nil {
		t.Fatal(err)
	}
	items, err := ListarCamadas([]string{global}, projeto)
	if err != nil || len(items) != 2 {
		t.Fatalf("camadas: %+v (%v)", items, err)
	}
	for _, item := range items {
		if item.Origem == "" {
			t.Fatalf("origem ausente: %+v", item)
		}
	}
	n, err := Migrar(projeto, destino)
	if err != nil || n != 1 {
		t.Fatalf("migração: %d (%v)", n, err)
	}
	if _, err := os.Stat(filepath.Join(projeto, "codex-dois.yaml")); err != nil {
		t.Fatalf("projeto foi apagado: %v", err)
	}
}
