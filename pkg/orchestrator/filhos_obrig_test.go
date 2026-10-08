package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// filhoFalso grava o meta.json de um filho em dir e devolve o trecho de shell que o registra no pai.
func filhoFalso(t *testing.T, dir, nome string, m meta) string {
	t.Helper()
	p := filepath.Join(dir, nome+".meta.json")
	b, _ := json.Marshal(m)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return "mkdir -p \"$OPENHEINERSS_PAI_LOGS/$OPENHEINERSS_PAI.filhos\" && echo '" + p + "' > \"$OPENHEINERSS_PAI_LOGS/$OPENHEINERSS_PAI.filhos/" + nome + "\"\n"
}

// processoVivo inicia um sleep (PID de um filho "vivo") e o encerra no fim do teste.
func processoVivo(t *testing.T) int {
	t.Helper()
	c := exec.Command("sleep", "60")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _, _ = c.Process.Wait() })
	return c.Process.Pid
}

type eventos struct {
	mu sync.Mutex
	l  []Evento
}

func (e *eventos) add(ev Evento) { e.mu.Lock(); e.l = append(e.l, ev); e.mu.Unlock() }
func (e *eventos) tipo(t string) *Evento {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.l {
		if e.l[i].Tipo == t {
			return &e.l[i]
		}
	}
	return nil
}

func lerMetaTeste(t *testing.T, p string) meta {
	t.Helper()
	var m meta
	if err := json.Unmarshal([]byte(mustRead(t, p)), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFilhosObrigatoriosFilhoFalhou(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts sh")
	}
	um := 1
	zero := 0
	for _, obrig := range []bool{false, true} {
		root, agents := repoFixture(t)
		dados := t.TempDir()
		corpo := filhoFalso(t, dados, "filho-ok", meta{Fim: time.Now().Format(time.RFC3339), Codigo: &zero}) +
			filhoFalso(t, dados, "filho-ruim", meta{Fim: time.Now().Format(time.RFC3339), Codigo: &um}) + texto("pronto")
		nomeMotor := "pai-filho-falho"
		if obrig {
			nomeMotor += "-obrig"
		}
		registrar(t, harness.CustomSpec{Name: nomeMotor, Command: paiScript(t, dados, corpo, texto("pronto"))})
		var evs eventos
		res, err := Run(context.Background(), root, Options{Name: "pai", Motor: nomeMotor, AgentsDir: agents, PromptText: "x", MaxAgents: 9, IntervaloFilhos: 20 * time.Millisecond, FilhosObrigatorios: obrig, OnEvent: evs.add})
		if err != nil {
			t.Fatal(err)
		}
		m := lerMetaTeste(t, res.MetaFile)
		if !obrig {
			if res.Code != 0 || m.Motivo != "" {
				t.Fatalf("sem --filhos-obrigatorios o pai segue com 0: código=%d motivo=%q", res.Code, m.Motivo)
			}
			continue
		}
		if res.Code != CodigoFilhoFalhou || !strings.Contains(res.Causa, "filho-ruim (código 1)") || strings.Contains(res.Causa, "filho-ok") {
			t.Fatalf("código=%d causa=%q", res.Code, res.Causa)
		}
		if m.Motivo != MotivoFilhoFalhou || len(m.FilhosFalhos) != 1 || m.FilhosFalhos[0] != "filho-ruim" || valueOr(m.Codigo, 0) != CodigoFilhoFalhou {
			t.Fatalf("meta: %+v", m)
		}
		log := mustRead(t, res.LogFile)
		if !strings.Contains(log, "filho falhou (--filhos-obrigatorios): filho-ruim (código 1)") || !strings.Contains(log, "código 4") {
			t.Fatalf("log:\n%s", log)
		}
		if fim := evs.tipo(EvFim); fim == nil || fim.Motivo != MotivoFilhoFalhou || fim.Codigo != 4 || len(fim.Filhos) != 1 {
			t.Fatalf("orq.fim: %+v", fim)
		}
	}
}

func TestFilhoOrfaoNaoDaFimZeroEmSilencio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("scripts sh")
	}
	for _, obrig := range []bool{false, true} {
		root, agents := repoFixture(t)
		dados := t.TempDir()
		pid := processoVivo(t)
		// O meta do filho fica na pasta de logs do pai (como um filho no mesmo repositório).
		logs := filepath.Join(agents, "logs")
		if err := os.MkdirAll(logs, 0755); err != nil {
			t.Fatal(err)
		}
		corpo := filhoFalso(t, logs, "filho-lento", meta{Inicio: time.Now().Format(time.RFC3339), PID: pid, Pai: "pai", Motor: "x"}) + textoAguardando
		nomeMotor := "pai-orfao"
		if obrig {
			nomeMotor += "-obrig"
		}
		registrar(t, harness.CustomSpec{Name: nomeMotor, Command: paiScript(t, dados, corpo, texto("encerrando"))})
		var evs eventos
		res, err := Run(context.Background(), root, Options{Name: "pai", Motor: nomeMotor, AgentsDir: agents, PromptText: "x", MaxAgents: 9, IntervaloFilhos: 20 * time.Millisecond, EsperarFilhos: 150 * time.Millisecond, RodadasFilhos: 1, FilhosObrigatorios: obrig, OnEvent: evs.add})
		if err != nil {
			t.Fatal(err)
		}
		m := lerMetaTeste(t, res.MetaFile)
		log := mustRead(t, res.LogFile)
		if !strings.Contains(log, "espera pelos filhos venceu") || !strings.Contains(log, "AVISO: o pai terminou com 1 agente(s) filho(s) ainda rodando (órfãos): filho-lento") {
			t.Fatalf("log sem aviso de órfão:\n%s", log)
		}
		if ev := evs.tipo(EvFilhosOrfaos); ev == nil || len(ev.Filhos) != 1 || ev.Filhos[0] != "filho-lento" {
			t.Fatalf("evento de órfãos: %+v", ev)
		}
		if len(m.FilhosOrfaos) != 1 {
			t.Fatalf("meta sem filhos_orfaos: %+v", m)
		}
		if obrig {
			if res.Code != CodigoFilhoFalhou || m.Motivo != MotivoFilhoFalhou || !strings.Contains(res.Causa, "filho-lento (ainda rodando") {
				t.Fatalf("com --filhos-obrigatorios órfão falha o pai: código=%d motivo=%q causa=%q", res.Code, m.Motivo, res.Causa)
			}
			continue
		}
		if res.Code != 0 || m.Motivo != MotivoFilhosOrfaos {
			t.Fatalf("código=%d motivo=%q", res.Code, m.Motivo)
		}
		if fim := evs.tipo(EvFim); fim == nil || fim.Motivo != MotivoFilhosOrfaos || len(fim.Filhos) != 1 {
			t.Fatalf("orq.fim: %+v", fim)
		}
		lista, err := ListAgents(agents, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		vistos := 0
		for _, a := range lista {
			if a.Nome == "pai" && (len(a.Orfaos) != 1 || a.Orfaos[0] != "filho-lento") {
				t.Fatalf("agentes não mostra os órfãos do pai: %+v", a)
			}
			if a.Nome == "filho-lento" && !a.Orfao {
				t.Fatalf("agentes não marca o filho órfão: %+v", a)
			}
			vistos++
		}
		if vistos != 2 {
			t.Fatalf("agentes: %+v", lista)
		}
	}
}
