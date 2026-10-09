package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListAgentsMostraEstadoDuracaoELinha(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	code := 0
	m := meta{Projeto: "openheinerss", Motor: "mock", Modelo: "teste", Tentativa: 2, Inicio: started.Format(time.RFC3339), Fim: started.Add(7 * time.Minute).Format(time.RFC3339), Codigo: &code}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(logs, "exemplo.meta.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "exemplo.log"), []byte("texto útil\nFIM 09:07 código 0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	items, err := ListAgents(dir, started.Add(10*time.Minute))
	if err != nil || len(items) != 1 {
		t.Fatalf("lista: err=%v itens=%d", err, len(items))
	}
	a := items[0]
	if a.Nome != "exemplo" || a.Estado != "terminou código 0" || a.Duracao != "7m0s" || a.UltimaLinha != "texto útil" {
		t.Fatalf("agente incorreto: %+v", a)
	}
}

func TestListAgentsMarcaLogLento(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatal(err)
	}
	m := meta{Inicio: time.Now().Add(-20 * time.Minute).Format(time.RFC3339), PID: os.Getpid()}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(logs, "velho.meta.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(logs, "velho.log")
	if err := os.WriteFile(log, []byte("sem atualização\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-16 * time.Minute)
	if err := os.Chtimes(log, old, old); err != nil {
		t.Fatal(err)
	}
	items, err := ListAgents(dir, time.Now())
	if err != nil || len(items) != 1 || items[0].Estado != "lento" {
		t.Fatalf("estado lento (só S1): err=%v itens=%+v", err, items)
	}
}

func TestShowAgentLogLimitaFim(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "a.log"), []byte("1\n2\n3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ShowAgentLog(dir, "a", 2)
	if err != nil || got != "2\n3" || strings.Contains(got, "1") {
		t.Fatalf("fim do log: %q (%v)", got, err)
	}
}

func TestStopAgentRecusaExecucaoDeServidor(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	_ = os.MkdirAll(logs, 0755)
	if err := writeMeta(filepath.Join(logs, "x.meta.json"), meta{Motor: "mock", PID: os.Getpid(), Servidor: true}); err != nil {
		t.Fatal(err)
	}
	err := StopAgent(dir, "x", time.Now())
	if err == nil || !strings.Contains(err.Error(), "rodar.parar") {
		t.Fatalf("esperava recusa orientando rodar.parar, veio %v", err)
	}
}
