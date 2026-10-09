package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParadoPadrao(t *testing.T) {
	_, repo := ambienteContexto(t)
	p, err := ParadoEfetivo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if p.Aviso != 10*time.Minute || p.Parar != 20*time.Minute || p.Acao != ParadoAcaoAviso || p.PodeParar() {
		t.Fatalf("padrão incorreto: %+v", p)
	}
}

func TestParadoProjetoVenceGlobalCampoACampo(t *testing.T) {
	home, repo := ambienteContexto(t)
	gravar(t, filepath.Join(home, WorkspaceDirName, ConfigFileName), "parado:\n  aviso_min: 5\n  parar_min: 30\n  acao: parar\n")
	gravar(t, filepath.Join(repo, WorkspaceDirName, ConfigFileName), "parado:\n  aviso_min: 7\n")
	p, err := ParadoEfetivo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if p.Aviso != 7*time.Minute || p.OrigemAviso != OrigemProjeto {
		t.Fatalf("aviso deveria vir do projeto: %+v", p)
	}
	if p.Parar != 30*time.Minute || p.OrigemParar != OrigemGlobal || p.Acao != ParadoAcaoParar || !p.PodeParar() {
		t.Fatalf("parar/acao deveriam vir do global: %+v", p)
	}
}

func TestParadoEnvFlagsEValidacao(t *testing.T) {
	home, repo := ambienteContexto(t)
	t.Setenv(EnvParadoAvisoMin, "3")
	p, _ := ParadoEfetivo(repo)
	if p.Aviso != 3*time.Minute || p.OrigemAviso != "env" {
		t.Fatalf("env: %+v", p)
	}
	gravar(t, filepath.Join(home, WorkspaceDirName, ConfigFileName), "parado:\n  aviso_min: 5\n")
	p, _ = ParadoEfetivo(repo)
	if p.Aviso != 5*time.Minute {
		t.Fatalf("arquivo vence env: %+v", p)
	}
	zero, vinte := time.Duration(0), 20*time.Minute
	q, err := AplicarFlagsParado(p, &zero, &vinte)
	if err != nil || q.Ligado() || q.OrigemAviso != OrigemFlag {
		t.Fatalf("flag 0 desliga: %+v %v", q, err)
	}
	q, _ = AplicarFlagsParado(p, nil, &vinte)
	if q.Acao != ParadoAcaoParar || !q.PodeParar() {
		t.Fatalf("--parado-parar liga a ação: %+v", q)
	}
	gravar(t, filepath.Join(repo, WorkspaceDirName, ConfigFileName), "parado:\n  acao: matar\n")
	if _, err := ParadoEfetivo(repo); err == nil {
		t.Fatal("acao inválida deveria dar erro")
	}
}
