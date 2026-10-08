package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestObservadorAvisaAgenteTravado(t *testing.T) {
	t.Setenv("OPENHEINERSS_LOG_PARADO_MIN", "1")
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.WriteFile(filepath.Join(logs, "x.log"), []byte("última linha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(logs, "x.log"), old, old); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]interface{}{"motor": "mock", "tentativa": 1, "inicio": old.Format(time.RFC3339), "pid": os.Getpid()})
	if err := os.WriteFile(filepath.Join(logs, "x.meta.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	o := newOrq()
	defer o.Close()
	var mu sync.Mutex
	var mensagens []string
	o.subs[&conexao{send: func(v interface{}) {
		n := v.(protocol.Notification)
		if n.Method == protocol.EventOrqErro {
			mu.Lock()
			mensagens = append(mensagens, n.Params.(protocol.OrqErroParams).Mensagem)
			mu.Unlock()
		}
	}}] = assinatura{}
	o.pastas[dir] = &pasta{dir: dir, projeto: "teste", estados: map[string]*estadoMeta{}}
	o.varrer(dir)
	mu.Lock()
	defer mu.Unlock()
	if len(mensagens) != 1 || !containsMensagem(mensagens[0], "travou") {
		t.Fatalf("aviso de travamento: %v", mensagens)
	}
}

func containsMensagem(s, parte string) bool { return strings.Contains(s, parte) }

func TestProgressoLimitadoPorAgente(t *testing.T) {
	old := intervaloProgresso
	intervaloProgresso = 150 * time.Millisecond
	defer func() { intervaloProgresso = old }()
	o := newOrq()
	defer o.Close()
	var mu sync.Mutex
	var recebidos []string
	c := &conexao{send: func(v interface{}) {
		mu.Lock()
		defer mu.Unlock()
		recebidos = append(recebidos, v.(protocol.Notification).Params.(protocol.OrqProgressoParams).Resumo)
	}}
	o.subs[c] = assinatura{}
	for _, r := range []string{"a", "b", "c", "d"} {
		o.progresso(protocol.OrqProgressoParams{Agente: "x", Projeto: "p", Resumo: r})
	}
	// outro agente tem a própria janela
	o.progresso(protocol.OrqProgressoParams{Agente: "y", Projeto: "p", Resumo: "y1"})
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	if len(recebidos) != 2 || recebidos[0] != "a" || recebidos[1] != "y1" {
		t.Fatalf("durante a janela: %v", recebidos)
	}
	mu.Unlock()
	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(recebidos) != 3 || recebidos[2] != "d" {
		t.Fatalf("depois da janela deveria sair só o mais recente: %v", recebidos)
	}
}

func TestFimDescartaProgressoPendente(t *testing.T) {
	old := intervaloProgresso
	intervaloProgresso = 100 * time.Millisecond
	defer func() { intervaloProgresso = old }()
	o := newOrq()
	defer o.Close()
	var mu sync.Mutex
	var metodos []string
	o.subs[&conexao{send: func(v interface{}) {
		mu.Lock()
		metodos = append(metodos, v.(protocol.Notification).Method)
		mu.Unlock()
	}}] = assinatura{}
	o.progresso(protocol.OrqProgressoParams{Agente: "x", Projeto: "p", Resumo: "1"})
	o.progresso(protocol.OrqProgressoParams{Agente: "x", Projeto: "p", Resumo: "2"})
	o.fim(protocol.OrqFimParams{Agente: "x", Projeto: "p"})
	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(metodos) != 2 || metodos[1] != protocol.EventOrqFim {
		t.Fatalf("esperava progresso e fim, veio %v", metodos)
	}
}

func TestObservadorEmiteFilhosOrfaos(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0755); err != nil {
		t.Fatal(err)
	}
	agora := time.Now().Format(time.RFC3339)
	b, _ := json.Marshal(map[string]interface{}{"motor": "mock", "tentativa": 1, "inicio": agora, "fim": agora, "codigo": 0, "motivo": "filhos órfãos", "filhos_orfaos": []string{"filho-a"}})
	if err := os.WriteFile(filepath.Join(logs, "pai.meta.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "pai.log"), []byte("FIM\n"), 0644); err != nil {
		t.Fatal(err)
	}
	o := newOrq()
	defer o.Close()
	var mu sync.Mutex
	var orfaos protocol.OrqFilhosOrfaosParams
	var fim protocol.OrqFimParams
	o.subs[&conexao{send: func(v interface{}) {
		n := v.(protocol.Notification)
		mu.Lock()
		defer mu.Unlock()
		switch n.Method {
		case protocol.EventOrqFilhosOrfaos:
			orfaos = n.Params.(protocol.OrqFilhosOrfaosParams)
		case protocol.EventOrqFim:
			fim = n.Params.(protocol.OrqFimParams)
		}
	}}] = assinatura{}
	o.pastas[dir] = &pasta{dir: dir, projeto: "teste", baseline: true, estados: map[string]*estadoMeta{}} // pasta já conhecida: o fim é novo
	o.varrer(dir)
	mu.Lock()
	defer mu.Unlock()
	if len(orfaos.Filhos) != 1 || orfaos.Filhos[0] != "filho-a" || orfaos.Geracao == "" || !strings.Contains(orfaos.Mensagem, "órfãos") {
		t.Fatalf("orq.filhos_orfaos: %+v", orfaos)
	}
	if fim.Motivo != "filhos órfãos" || len(fim.Filhos) != 1 {
		t.Fatalf("orq.fim: %+v", fim)
	}
}
