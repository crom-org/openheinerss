package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
)

// repoTemp cria um repositório git com um commit em /tmp.
func repoTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "parado-repo-")
	if err != nil {
		dir = t.TempDir()
	} else {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "base"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

type relogio struct{ t time.Time }

func (r *relogio) now() time.Time { return r.t }

func regra(acao string) config.Parado {
	return config.Parado{Aviso: 10 * time.Minute, Parar: 20 * time.Minute, Acao: acao}
}

// novoTeste monta um detector com relógio falso, repositório real e log "congelado" na hora inicial.
func novoTeste(t *testing.T, cfg config.Parado, cpu func() (int64, bool)) (*DetectorParado, *relogio, string, *time.Time, *string) {
	t.Helper()
	dir := repoTemp(t)
	rel := &relogio{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	mtime, ultimo := rel.t, "mesma linha"
	if cpu == nil {
		cpu = func() (int64, bool) { return 0, false }
	}
	d := novoDetectorParado(cfg, rel.now, fontesParado{
		logMtime:  func() (time.Time, bool) { return mtime, true },
		worktree:  func() string { return assinaturaWorktree(dir) },
		logUltimo: func() string { return ultimo },
		cpu:       cpu,
	})
	return d, rel, dir, &mtime, &ultimo
}

func TestParadoAvisoDepoisParar(t *testing.T) {
	d, rel, _, _, _ := novoTeste(t, regra(config.ParadoAcaoParar), nil)
	rel.t = rel.t.Add(9 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelNenhum {
		t.Fatalf("aos 9 min não deveria avisar: %+v", v)
	}
	rel.t = rel.t.Add(1 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelAviso || !v.Novo {
		t.Fatalf("aos 10 min deveria avisar (novo): %+v", v)
	}
	rel.t = rel.t.Add(5 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelAviso || v.Novo {
		t.Fatalf("aviso não se repete: %+v", v)
	}
	rel.t = rel.t.Add(5 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelParar || !v.Novo {
		t.Fatalf("aos 20 min deveria parar: %+v", v)
	}
}

func TestParadoAcaoAvisoNuncaPara(t *testing.T) {
	d, rel, _, _, _ := novoTeste(t, regra(config.ParadoAcaoAviso), nil)
	rel.t = rel.t.Add(40 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelAviso {
		t.Fatalf("acao aviso só avisa: %+v", v)
	}
}

func TestParadoArquivoMudandoNaoEhParado(t *testing.T) {
	d, rel, dir, _, _ := novoTeste(t, regra(config.ParadoAcaoParar), nil)
	arq := filepath.Join(dir, "a.txt")
	for i := 0; i < 6; i++ { // 30 min com o log e o evento parados, mas a worktree mudando a cada 5 min
		rel.t = rel.t.Add(5 * time.Minute)
		if err := os.WriteFile(arq, []byte{byte('a' + i)}, 0644); err != nil {
			t.Fatal(err)
		}
		if v := d.Verificar(); v.Nivel != nivelNenhum {
			t.Fatalf("passo %d: worktree mudando não é parado: %+v", i, v)
		}
	}
	// Parando de editar, a contagem recomeça do último movimento.
	rel.t = rel.t.Add(10 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelAviso {
		t.Fatalf("10 min sem mudar deveria avisar: %+v", v)
	}
}

func TestParadoCPUAltoSoAvisa(t *testing.T) {
	var ticks int64
	d, rel, _, _, _ := novoTeste(t, regra(config.ParadoAcaoParar), func() (int64, bool) { return ticks, true })
	for i := 0; i < 8; i++ { // 40 min com ~100% de um núcleo
		rel.t = rel.t.Add(5 * time.Minute)
		ticks += 30000
		v := d.Verificar()
		if v.Nivel == nivelParar {
			t.Fatalf("CPU alta nunca deve parar: %+v", v)
		}
		if i >= 3 && (v.Nivel != nivelAviso || v.CPUAlto == nil || !*v.CPUAlto) {
			t.Fatalf("passo %d: esperava aviso com CPU alta: %+v", i, v)
		}
	}
	// CPU zerou: agora vale parar.
	rel.t = rel.t.Add(5 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelParar {
		t.Fatalf("CPU ~0 depois do tempo deveria parar: %+v", v)
	}
}

func TestParadoLacoComLogAtivo(t *testing.T) {
	d, rel, dir, mtime, _ := novoTeste(t, regra(config.ParadoAcaoParar), nil)
	for i := 0; i < 2; i++ { // log escrito sempre, mesmo último evento
		rel.t = rel.t.Add(6 * time.Minute)
		*mtime = rel.t
		_ = os.WriteFile(filepath.Join(dir, "x"), []byte{byte(i)}, 0644)
		v := d.Verificar()
		if i == 0 && v.Nivel != nivelNenhum || i == 1 && (v.Nivel != nivelLaco || !v.Novo) {
			t.Fatalf("passo %d: %+v", i, v)
		}
	}
}

func TestParadoEscritaPropriaNaoContaComoAtividade(t *testing.T) {
	d, rel, _, mtime, _ := novoTeste(t, regra(config.ParadoAcaoParar), nil)
	rel.t = rel.t.Add(10 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelAviso {
		t.Fatal("esperava aviso")
	}
	*mtime = rel.t // o detector escreveu "[parado]" no log
	d.MarcarEscritaPropria()
	rel.t = rel.t.Add(10 * time.Minute)
	if v := d.Verificar(); v.Nivel != nivelParar {
		t.Fatalf("o aviso no log não pode zerar o S1: %+v", v)
	}
}

func TestUltimaLinhaLogIgnoraMarcasDoRunner(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	_ = os.WriteFile(p, []byte("trabalho real\n\n[parado] aviso\n"), 0644)
	if got := ultimaLinhaLog(p); got != "trabalho real" {
		t.Fatalf("got %q", got)
	}
}

func TestAssinaturaWorktreeMudaComEdicao(t *testing.T) {
	dir := repoTemp(t)
	a := assinaturaWorktree(dir)
	arq := filepath.Join(dir, "n.txt")
	_ = os.WriteFile(arq, []byte("1"), 0644)
	b := assinaturaWorktree(dir)
	if a == b {
		t.Fatal("arquivo novo deveria mudar a assinatura")
	}
	_ = os.WriteFile(arq, []byte("2"), 0644)
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(arq, future, future)
	if c := assinaturaWorktree(dir); c == b {
		t.Fatal("nova edição do mesmo arquivo (mtime) deveria mudar a assinatura")
	}
}

func TestCPUDescendentesLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("só Linux")
	}
	cmd := exec.Command("sh", "-c", "sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if _, ok := cpuDescendentes(os.Getpid()); !ok {
		t.Fatal("deveria ler /proc")
	}
}

func TestEstadoVivoParadoLentoRodando(t *testing.T) {
	logs := t.TempDir()
	agora := time.Now()
	logp := filepath.Join(logs, "a.log")
	_ = os.WriteFile(logp, []byte("x\n"), 0644)
	if got := estadoVivo(logs, "a", logp, agora); got != "rodando" {
		t.Fatalf("log fresco: %s", got)
	}
	velho := agora.Add(-16 * time.Minute)
	_ = os.Chtimes(logp, velho, velho)
	if got := estadoVivo(logs, "a", logp, agora); got != "lento" {
		t.Fatalf("só S1: %s", got)
	}
	grava := func(wt, ev int64, quando time.Time) {
		b, _ := json.Marshal(EstadoParado{VerificadoEm: quando.Format(time.RFC3339), LogParadoS: 960, WorktreeS: wt, EventoS: ev, AvisoS: 600})
		_ = os.WriteFile(caminhoEstadoParado(logs, "a"), b, 0644)
	}
	grava(700, 700, agora.Add(-30*time.Second))
	if got := estadoVivo(logs, "a", logp, agora); got != "parado" {
		t.Fatalf("S1+S2+S3: %s", got)
	}
	grava(100, 700, agora.Add(-30*time.Second))
	if got := estadoVivo(logs, "a", logp, agora); got != "lento" {
		t.Fatalf("worktree mexeu: %s", got)
	}
	grava(700, 700, agora.Add(-10*time.Minute))
	if got := estadoVivo(logs, "a", logp, agora); got != "lento" {
		t.Fatalf("estado velho: %s", got)
	}
}
