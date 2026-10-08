package server

import (
	"sync"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

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
