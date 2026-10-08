package comandos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/crom-org/openheinerss/pkg/config"
)

// Filho do teste entre processos: anota N comandos com prefixo P na --config indicada.
func TestMain(m *testing.M) {
	if dir := os.Getenv("OH_TESTE_ANOTAR_DIR"); dir != "" {
		config.SetConfigDir(dir)
		n, _ := strconv.Atoi(os.Getenv("OH_TESTE_ANOTAR_N"))
		nota := strings.Repeat("x", 10*1024)
		for i := 0; i < n; i++ {
			if _, err := Anotar("codex", fmt.Sprintf("/%s%d", os.Getenv("OH_TESTE_ANOTAR_P"), i), nota, ""); err != nil {
				fmt.Fprintln(os.Stderr, "ERRO:", err)
				os.Exit(2)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Reproduz a auditoria 26: dois processos na mesma --config, 25 anotações de 10 KB cada (a auditoria usou 100 KB).
// Antes da trava entre processos sobravam 23 de 50 (e houve erro de rename do .tmp fixo).
func TestAnotarEntreProcessosNaoPerde(t *testing.T) {
	tmp := ambiente(t)
	cfg := filepath.Join(tmp, "cfg")
	if err := os.Mkdir(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	const porProcesso = 25
	prefixos := []string{"aa", "bb"}
	var wg sync.WaitGroup
	erros := make([]error, len(prefixos))
	saidas := make([][]byte, len(prefixos))
	for i, p := range prefixos {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "OH_TESTE_ANOTAR_DIR="+cfg, "OH_TESTE_ANOTAR_P="+p, "OH_TESTE_ANOTAR_N="+strconv.Itoa(porProcesso))
			saidas[i], erros[i] = cmd.CombinedOutput()
		}(i, p)
	}
	wg.Wait()
	for i := range prefixos {
		if erros[i] != nil {
			t.Fatalf("processo %s: %v\n%s", prefixos[i], erros[i], saidas[i])
		}
	}
	config.SetConfigDir(cfg)
	l, err := Listar("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	salvos := 0
	for _, c := range l.Comandos {
		if (strings.HasPrefix(c.Nome, "/aa") || strings.HasPrefix(c.Nome, "/bb")) && len(c.Anotacao) == 10*1024 {
			salvos++
		}
	}
	if salvos != porProcesso*len(prefixos) {
		t.Fatalf("salvos %d de %d", salvos, porProcesso*len(prefixos))
	}
	sobras, _ := filepath.Glob(filepath.Join(cfg, "*.tmp"))
	if len(sobras) > 0 {
		t.Fatalf("temporários esquecidos: %v", sobras)
	}
	t.Logf("salvos %d de %d", salvos, porProcesso*len(prefixos))
}

func TestGravacaoComFalhaDevolveErro(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissão de pasta não impede root/Windows")
	}
	tmp := ambiente(t)
	cfg := filepath.Join(tmp, "cfg")
	if err := os.Mkdir(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	config.SetConfigDir(cfg)
	if _, err := Anotar("codex", "/a", "primeira", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cfg, 0o755)
	if _, err := Anotar("codex", "/b", "segunda", ""); err == nil {
		t.Fatal("gravação sem permissão devolveu sucesso")
	}
}

func TestAnotacaoMascaraSegredoEArquivo0600(t *testing.T) {
	tmp := ambiente(t)
	cfg := filepath.Join(tmp, "nova", "cfg")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	config.SetConfigDir(cfg)
	c, err := Anotar("claude-code", "/audit", "TOKEN=FAKE_R9_SECRET_123 e api_key: abc123 e Bearer abcdefghijkl e sk-abcdefghijklmnopqrstu; author: Ana", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(cfg, NomeArquivo))
	for _, segredo := range []string{"FAKE_R9_SECRET_123", "abc123", "abcdefghijkl", "sk-abcdefghijklmnopqrstu"} {
		if strings.Contains(c.Anotacao, segredo) || strings.Contains(string(b), segredo) {
			t.Fatalf("segredo %q vazou: %q / %s", segredo, c.Anotacao, b)
		}
	}
	if !strings.Contains(c.Anotacao, "TOKEN=***") || !strings.Contains(c.Anotacao, "author: Ana") {
		t.Fatalf("máscara errada: %q", c.Anotacao)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(cfg, NomeArquivo))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("comandos.yaml com %v, esperado 0600", st.Mode().Perm())
		}
	}
	// arquivo antigo editado à mão: a listagem também mascara
	if err := os.WriteFile(filepath.Join(cfg, NomeArquivo), []byte("codex:\n  \"/x\":\n    anotacao: \"senha=hunter2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Listar("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	if a := achar(t, l, "/x").Anotacao; a != "senha=***" {
		t.Fatalf("listagem não mascarou: %q", a)
	}
}

func TestPastaGlobalCriada0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sem bits de permissão no Windows")
	}
	tmp := ambiente(t)
	if _, err := Anotar("codex", "/a", "x", ""); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(tmp, ".config", "openheinerss"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("pasta criada com %v", st.Mode().Perm())
	}
}
