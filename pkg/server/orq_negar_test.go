package server_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

func servidorNegar(t *testing.T, negarEncerra bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := server.NewWSServerWithMaxAgents(session.NewManager(), 0)
	s.SetNegarEncerra(negarEncerra)
	go func() { _ = s.ListenAndServe(addr) }()
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	return addr
}

// negarAteFim nega a primeira decisão (decidir monta os params) e devolve o orq.fim
// e quantas tentativas (orq.inicio) houve. Decisões seguintes também são negadas.
func negarAteFim(t *testing.T, addr, root, nome string, decidir func(dec protocol.OrqDecisaoParams) interface{}) (protocol.OrqFimParams, int) {
	t.Helper()
	c := conectar(t, addr)
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)
	c.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: nome, Motor: "mock", CWD: root, MaxAgentes: 99, Tentativas: 2}}, nil)
	inicios := 0
	timeout := time.After(60 * time.Second)
	for {
		select {
		case n := <-c.eventos:
			raw := n.Params.(json.RawMessage)
			switch n.Method {
			case protocol.EventOrqInicio:
				inicios++
			case protocol.EventOrqPrecisaDecisao:
				var dec protocol.OrqDecisaoParams
				_ = json.Unmarshal(raw, &dec)
				c.resultado(protocol.MethodRodarDecidir, decidir(dec), nil)
			case protocol.EventOrqFim:
				var fim protocol.OrqFimParams
				_ = json.Unmarshal(raw, &fim)
				return fim, inicios
			}
		case <-timeout:
			t.Fatal("timeout esperando orq.fim")
		}
	}
}

func lerMetaNegar(t *testing.T, root, nome string) (map[string]interface{}, string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "agentes", "logs")
	b, err := os.ReadFile(filepath.Join(dir, nome+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	log, _ := os.ReadFile(filepath.Join(dir, nome+".log"))
	return m, string(log)
}

var fimCodigo3 = regexp.MustCompile(`(?m)^FIM \d\d:\d\d código 3$`)

func TestNegarComEncerrarTerminaComCodigo3(t *testing.T) {
	root := repoOrq(t, "mock-ne")
	sim := true
	fim, inicios := negarAteFim(t, servidorNegar(t, false), root, "mock-ne", func(d protocol.OrqDecisaoParams) interface{} {
		return protocol.RodarDecidirParams{Run: d.Run, ID: d.ID, Resposta: "negar", Encerrar: &sim}
	})
	if fim.Codigo != 3 || fim.Motivo != "negado" || fim.Tentativas != 1 || inicios != 1 {
		t.Fatalf("fim %+v, inícios %d", fim, inicios)
	}
	m, log := lerMetaNegar(t, root, "mock-ne")
	if m["codigo"] != 3.0 || m["motivo"] != "negado" {
		t.Fatalf("meta: %v", m)
	}
	if !fimCodigo3.MatchString(log) || regexp.MustCompile(`tentando continuar`).MatchString(log) {
		t.Fatalf("log:\n%s", log)
	}
}

func TestNegarSemEncerrarMantemNovaTentativa(t *testing.T) {
	root := repoOrq(t, "mock-nc")
	fim, inicios := negarAteFim(t, servidorNegar(t, false), root, "mock-nc", func(d protocol.OrqDecisaoParams) interface{} {
		return protocol.RodarDecidirParams{Run: d.Run, ID: d.ID, Resposta: "negar"}
	})
	if fim.Codigo != 1 || fim.Motivo != "" || inicios != 2 {
		t.Fatalf("fim %+v, inícios %d", fim, inicios)
	}
	if m, _ := lerMetaNegar(t, root, "mock-nc"); m["motivo"] != nil {
		t.Fatalf("meta não deveria ter motivo: %v", m)
	}
}

func TestServeNegarEncerraClienteAntigo(t *testing.T) {
	root := repoOrq(t, "mock-na")
	// Cliente antigo: JSON cru sem o campo encerrar; vale a flag do servidor.
	fim, inicios := negarAteFim(t, servidorNegar(t, true), root, "mock-na", func(d protocol.OrqDecisaoParams) interface{} {
		return map[string]string{"id": d.ID, "resposta": "negar"}
	})
	if fim.Codigo != 3 || fim.Motivo != "negado" || inicios != 1 {
		t.Fatalf("fim %+v, inícios %d", fim, inicios)
	}
}

func TestEncerrarFalseVenceFlagDoServidor(t *testing.T) {
	root := repoOrq(t, "mock-nf")
	nao := false
	fim, inicios := negarAteFim(t, servidorNegar(t, true), root, "mock-nf", func(d protocol.OrqDecisaoParams) interface{} {
		return protocol.RodarDecidirParams{Run: d.Run, ID: d.ID, Resposta: "negar", Encerrar: &nao}
	})
	if fim.Codigo != 1 || inicios != 2 {
		t.Fatalf("fim %+v, inícios %d", fim, inicios)
	}
}

func TestPermitirComEncerrarNaoEncerra(t *testing.T) {
	root := repoOrq(t, "mock-np")
	sim := true
	fim, inicios := negarAteFim(t, servidorNegar(t, true), root, "mock-np", func(d protocol.OrqDecisaoParams) interface{} {
		return protocol.RodarDecidirParams{Run: d.Run, ID: d.ID, Resposta: "permitir", Encerrar: &sim}
	})
	if fim.Codigo != 0 || fim.Motivo != "" || inicios != 1 {
		t.Fatalf("fim %+v, inícios %d", fim, inicios)
	}
}
