package server

import (
	"testing"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestLimitarAgentes(t *testing.T) {
	sem := newOrq()
	defer sem.Close()
	if got := sem.limitarAgentes(7); got != 7 {
		t.Fatalf("sem teto: %d", got)
	}
	o := newOrq(3)
	defer o.Close()
	for pedido, want := range map[int]int{0: 3, -1: 3, 2: 2, 3: 3, 99: 3} {
		if got := o.limitarAgentes(pedido); got != want {
			t.Fatalf("limitarAgentes(%d) = %d, quer %d", pedido, got, want)
		}
	}
}

func TestDecidirRunErradoNaoConsomeDecisao(t *testing.T) {
	o := newOrq()
	defer o.Close()
	d := &decisao{ch: make(chan decisaoResp, 1)}
	d.params.ID, d.params.Run = "dec-1", "rodar-1"
	o.decisoes["dec-1"] = d
	if err := o.decidir(protocol.RodarDecidirParams{Run: "rodar-2", ID: "dec-1", Resposta: "permitir"}); err == nil {
		t.Fatal("run errado deveria falhar")
	}
	if o.decisoes["dec-1"] == nil {
		t.Fatal("decisão foi consumida pelo run errado")
	}
	if err := o.decidir(protocol.RodarDecidirParams{Run: "rodar-1", ID: "dec-1", Resposta: "permitir"}); err != nil {
		t.Fatal(err)
	}
	if r := <-d.ch; !r.allow {
		t.Fatal("deveria permitir")
	}
}
