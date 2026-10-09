package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func binFalso(t *testing.T, path, versao string) {
	t.Helper()
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'openheinerss " + versao + " (commit x, data y)'; fi\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func pular(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("depende de /proc e /bin/sh")
	}
}

func procFalso(t *testing.T, root string, pid int, exe string, env []string, cwd string, argv ...string) {
	t.Helper()
	d := filepath.Join(root, fmt.Sprint(pid))
	os.MkdirAll(d, 0o755)
	os.Symlink(exe, filepath.Join(d, "exe"))
	os.Symlink(cwd, filepath.Join(d, "cwd"))
	os.WriteFile(filepath.Join(d, "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0o644)
	os.WriteFile(filepath.Join(d, "environ"), []byte(strings.Join(env, "\x00")+"\x00"), 0o644)
}

type fakes struct {
	sinais  []int
	iniciou []processo
}

func ambFalso(t *testing.T, root string, f *fakes) ambienteAtualizar {
	vivos := map[int]bool{}
	return ambienteAtualizar{
		ProcRoot:  root,
		Sinalizar: func(pid int) error { f.sinais = append(f.sinais, pid); vivos[pid] = false; return nil },
		Viva:      func(pid int) bool { return vivos[pid] },
		Iniciar:   func(p processo, d string) (int, error) { f.iniciou = append(f.iniciou, p); return p.PID + 1000, nil },
		Sonda:     func(string) bool { return true },
		Espera:    time.Second,
		Compilar:  func(repo, saida string) error { binFalso(t, saida, "2.0.0"); return nil },
		HTTP:      http.DefaultClient,
		GOOS:      "linux",
		GOARCH:    "amd64",
	}
}

func TestInstalarEVoltarAtomico(t *testing.T) {
	pular(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "openheinerss")
	binFalso(t, dest, "1.0.0")
	novo := filepath.Join(dir, "novo")
	binFalso(t, novo, "2.0.0")
	if err := instalarAtomico(dest, novo, dest+".anterior"); err != nil {
		t.Fatal(err)
	}
	if v := versaoDoBinario(dest); !strings.HasPrefix(v, "2.0.0") {
		t.Fatalf("destino: %q", v)
	}
	if v := versaoDoBinario(dest + ".anterior"); !strings.HasPrefix(v, "1.0.0") {
		t.Fatalf("anterior: %q", v)
	}
	if err := trocarComAnterior(dest, dest+".anterior"); err != nil {
		t.Fatal(err)
	}
	if v := versaoDoBinario(dest); !strings.HasPrefix(v, "1.0.0") {
		t.Fatalf("depois de voltar: %q", v)
	}
	if v := versaoDoBinario(dest + ".anterior"); !strings.HasPrefix(v, "2.0.0") {
		t.Fatalf("anterior após voltar: %q", v)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".openheinerss-novo") {
			t.Fatalf("temporário sobrou: %s", e.Name())
		}
	}
}

func TestPortaEClassificacao(t *testing.T) {
	cases := []struct {
		argv  []string
		env   []string
		porta int
		tipo  string
	}{
		{[]string{"/x/openheinerss", "serve"}, nil, 4820, "serve"},
		{[]string{"/x/openheinerss", "serve", "--porta", "5001"}, nil, 5001, "serve"},
		{[]string{"x", "servir", "-p=5002"}, nil, 5002, "serve"},
		{[]string{"x", "--projeto", "serve", "rodar", "a"}, nil, 4820, "rodar"},
		{[]string{"x", "serve"}, []string{"OPENHEINERSS_PORTA=5003"}, 5003, "serve"},
		{[]string{"x", "doctor"}, nil, 4820, ""},
	}
	for _, c := range cases {
		p := processo{Argv: c.argv, Env: c.env}
		if classificar(p) != c.tipo || (c.tipo == "serve" && portaDoServe(p) != c.porta) {
			t.Errorf("%v: tipo=%q porta=%d", c.argv, classificar(p), portaDoServe(p))
		}
	}
}

func TestAtualizarDeFonteReiniciaSoServeDoDestino(t *testing.T) {
	pular(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin", "openheinerss")
	os.MkdirAll(filepath.Dir(dest), 0o755)
	binFalso(t, dest, "1.0.0")
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "cmd", "openheinerss"), 0o755)
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module github.com/crom-org/openheinerss\n"), 0o644)

	root := t.TempDir()
	cwd := t.TempDir()
	procFalso(t, root, 101, dest, []string{"A=1", "OPENHEINERSS_PORTA=5555"}, cwd, dest, "serve", "--porta", "5555")
	procFalso(t, root, 102, dest, nil, cwd, dest, "rodar", "agente")
	procFalso(t, root, 103, "/outro/openheinerss", nil, cwd, "/outro/openheinerss", "serve")
	procFalso(t, root, 104, dest+" (deleted)", nil, cwd, dest, "serve", "--stdio")

	f := &fakes{}
	env := ambFalso(t, root, f)
	rel, err := executarAtualizar(opcoesAtualizar{DeFonte: true, Destino: dest, Repo: repo}, env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel.Antes, "1.0.0") || !strings.HasPrefix(rel.Depois, "2.0.0") {
		t.Fatalf("antes/depois: %q → %q", rel.Antes, rel.Depois)
	}
	if len(f.sinais) != 1 || f.sinais[0] != 101 {
		t.Fatalf("sinais: %v (só o serve 101 pode ser encerrado)", f.sinais)
	}
	if len(f.iniciou) != 1 || f.iniciou[0].Cwd != cwd || strings.Join(f.iniciou[0].Env, ",") != "A=1,OPENHEINERSS_PORTA=5555" {
		t.Fatalf("relançamento: %+v", f.iniciou)
	}
	if len(rel.RodarAntigos) != 1 || rel.RodarAntigos[0].PID != 102 {
		t.Fatalf("rodar: %+v", rel.RodarAntigos)
	}
	estados := map[int]string{}
	for _, s := range rel.Serves {
		estados[s.PID] = s.Estado
	}
	if estados[101] != "reiniciado" || estados[104] != "mantido" || len(estados) != 2 {
		t.Fatalf("estados: %v", estados)
	}
	if !strings.HasPrefix(versaoDoBinario(dest+".anterior"), "1.0.0") {
		t.Fatal("anterior não guardado")
	}

	// --voltar
	f2 := &fakes{}
	rel, err = executarAtualizar(opcoesAtualizar{Voltar: true, Destino: dest}, ambFalso(t, root, f2))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(versaoDoBinario(dest), "1.0.0") || len(f2.iniciou) != 1 {
		t.Fatalf("voltar: %q %+v", versaoDoBinario(dest), f2.iniciou)
	}
}

func TestAtualizarSecoESemReiniciar(t *testing.T) {
	pular(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "openheinerss")
	binFalso(t, dest, "1.0.0")
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "cmd", "openheinerss"), 0o755)
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module github.com/crom-org/openheinerss\n"), 0o644)
	root := t.TempDir()
	procFalso(t, root, 201, dest, nil, dir, dest, "serve")

	f := &fakes{}
	env := ambFalso(t, root, f)
	rel, err := executarAtualizar(opcoesAtualizar{Seco: true, DeFonte: true, Destino: dest, Repo: repo}, env)
	if err != nil || len(f.sinais) != 0 || existe(dest+".anterior") || !strings.HasPrefix(versaoDoBinario(dest), "1.0.0") {
		t.Fatalf("seco alterou algo: %v %v", err, f)
	}
	if len(rel.Serves) != 1 || rel.Serves[0].Estado != "reiniciar" {
		t.Fatalf("plano: %+v", rel.Serves)
	}
	rel, err = executarAtualizar(opcoesAtualizar{SemReiniciar: true, DeFonte: true, Destino: dest, Repo: repo}, env)
	if err != nil || len(f.sinais) != 0 || rel.Serves[0].Estado != "mantido" || !strings.HasPrefix(versaoDoBinario(dest), "2.0.0") {
		t.Fatalf("sem-reiniciar: %v %+v", err, rel)
	}
}

func TestAtualizarVoltarSemAnterior(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "openheinerss")
	_, err := executarAtualizar(opcoesAtualizar{Voltar: true, Destino: dest}, ambienteAtualizar{ProcRoot: t.TempDir()})
	if err == nil {
		t.Fatal("esperava erro")
	}
}

func pacoteTar(t *testing.T, conteudo string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "openheinerss", Mode: 0o755, Size: int64(len(conteudo)), Typeflag: tar.TypeReg})
	tw.Write([]byte(conteudo))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestAtualizarRelease(t *testing.T) {
	pular(t)
	pkg := pacoteTar(t, "#!/bin/sh\necho 'openheinerss 9.9.9 (commit a, data b)'\n")
	soma := sha256.Sum256(pkg)
	nome := "openheinerss_9.9.9_linux_amd64.tar.gz"
	boa := hex.EncodeToString(soma[:])
	var sumTxt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"):
			fmt.Fprint(w, `{"tag_name":"v9.9.9"}`)
		case strings.HasSuffix(r.URL.Path, nome):
			w.Write(pkg)
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			fmt.Fprint(w, sumTxt)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "openheinerss")
	binFalso(t, dest, "1.0.0")
	env := ambFalso(t, t.TempDir(), &fakes{})
	env.APIURL, env.BaseURL = srv.URL+"/latest", srv.URL

	sumTxt = strings.Repeat("0", 64) + "  " + nome + "\n"
	if _, err := executarAtualizar(opcoesAtualizar{Release: true, Destino: dest}, env); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("checksum errado deveria falhar: %v", err)
	}
	if !strings.HasPrefix(versaoDoBinario(dest), "1.0.0") {
		t.Fatal("destino mudou apesar do checksum inválido")
	}
	sumTxt = boa + "  " + nome + "\n"
	rel, err := executarAtualizar(opcoesAtualizar{Release: true, Destino: dest}, env)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Disponivel != "v9.9.9" || !strings.HasPrefix(rel.Depois, "9.9.9") {
		t.Fatalf("%+v", rel)
	}
}
