package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/limites"
)

func scriptFixo(t *testing.T, dir, nome, texto string) string {
	t.Helper()
	p := filepath.Join(dir, nome)
	corpo := "#!/bin/sh\nread p\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"" + texto + "\"}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"
	if err := os.WriteFile(p, []byte(corpo), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunTrocaDeContaAcimaDoLimiarRegistraMotivo(t *testing.T) {
	root, agents := repoFixture(t)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-cheia", Command: scriptFixo(t, root, "cheia.sh", "ERRADO conta cheia")}); err != nil {
		t.Fatal(err)
	}
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-livre", Command: scriptFixo(t, root, "livre.sh", "OK conta livre")}); err != nil {
		t.Fatal(err)
	}
	velho := avaliarCota
	t.Cleanup(func() { avaliarCota = velho })
	avaliarCota = func(_ context.Context, nome string, limiar float64, _ time.Duration, valida func(string) bool) limites.DecisaoCota {
		if nome != "conta-cheia" {
			return limites.DecisaoCota{Conhecida: true, Percentual: 10}
		}
		if !valida("conta-livre") || valida("nao-existe-xyz") {
			t.Errorf("validação de instâncias incorreta")
		}
		return limites.DecisaoCota{Conhecida: true, Percentual: 91, Acima: true, Alternativa: "conta-livre", AlternativaPercentual: 12}
	}
	var eventos []Evento
	res, err := Run(context.Background(), root, Options{Name: "troca", Motor: "conta-cheia", PromptText: "oi", AgentsDir: agents, QuotaMax: 80, MaxAgents: 99, OnEvent: func(e Evento) { eventos = append(eventos, e) }})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	log := mustRead(t, res.LogFile)
	if !strings.Contains(log, "OK conta livre") || strings.Contains(log, "ERRADO") || !strings.Contains(log, "trocando de conta") {
		t.Fatalf("log: %s", log)
	}
	var m meta
	if err := json.Unmarshal([]byte(mustRead(t, res.MetaFile)), &m); err != nil {
		t.Fatal(err)
	}
	if m.Motor != "conta-livre" || len(m.TrocasConta) != 1 || m.TrocasConta[0].De != "conta-cheia" || m.TrocasConta[0].Para != "conta-livre" || m.TrocasConta[0].Percentual != 91 || m.TrocasConta[0].Limiar != 80 || !strings.Contains(m.TrocasConta[0].Motivo, "91.0%") {
		t.Fatalf("meta: %+v", m)
	}
	achou := false
	for _, e := range eventos {
		if e.Tipo == EvInicio && e.Motor == "conta-livre" && e.TrocaDe == "conta-cheia" && strings.Contains(e.TrocaMotivo, "troca para conta-livre") {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("evento de início sem o motivo da troca: %+v", eventos)
	}
}

func TestRunSemTrocaDeContaSoPulaEFalhaSemReserva(t *testing.T) {
	root, agents := repoFixture(t)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-cheia2", Command: scriptFixo(t, root, "cheia2.sh", "nao deve rodar")}); err != nil {
		t.Fatal(err)
	}
	velho := avaliarCota
	t.Cleanup(func() { avaliarCota = velho })
	avaliarCota = func(context.Context, string, float64, time.Duration, func(string) bool) limites.DecisaoCota {
		return limites.DecisaoCota{Conhecida: true, Percentual: 95, Acima: true, Alternativa: "conta-livre", AlternativaPercentual: 5}
	}
	res, err := Run(context.Background(), root, Options{Name: "sem-troca", Motor: "conta-cheia2", PromptText: "oi", AgentsDir: agents, QuotaMax: 80, SemTrocaConta: true, MaxAgents: 99})
	if err == nil && res.Code == 0 {
		t.Fatalf("esperava falha sem troca; res=%+v", res)
	}
	if strings.Contains(mustRead(t, res.LogFile), "nao deve rodar") {
		t.Fatal("rodou a instância acima do limiar")
	}
}

func TestRunSemDadoFrescoNaoTrocaNemPula(t *testing.T) {
	root, agents := repoFixture(t)
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "conta-sem-dado", Command: scriptFixo(t, root, "semdado.sh", "rodou normal")}); err != nil {
		t.Fatal(err)
	}
	velho := avaliarCota
	t.Cleanup(func() { avaliarCota = velho })
	avaliarCota = func(context.Context, string, float64, time.Duration, func(string) bool) limites.DecisaoCota {
		return limites.DecisaoCota{}
	}
	res, err := Run(context.Background(), root, Options{Name: "sem-dado", Motor: "conta-sem-dado", PromptText: "oi", AgentsDir: agents, QuotaMax: 80, MaxAgents: 99})
	if err != nil || res.Code != 0 || !strings.Contains(mustRead(t, res.LogFile), "rodou normal") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func binarioQueGravaArgs(t *testing.T, nome string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	saida := filepath.Join(t.TempDir(), "args.txt")
	corpo := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + saida + "\nprintf '%s\\n' '{\"type\":\"text\",\"sessionID\":\"ses_x\",\"part\":{\"type\":\"text\",\"text\":\"ok\"}}'\nprintf '%s\\n' '{\"type\":\"step_finish\",\"sessionID\":\"ses_x\",\"part\":{\"type\":\"step-finish\",\"tokens\":{\"total\":1}}}'\n"
	if err := os.WriteFile(filepath.Join(dir, nome), []byte(corpo), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, saida
}

func TestRunRetomaSessaoNativaDoOpencode(t *testing.T) {
	root, agents := repoFixture(t)
	_, saida := binarioQueGravaArgs(t, "opencode")
	if err := harness.RegisterCustom(harness.CustomSpec{Name: "oc-retoma", Base: "opencode"}); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), root, Options{Name: "retoma-oc", Motor: "oc-retoma", PromptText: "continue", SessaoNativa: "ses_antiga123", AgentsDir: agents, MaxAgents: 99})
	if err != nil || res.Code != 0 {
		t.Fatalf("res=%+v err=%v log=%s", res, err, mustRead(t, res.LogFile))
	}
	args := mustRead(t, saida)
	if !strings.Contains(args, "--session\nses_antiga123\n") {
		t.Fatalf("argumentos do opencode: %q", args)
	}
	var m meta
	_ = json.Unmarshal([]byte(mustRead(t, res.MetaFile)), &m)
	if m.SessaoNativa != "ses_antiga123" {
		t.Fatalf("meta: %+v", m)
	}
}

func TestRunRetomarSessaoNaoSuportadaPeloAider(t *testing.T) {
	root, agents := repoFixture(t)
	binarioQueGravaArgs(t, "aider")
	res, err := Run(context.Background(), root, Options{Name: "retoma-aider", Motor: "aider", PromptText: "x", SessaoNativa: "qualquer", AgentsDir: agents, MaxAgents: 99})
	if err == nil || !strings.Contains(err.Error(), "retomada nativa não suportada") {
		t.Fatalf("err=%v res=%+v", err, res)
	}
}
