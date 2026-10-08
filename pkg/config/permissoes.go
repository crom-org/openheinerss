package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Avisar recebe os avisos de permissão que não deu para ajustar (padrão: stderr).
var Avisar = func(msg string) { fmt.Fprintln(os.Stderr, "AVISO: "+msg) }

var endurecidas sync.Map // pastas já conferidas neste processo

var chmod = os.Chmod // trocado nos testes

// PastaPrivada garante que dir (que guarda segredo, prompt ou anotação) exista com 0700 e que os
// arquivos comuns logo dentro dela fiquem com 0600. Pasta ou arquivo que já existiam mais abertos são
// fechados; o que não der para ajustar vira aviso (Avisar), não erro. Só falha se não conseguir criar
// a pasta. Cada pasta é conferida uma vez por processo. No Windows não faz nada além de criar.
func PastaPrivada(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	if _, feito := endurecidas.LoadOrStore(abs, true); feito {
		return nil
	}
	fechar(abs, 0o700)
	entries, err := os.ReadDir(abs)
	if err != nil {
		Avisar(fmt.Sprintf("não consegui ler %s para ajustar as permissões: %v", abs, err))
		return nil
	}
	for _, e := range entries {
		if e.Type().IsRegular() {
			fechar(filepath.Join(abs, e.Name()), 0o600)
		}
	}
	return nil
}

// ArquivoPrivado deixa um arquivo existente com 0600 se estiver mais aberto (aviso se falhar).
func ArquivoPrivado(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	fechar(path, 0o600)
}

// fechar tira as permissões de grupo/outros de path (sem seguir links simbólicos).
func fechar(path string, modo os.FileMode) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if info.Mode().Perm()&0o077 == 0 {
		return
	}
	if err := chmod(path, modo); err != nil {
		Avisar(fmt.Sprintf("não consegui ajustar %s para %#o (está %#o): %v", path, modo, info.Mode().Perm(), err))
	}
}
