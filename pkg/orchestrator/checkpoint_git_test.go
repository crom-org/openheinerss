package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func escrever(t *testing.T, caminho, conteudo string) {
	t.Helper()
	if err := os.WriteFile(caminho, []byte(conteudo), 0644); err != nil {
		t.Fatal(err)
	}
}

// worktreeFixture cria o repositório temporário e a worktree agente/ag, como o rodar faz.
func worktreeFixture(t *testing.T) (agents, wt string) {
	t.Helper()
	root, agents := repoFixture(t)
	escrever(t, filepath.Join(root, ".gitignore"), "*.log\n.claude/\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "gitignore")
	wt = filepath.Join(agents, "ag")
	git(t, root, "worktree", "add", wt, "-b", "agente/ag", "main")
	return agents, wt
}

func TestMetaAntigoLeSemCamposNovos(t *testing.T) {
	var m meta
	if err := json.Unmarshal([]byte(`{"projeto":"p","motor":"mock","pid":1,"tentativa":1}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.UltimoEventoEm != "" || m.Head != "" || m.InicioPID != "" || m.Checkpoints != nil {
		t.Fatalf("campos novos deviam estar vazios: %+v", m)
	}
	b, _ := json.Marshal(m)
	if strings.Contains(string(b), "ultimo_evento_em") || strings.Contains(string(b), "checkpoints") {
		t.Fatalf("omitempty falhou: %s", b)
	}
}

func TestRunPreencheMetaNovo(t *testing.T) {
	root, agents := repoFixture(t)
	res, err := Run(context.Background(), root, Options{Name: "agente", Motor: "mock", AgentsDir: agents, PromptText: "tarefa", MaxAgents: 99})
	if err != nil {
		t.Fatal(err)
	}
	var m meta
	if err := json.Unmarshal([]byte(mustRead(t, res.MetaFile)), &m); err != nil {
		t.Fatal(err)
	}
	if m.UltimoEventoEm == "" {
		t.Fatal("ultimo_evento_em vazio")
	}
	if want := gitOut(t, filepath.Join(agents, "agente"), "rev-parse", "HEAD"); m.Head != want {
		t.Fatalf("head=%q, quero %q", m.Head, want)
	}
	if m.InicioPID == "" {
		if _, err := os.Stat("/proc/self/stat"); err == nil {
			t.Fatal("inicio_pid vazio no Linux")
		}
	}
	if len(m.Checkpoints) == 0 || m.Checkpoints[0].Motivo != "base" {
		t.Fatalf("checkpoints=%+v", m.Checkpoints)
	}
}

func TestInicioProcessoDoProprioPID(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("sem /proc")
	}
	a, b := inicioProcesso(os.Getpid()), inicioProcesso(os.Getpid())
	if a == "" || a != b {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func gravarMeta(t *testing.T, agents, nome string, m meta) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(agents, "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(filepath.Join(agents, "logs", nome+".meta.json"), m); err != nil {
		t.Fatal(err)
	}
	escrever(t, filepath.Join(agents, "logs", nome+".log"), "oi\n")
}

func TestOrfaoPIDInexistenteEPIDTrocado(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("sem /proc")
	}
	agents := t.TempDir()
	ini := time.Now().Add(-time.Hour).Format(time.RFC3339)
	gravarMeta(t, agents, "morto", meta{Inicio: ini, PID: 2147483646})
	gravarMeta(t, agents, "trocado", meta{Inicio: ini, PID: os.Getpid(), InicioPID: "1"})
	gravarMeta(t, agents, "vivo", meta{Inicio: ini, PID: os.Getpid(), InicioPID: inicioProcesso(os.Getpid())})
	gravarMeta(t, agents, "filho", meta{Inicio: ini, PID: os.Getpid(), Pai: "morto"})
	items, err := ListAgents(agents, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	por := map[string]Agente{}
	for _, a := range items {
		por[a.Nome] = a
	}
	for _, n := range []string{"morto", "trocado"} {
		if por[n].Estado != "órfão" {
			t.Fatalf("%s: estado %q", n, por[n].Estado)
		}
		var m meta
		_ = json.Unmarshal([]byte(mustRead(t, filepath.Join(agents, "logs", n+".meta.json"))), &m)
		if m.Fim == "" || m.Motivo != MotivoOrfao || m.Codigo == nil || *m.Codigo != -1 {
			t.Fatalf("%s: meta não fechado: %+v", n, m)
		}
	}
	if por["vivo"].Estado != "rodando" {
		t.Fatalf("vivo: %q", por["vivo"].Estado)
	}
	if !por["filho"].PaiMorto {
		t.Fatal("filho devia mostrar pai morto")
	}
	var f meta
	_ = json.Unmarshal([]byte(mustRead(t, filepath.Join(agents, "logs", "filho.meta.json"))), &f)
	if f.Fim != "" {
		t.Fatal("filho de pai morto não deve ser alterado")
	}
	// liberou a vaga
	if n, _ := activeAgents(agents); n != 2 {
		t.Fatalf("ativos=%d, quero 2 (vivo e filho)", n)
	}
}

func TestPararOrfaoNaoMandaSinal(t *testing.T) {
	agents := t.TempDir()
	gravarMeta(t, agents, "x", meta{Inicio: time.Now().Format(time.RFC3339), PID: 2147483646, Servidor: true})
	orfao, err := PararAgente(agents, "x", time.Now())
	if err != nil || !orfao {
		t.Fatalf("orfao=%v err=%v", orfao, err)
	}
	var m meta
	_ = json.Unmarshal([]byte(mustRead(t, filepath.Join(agents, "logs", "x.meta.json"))), &m)
	if m.Fim == "" || m.Motivo != MotivoOrfao {
		t.Fatalf("%+v", m)
	}
}

func TestCheckpointDesfazerERefazer(t *testing.T) {
	agents, wt := worktreeFixture(t)
	headIni := gitOut(t, wt, "rev-parse", "HEAD")
	escrever(t, filepath.Join(wt, "README"), "editado\n")
	escrever(t, filepath.Join(wt, "novo.txt"), "novo\n")
	escrever(t, filepath.Join(wt, "x.log"), "ignorado\n")
	indiceAntes := gitOut(t, wt, "status", "--porcelain")

	c1, ok, err := CheckpointGit(wt, "ag", "base")
	if err != nil || !ok || c1.N != 1 {
		t.Fatalf("c1=%+v ok=%v err=%v", c1, ok, err)
	}
	if gitOut(t, wt, "rev-parse", "HEAD") != headIni || gitOut(t, wt, "status", "--porcelain") != indiceAntes {
		t.Fatal("checkpoint mexeu no HEAD ou no índice")
	}
	if _, ok, _ := CheckpointGit(wt, "ag", "igual"); ok {
		t.Fatal("árvore igual não devia gerar checkpoint")
	}
	if err := os.Remove(filepath.Join(wt, "README")); err != nil {
		t.Fatal(err)
	}
	escrever(t, filepath.Join(wt, "outro.txt"), "outro\n")
	c2, ok, _ := CheckpointGit(wt, "ag", "turno")
	if !ok || c2.N != 2 {
		t.Fatalf("c2=%+v", c2)
	}
	infos, err := ResumoCheckpoints(wt, "ag")
	if err != nil || len(infos) != 2 || infos[1].Resumo == "" {
		t.Fatalf("infos=%+v err=%v", infos, err)
	}

	escrever(t, filepath.Join(wt, "extra.txt"), "depois do último checkpoint\n")
	cp, err := DesfazerAgente(agents, "ag", 0, false)
	if err != nil || cp.N != 1 {
		t.Fatalf("desfazer: %+v %v", cp, err)
	}
	if mustRead(t, filepath.Join(wt, "README")) != "editado\n" {
		t.Fatal("arquivo apagado não voltou")
	}
	if _, err := os.Stat(filepath.Join(wt, "outro.txt")); !os.IsNotExist(err) {
		t.Fatal("arquivo novo devia ter sumido")
	}
	if _, err := os.Stat(filepath.Join(wt, "x.log")); err != nil {
		t.Fatal("arquivo ignorado devia ficar")
	}
	if mustRead(t, filepath.Join(wt, "novo.txt")) != "novo\n" {
		t.Fatal("novo.txt do checkpoint 1 devia existir")
	}
	// refazer pelo "antes-de-desfazer"
	lista, _ := ListarCheckpoints(wt, "ag")
	ult := lista[len(lista)-1]
	if ult.Motivo != MotivoAntesDeDesfazer || ult.N != 3 {
		t.Fatalf("ultimo=%+v", ult)
	}
	if _, err := DesfazerAgente(agents, "ag", ult.N, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wt, "README")); !os.IsNotExist(err) {
		t.Fatal("README devia estar apagado de novo")
	}
	if mustRead(t, filepath.Join(wt, "outro.txt")) != "outro\n" || mustRead(t, filepath.Join(wt, "extra.txt")) == "" {
		t.Fatal("outro.txt e extra.txt deviam voltar ao refazer")
	}
	if err := ApagarCheckpoints(wt, "ag"); err != nil {
		t.Fatal(err)
	}
	if l, _ := ListarCheckpoints(wt, "ag"); len(l) != 0 {
		t.Fatalf("refs restantes: %+v", l)
	}
}

func TestDesfazerRestauraHeadERecusaRodando(t *testing.T) {
	agents, wt := worktreeFixture(t)
	head0 := gitOut(t, wt, "rev-parse", "HEAD")
	escrever(t, filepath.Join(wt, "a.txt"), "1\n")
	if _, ok, err := CheckpointGit(wt, "ag", "base"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "agente commitou")
	escrever(t, filepath.Join(wt, "a.txt"), "2\n")
	if _, ok, err := CheckpointGit(wt, "ag", "turno"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	gravarMeta(t, agents, "ag", meta{Projeto: "p", PID: os.Getpid(), Worktree: wt})
	if _, err := DesfazerAgente(agents, "ag", 1, false); err == nil || !strings.Contains(err.Error(), "rodando") {
		t.Fatalf("devia recusar: %v", err)
	}
	if _, err := DesfazerAgente(agents, "ag", 1, true); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, wt, "rev-parse", "HEAD"); got != head0 {
		t.Fatalf("HEAD=%s, quero %s", got, head0)
	}
	if mustRead(t, filepath.Join(wt, "a.txt")) != "1\n" {
		t.Fatal("conteúdo errado")
	}
}

func TestDesfazerRecusaRepositorioPrincipal(t *testing.T) {
	root, agents := repoFixture(t)
	if _, ok, err := CheckpointGit(root, "ag", "base"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	escrever(t, filepath.Join(root, "README"), "mudou\n")
	if _, _, err := CheckpointGit(root, "ag", "turno"); err != nil {
		t.Fatal(err)
	}
	gravarMeta(t, agents, "ag", meta{Projeto: "p", PID: 2147483646, Worktree: root, Fim: "2026-01-01T00:00:00Z"})
	if _, err := DesfazerAgente(agents, "ag", 1, true); err == nil {
		t.Fatal("devia recusar o repositório principal")
	}
	if mustRead(t, filepath.Join(root, "README")) != "mudou\n" {
		t.Fatal("repositório principal foi alterado")
	}
}

func TestCheckpointForaDeGitNaoFazNada(t *testing.T) {
	if _, ok, err := CheckpointGit(t.TempDir(), "ag", "base"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
