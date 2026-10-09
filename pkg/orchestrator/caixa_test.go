package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func agenteVivoFixture(t *testing.T, nome string) string {
	t.Helper()
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatal(err)
	}
	m := meta{Projeto: "p", Motor: "mock", Inicio: time.Now().Format(time.RFC3339), PID: os.Getpid()}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(logs, nome+".meta.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnviarMensagemGravaNaCaixaComPermissaoPrivada(t *testing.T) {
	dir := agenteVivoFixture(t, "vivo")
	m, err := EnviarMensagem(dir, "vivo", "p", "faça X", time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.Recibo() != MsgPendente || !strings.HasPrefix(m.ID, "msg-") {
		t.Fatalf("mensagem: %+v", m)
	}
	path := filepath.Join(caixaDir(dir, "vivo"), m.ID+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("permissão do arquivo: %v", info.Mode().Perm())
	}
	if pend := MensagensPendentes(dir, "vivo"); len(pend) != 1 || pend[0].Texto != "faça X" {
		t.Fatalf("pendentes: %+v", pend)
	}
	items, _ := ListAgents(dir, time.Now())
	if len(items) != 1 || items[0].MensagensPendentes != 1 {
		t.Fatalf("agentes ver deve mostrar pendentes: %+v", items)
	}
}

func TestEnviarMensagemRecusaAgenteTerminadoInexistenteEVazio(t *testing.T) {
	dir := agenteVivoFixture(t, "fim")
	code := 0
	m := meta{Fim: time.Now().Format(time.RFC3339), Codigo: &code}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "logs", "fim.meta.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnviarMensagem(dir, "fim", "", "oi", time.Now(), 0); err == nil || !strings.Contains(err.Error(), "já terminou") {
		t.Fatalf("terminado: %v", err)
	}
	if _, err := EnviarMensagem(dir, "nada", "", "oi", time.Now(), 0); err == nil || !strings.Contains(err.Error(), "não encontrado") {
		t.Fatalf("inexistente: %v", err)
	}
	if _, err := EnviarMensagem(dir, "../x", "", "oi", time.Now(), 0); err == nil {
		t.Fatal("nome com barra deve ser recusado")
	}
	if _, err := EnviarMensagem(dir, "fim", "", "  ", time.Now(), 0); err == nil {
		t.Fatal("texto vazio deve ser recusado")
	}
	if _, err := os.Stat(caixaDir(dir, "fim")); err == nil {
		t.Fatal("agente terminado não deve ganhar caixa")
	}
}

type vivoFalso struct {
	harness.Harness
	recebidos []string
}

func (v *vivoFalso) EnviarVivo(id, texto string) error {
	v.recebidos = append(v.recebidos, texto)
	return nil
}

func TestCaixaRunEntregaVivoERetomada(t *testing.T) {
	dir := agenteVivoFixture(t, "ag")
	var log strings.Builder
	var eventos []Evento
	o := Options{Name: "ag", Now: time.Now, OnEvent: func(e Evento) { eventos = append(eventos, e) }}
	c := novaCaixaRun(dir, o, func(s string) { log.WriteString(s) })

	a, _ := EnviarMensagem(dir, "ag", "", "um", time.Now(), 0)
	f := &vivoFalso{}
	c.entregarVivas(f)
	if len(f.recebidos) != 1 || !strings.Contains(f.recebidos[0], a.ID) || !strings.Contains(f.recebidos[0], "um") {
		t.Fatalf("vivo recebeu: %v", f.recebidos)
	}
	if got := ListarMensagens(dir, "ag")[0]; got.Estado != MsgEntregue || got.Modo != ModoVivo {
		t.Fatalf("estado: %+v", got)
	}

	// Motor sem entrada viva: fica pendente até o fim do turno e vai na retomada.
	b, _ := EnviarMensagem(dir, "ag", "", "dois", time.Now(), 0)
	c.entregarVivas(struct{ harness.Harness }{})
	if len(MensagensPendentes(dir, "ag")) != 1 {
		t.Fatal("deveria seguir pendente")
	}
	texto := c.retomada()
	if !strings.Contains(texto, "dois") || !strings.Contains(texto, b.ID) {
		t.Fatalf("texto da retomada: %q", texto)
	}
	if c.retomada() != "" || len(MensagensPendentes(dir, "ag")) != 0 {
		t.Fatal("retomada deve consumir as pendentes")
	}
	for _, want := range []string{"MENSAGEM recebida " + a.ID, "MENSAGEM entregue " + a.ID + " (vivo)", "MENSAGEM entregue " + b.ID + " (retomada)"} {
		if !strings.Contains(log.String(), want) {
			t.Fatalf("log sem %q:\n%s", want, log.String())
		}
	}
	var entregues int
	for _, e := range eventos {
		if e.Tipo == EvMensagem && e.MensagemEstado == MsgEntregue {
			entregues++
		}
	}
	if entregues != 2 {
		t.Fatalf("eventos orq.mensagem entregues: %d", entregues)
	}
}
