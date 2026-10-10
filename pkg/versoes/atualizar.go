package versoes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/capacidades"
)

// Resultados de Resultado.Resultado.
const (
	ResSeco        = "seco"
	ResJaNaUltima  = "ja_na_ultima"
	ResJaNaVersao  = "ja_na_versao" // harness voltar: já está na versão pedida
	ResAtualizado  = "atualizado"   // instalou e o teste passou
	ResVoltou      = "voltou"       // harness voltar: versão restaurada, confirmada e testada
	ResTesteFalhou = "teste_falhou" // instalou, mas o teste falhou: veja Diagnostico/Recomendacao
	ResFalhou      = "falhou"       // a instalação (ou a volta) falhou
	ResOcupado     = "ocupado"
	ResSemCLI      = "sem_cli"
	ResDesconhec   = "desconhecida"
)

// Diagnósticos de uma falha de teste (Resultado.Diagnostico).
const (
	DiagVersaoNova = "versao_nova"   // flag/formato mudou, crash, erro de parse: a versão nova é a causa
	DiagExterna    = "externa"       // login vencido, rede, cota/429: não é a versão
	DiagIndefinido = "indeterminado" // sem como separar (e sem como comparar com a anterior)
)

// Recomendações (Resultado.Recomendacao). Quem decide voltar é o orquestrador, com `harness voltar`.
const (
	RecVoltar     = "voltar"
	RecNaoEVersao = "nao_e_a_versao"
	RecAvaliar    = "avaliar"
)

// Falhas simuladas aceitas por --simular-falha-teste.
const (
	SimVersao = "versao"
	SimLogin  = "login"
)

// Opcoes de Atualizar.
type Opcoes struct {
	Seco bool
	// Esperar aguarda os agentes/sessões que usam o harness terminarem (até Espera); sem isso, recusa.
	Esperar bool
	Espera  time.Duration
	// Para instala exatamente esta versão (padrão: a última da fonte oficial).
	Para string
	// Forcar reinstala mesmo já na última.
	Forcar bool
	// SimularFalhaTeste faz o teste pós-instalação contar como falho: "versao" (flag/formato mudou)
	// ou "login" (login vencido); exercita as duas classificações.
	SimularFalhaTeste string
	// TesteTimeout limita o harness test (padrão 3 min).
	TesteTimeout time.Duration
	// EventLog é o arquivo de log de eventos (eventos_log); vazio não grava linha.
	EventLog string
}

// Uso é um agente `rodar`, sessão `serve` ou outro processo que está usando o harness.
type Uso struct {
	PID     int    `json:"pid"`
	Tipo    string `json:"tipo"` // rodar | serve | processo
	Comando string `json:"comando"`
}

// Teste descreve o teste pós-instalação.
type Teste struct {
	Modo      string `json:"modo"` // real (harness test da instância) | fumaca (--version + --help)
	Instancia string `json:"instancia,omitempty"`
	OK        bool   `json:"ok"`
	// Detalhe é a saída do teste (sem segredos).
	Detalhe  string `json:"detalhe,omitempty"`
	TempoMS  int64  `json:"tempo_ms"`
	Simulado bool   `json:"simulado,omitempty"`
}

// Comparacao é o mesmo teste rodado na versão anterior, quando a falha não se separava pela saída.
type Comparacao struct {
	Versao  string `json:"versao"`
	OK      bool   `json:"ok"`
	Detalhe string `json:"detalhe,omitempty"`
}

// Resultado é a saída de `harness atualizar`/`harness voltar` e dos RPCs; também é o payload do
// evento harness.atualizado. O openheinerss só executa e informa: nunca volta sozinho.
type Resultado struct {
	Harness    string `json:"harness"`
	Acao       string `json:"acao"` // atualizar | voltar
	CLI        string `json:"cli,omitempty"`
	Instalacao string `json:"instalacao,omitempty"`
	Pacote     string `json:"pacote,omitempty"`
	Antes      string `json:"antes"`
	Disponivel string `json:"disponivel,omitempty"`
	Alvo       string `json:"alvo,omitempty"`
	Depois     string `json:"depois,omitempty"`
	// Anterior é a versão guardada para `harness voltar` (a que estava antes da última troca).
	Anterior  string `json:"anterior,omitempty"`
	Resultado string `json:"resultado"`
	Motivo    string `json:"motivo,omitempty"`
	// Diagnostico/Recomendacao só existem quando o teste falhou.
	Diagnostico  string      `json:"diagnostico,omitempty"`
	Recomendacao string      `json:"recomendacao,omitempty"`
	Comparacao   *Comparacao `json:"comparacao,omitempty"`
	Seco         bool        `json:"seco"`
	Comando      string      `json:"comando,omitempty"`
	Volta        string      `json:"volta,omitempty"` // como voltar: automatica | manual
	ComandoVolta string      `json:"comandoVolta,omitempty"`
	Passos       []string    `json:"passos"`
	Ocupado      []Uso       `json:"ocupado,omitempty"`
	Teste        *Teste      `json:"teste,omitempty"`
	// AjudaMudou: a ajuda do CLI mudou e o cache de `capacidades` foi recalculado.
	AjudaMudou bool   `json:"ajudaMudou,omitempty"`
	Em         string `json:"em"`
}

// comandos monta o comando de atualização e o de volta (nil quando não há volta automática).
// alvo vazio = a última (ultima, se conhecida, é a versão exata a fixar no npm); anterior vazio = ainda não conhecida (prévia).
func (e Ambiente) comandos(d detectado, sp baseSpec, alvo, ultima, anterior string) (atualizar, voltar []string, volta string, err error) {
	pkgVer := func(sep, v string) string {
		if v == "" {
			return d.pacote
		}
		return d.pacote + sep + v
	}
	volta = "automatica"
	switch d.metodo {
	case InstNPM:
		if d.pacote == "" {
			return nil, nil, "manual", errors.New("pacote npm não identificado")
		}
		atualizar = []string{"npm", "install", "-g", pkgVer("@", firstOr(alvo, firstOr(ultima, "latest")))}
		if anterior != "" {
			voltar = []string{"npm", "install", "-g", pkgVer("@", anterior)}
		}
	case InstUV:
		py := []string{}
		if d.python != "" && existe(d.python) {
			py = []string{"--python", d.python}
		}
		if alvo == "" {
			// "uv tool upgrade" respeita a versão fixada no recibo (deixada por --para ou por uma volta);
			// reinstalar sem versão sempre pega a última.
			atualizar = append([]string{"uv", "tool", "install", "--force"}, append(py, d.pacote)...)
		} else {
			atualizar = append([]string{"uv", "tool", "install", "--force"}, append(py, pkgVer("==", alvo))...)
		}
		if anterior != "" {
			voltar = append([]string{"uv", "tool", "install", "--force"}, append(py, pkgVer("==", anterior))...)
		}
	case InstPipx:
		if alvo == "" {
			atualizar = []string{"pipx", "install", "--force", d.pacote}
		} else {
			atualizar = []string{"pipx", "install", "--force", pkgVer("==", alvo)}
		}
		if anterior != "" {
			voltar = []string{"pipx", "install", "--force", pkgVer("==", anterior)}
		}
	case InstPip:
		py := firstOr(d.python, "python3")
		if alvo == "" {
			atualizar = []string{py, "-m", "pip", "install", "--user", "--upgrade", d.pacote}
		} else {
			atualizar = []string{py, "-m", "pip", "install", "--user", pkgVer("==", alvo)}
		}
		if anterior != "" {
			voltar = []string{py, "-m", "pip", "install", "--user", pkgVer("==", anterior)}
		}
	case InstBrew:
		if alvo != "" {
			return nil, nil, "manual", errors.New("brew não instala versão específica (--para)")
		}
		atualizar = []string{"brew", "upgrade", d.pacote}
		volta = "manual"
	default: // script/binário: autoatualização do próprio CLI
		if sp.atualiza == "" {
			return nil, nil, "manual", errors.New("o CLI não tem comando de autoatualização")
		}
		atualizar = []string{sp.cli, sp.atualiza}
		if alvo != "" {
			if !sp.aceitaAlvo {
				return nil, nil, "manual", fmt.Errorf("%s %s não aceita versão específica (--para)", sp.cli, sp.atualiza)
			}
			atualizar = append(atualizar, alvo)
		}
		switch {
		case sp.aceitaAlvo:
			if anterior != "" {
				voltar = []string{sp.cli, sp.atualiza, anterior}
			}
		case ligacaoVersoes(d.caminho) != "":
			volta = "automatica"
		default:
			volta = "manual"
		}
	}
	return atualizar, voltar, volta, nil
}

func firstOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }

// ligacaoVersoes devolve a pasta que guarda as versões lado a lado (instalador nativo do claude:
// .../claude/versions/<v>) quando o executável vive nela; "" caso contrário.
func ligacaoVersoes(real string) string {
	if filepath.Base(filepath.Dir(real)) == "versions" {
		return filepath.Dir(real)
	}
	return ""
}

func trecho(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = "…" + s[len(s)-400:]
	}
	return s
}

// Atualizar executa o fluxo completo para uma base. Nunca devolve erro Go para falhas do
// fluxo: elas ficam em Resultado.Resultado/Motivo (o erro é só para argumento inválido).
func (e Ambiente) Atualizar(ctx context.Context, base string, o Opcoes) (*Resultado, error) {
	sp, ok := specs[base]
	if !ok {
		return nil, fmt.Errorf("harness %q não é uma base embutida (use: %s)", base, strings.Join(Bases(), ", "))
	}
	r := &Resultado{Harness: base, Acao: "atualizar", CLI: sp.cli, Seco: o.Seco, Passos: []string{}, Em: e.Agora().UTC().Format(time.RFC3339)}
	d, err := e.detectar(base)
	if err != nil {
		r.Resultado, r.Motivo = ResSemCLI, sp.cli+" não está no PATH"
		return r, nil
	}
	r.Instalacao, r.Pacote = d.metodo, d.pacote
	antes, err := e.versaoCLI(ctx, sp.cli)
	if err != nil {
		r.Resultado, r.Motivo = ResDesconhec, err.Error()
		return r, nil
	}
	r.Antes = antes
	r.Alvo = o.Para
	disp, fonte, uerr := e.Ultima(ctx, base, d.metodo, d.pacote)
	if uerr == nil {
		r.Disponivel = disp
		r.Passos = append(r.Passos, fmt.Sprintf("última versão (%s): %s", fonte, disp))
	} else if o.Para == "" {
		// Sem a última conhecida: a autoatualização do CLI/gerenciador ainda pode decidir sozinha,
		// mas sem rede não há o que prometer; o comando que não pinou versão roda mesmo assim.
		r.Passos = append(r.Passos, "última versão desconhecida: "+uerr.Error())
	}
	alvo := o.Para
	pinar := alvo != ""
	if alvo == "" {
		alvo = r.Disponivel
	}
	if alvo != "" && Comparar(antes, alvo) == 0 && !o.Forcar {
		r.Resultado = ResJaNaUltima
		r.Depois = antes
		r.Passos = append(r.Passos, fmt.Sprintf("já está em %s", antes))
		return r, nil
	}
	if alvo == "" && uerr != nil {
		r.Resultado, r.Motivo = ResDesconhec, "última versão desconhecida e nenhuma versão pedida (--para): "+uerr.Error()
		return r, nil
	}
	pin := ""
	if pinar {
		pin = alvo
	}
	cmd, volt, volta, cerr := e.comandos(d, sp, pin, r.Disponivel, antes)
	if cerr != nil {
		r.Resultado, r.Motivo = ResDesconhec, cerr.Error()
		return r, nil
	}
	r.Volta = volta
	r.Comando = strings.Join(cmd, " ")
	r.ComandoVolta = strings.Join(volt, " ")
	r.Passos = append(r.Passos, "atualizar: "+r.Comando)
	if g, ok := e.anteriorGuardada(base); ok {
		r.Anterior = g
	}
	r.Passos = append(r.Passos, fmt.Sprintf("guardar a versão anterior (%s) para `harness voltar %s`", antes, base))
	if volta != "automatica" {
		r.Passos = append(r.Passos, "voltar neste método ("+d.metodo+") é manual: reinstale a versão anterior com o gerenciador")
	}
	inst := ""
	if e.InstanciaGratis != nil {
		inst = e.InstanciaGratis(base)
	}
	if inst != "" {
		r.Passos = append(r.Passos, "testar com `harness test "+inst+"` (instância grátis)")
	} else {
		r.Passos = append(r.Passos, "testar com "+sp.cli+" --version e --help (sem instância grátis para a base)")
	}

	r.Ocupado = e.EmUso(base, d.caminho)
	if o.Seco {
		r.Resultado = ResSeco
		if len(r.Ocupado) > 0 {
			r.Motivo = fmt.Sprintf("%d processo(s) usam %s agora; a atualização esperaria (--esperar) ou seria recusada", len(r.Ocupado), base)
		}
		return r, nil
	}
	if !e.aguardarLivre(ctx, r, base, d.caminho, o.Esperar, o.Espera) {
		return r, nil
	}

	// Instalar.
	ictx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	saida, ierr := e.Executar(ictx, cmd[0], cmd[1:]...)
	cancel()
	depois, verr := e.versaoCLI(ctx, sp.cli)
	if ierr != nil {
		r.Motivo = "instalação falhou: " + ierr.Error() + ": " + semSegredos(trecho(saida))
		r.Resultado = ResFalhou
		if verr == nil && depois == antes {
			r.Depois = depois
			r.Passos = append(r.Passos, "a instalação falhou; a versão anterior continua instalada")
		} else {
			// Instalação pela metade: informa; quem decide voltar é o orquestrador.
			r.Depois = depois
			r.Diagnostico, r.Recomendacao = DiagVersaoNova, RecVoltar
			e.guardarAnterior(base, antes, depois)
			r.Anterior = antes
			r.Passos = append(r.Passos, "a instalação falhou e deixou o CLI em estado inesperado; versão anterior guardada: "+antes)
		}
		e.registrar(r, o)
		return r, nil
	}
	if verr != nil {
		r.Resultado = ResTesteFalhou
		r.Motivo = "o CLI novo não responde a --version: " + verr.Error()
		r.Teste = &Teste{Modo: "fumaca", Detalhe: semSegredos(r.Motivo)}
		r.Diagnostico, r.Recomendacao = DiagVersaoNova, RecVoltar
		e.guardarAnterior(base, antes, "")
		r.Anterior = antes
		e.registrar(r, o)
		return r, nil
	}
	r.Depois = depois
	if depois == antes && Comparar(antes, alvo) != 0 && alvo != "" {
		r.Resultado = ResFalhou
		r.Motivo = fmt.Sprintf("o comando terminou sem erro, mas o CLI continua em %s (esperado %s): %s", depois, alvo, semSegredos(trecho(saida)))
		r.Passos = append(r.Passos, "a versão não mudou; nada a testar")
		e.registrar(r, o)
		return r, nil
	}
	if depois != antes {
		e.guardarAnterior(base, antes, depois)
		r.Anterior = antes
	}
	r.Passos = append(r.Passos, fmt.Sprintf("instalado: %s → %s", antes, depois))

	// Testar e, se falhar, classificar (sem voltar).
	t := e.testar(ctx, sp, inst, o.SimularFalhaTeste, o.TesteTimeout)
	r.Teste = t
	if !t.OK {
		r.Resultado = ResTesteFalhou
		r.Motivo = "teste falhou: " + t.Detalhe
		e.diagnosticar(ctx, r, base, sp, d, inst, o, cmd, volt, antes, depois)
		e.registrar(r, o)
		return r, nil
	}
	r.Resultado = ResAtualizado
	r.Passos = append(r.Passos, "teste OK ("+t.Modo+")")
	e.recalcular(ctx, r, base, sp, depois)
	e.registrar(r, o)
	return r, nil
}

// aguardarLivre recusa (ou espera, com esperar) enquanto algum agente/sessão usa o harness.
// Devolve false com r já preenchido quando não pode seguir.
func (e Ambiente) aguardarLivre(ctx context.Context, r *Resultado, base, caminho string, esperar bool, espera time.Duration) bool {
	r.Ocupado = e.EmUso(base, caminho)
	if len(r.Ocupado) == 0 {
		return true
	}
	if !esperar {
		r.Resultado, r.Motivo = ResOcupado, fmt.Sprintf("%d agente(s)/sessão(ões) usam %s; termine-os ou use --esperar", len(r.Ocupado), base)
		return false
	}
	limite := e.Agora().Add(firstDur(espera, 15*time.Minute))
	for len(r.Ocupado) > 0 {
		if e.Agora().After(limite) {
			r.Resultado, r.Motivo = ResOcupado, "tempo de espera esgotado com processos ainda usando "+base
			return false
		}
		select {
		case <-ctx.Done():
			r.Resultado, r.Motivo = ResOcupado, "cancelado enquanto esperava"
			return false
		default:
		}
		e.Dormir(2 * time.Second)
		r.Ocupado = e.EmUso(base, caminho)
	}
	r.Passos = append(r.Passos, "espera concluída: ninguém mais usa o harness")
	return true
}

func firstDur(a, b time.Duration) time.Duration {
	if a > 0 {
		return a
	}
	return b
}

func (e Ambiente) testar(ctx context.Context, sp baseSpec, inst, simular string, timeout time.Duration) *Teste {
	ini := e.Agora()
	t := &Teste{Modo: "fumaca"}
	if inst != "" && e.Testar != nil {
		t.Modo, t.Instancia = "real", inst
		t.OK, t.Detalhe = e.Testar(ctx, inst, firstDur(timeout, 3*time.Minute))
	} else {
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		v, err := e.Executar(c, sp.cli, "--version")
		h, err2 := e.Executar(c, sp.cli, "--help")
		switch {
		case err != nil:
			t.Detalhe = "--version: " + err.Error()
		case ExtrairVersao(v) == "":
			t.Detalhe = "--version sem versão"
		case err2 != nil:
			t.Detalhe = "--help: " + err2.Error()
		case strings.TrimSpace(h) == "":
			t.Detalhe = "--help vazio"
		default:
			t.OK, t.Detalhe = true, "--version e --help responderam"
		}
	}
	t.TempoMS = e.Agora().Sub(ini).Milliseconds()
	if simular != "" {
		t.Simulado = true
		verdadeiro := t.OK
		t.OK = false
		switch simular {
		case SimLogin:
			t.Detalhe = fmt.Sprintf("falha simulada (login): not authenticated, please login again [401 unauthorized] (teste verdadeiro: ok=%v)", verdadeiro)
		default:
			t.Detalhe = fmt.Sprintf("falha simulada (versão): error: unknown flag --output-format: formato mudou na versão nova (teste verdadeiro: ok=%v)", verdadeiro)
		}
	}
	t.Detalhe = semSegredos(trecho(t.Detalhe))
	return t
}

var (
	padroesExterna = []string{
		"login", "auth", "unauthorized", "401", "403", "não autentic", "not authenticated", "api key", "api_key", "token expired", "expired",
		"quota", "cota", "rate limit", "rate_limit", "429", "too many requests", "more credits", "insufficient_quota", "usage limit", "billing",
		"no such host", "dial tcp", "connection refused", "connection reset", "econn", "enotfound", "network", "timed out", "timeout",
		"unreachable", "temporary failure in name resolution", "certificate", "502", "503", "overloaded", "sem modelo", "no llm model",
	}
	padroesVersao = []string{
		"unknown flag", "unknown option", "unrecognized arguments", "unrecognized option", "no such option", "unexpected argument",
		"invalid flag", "flag provided but not defined", "panic", "segmentation fault", "traceback", "stack trace", "sigsegv", "fatal error",
		"parse", "unexpected token", "invalid json", "cannot unmarshal", "formato", "schema", "deprecated", "removed", "no longer supported",
		"illegal instruction", "cannot find module", "modulenotfounderror", "importerror", "syntaxerror",
	}
)

// Classificar separa a falha de um teste pela saída: versão nova (flag/formato mudou, crash,
// parse), externa (login, rede, cota) ou indeterminado. A fumaça (--version/--help) é só do CLI,
// então falhar nela é sempre a versão.
func Classificar(t *Teste) string {
	if t == nil || t.OK {
		return ""
	}
	l := strings.ToLower(t.Detalhe)
	tem := func(ps []string) bool {
		for _, p := range ps {
			if strings.Contains(l, p) {
				return true
			}
		}
		return false
	}
	switch {
	case t.Modo == "fumaca":
		return DiagVersaoNova
	case tem(padroesExterna):
		return DiagExterna
	case tem(padroesVersao):
		return DiagVersaoNova
	}
	return DiagIndefinido
}

// diagnosticar preenche Diagnostico/Recomendacao. Em dúvida roda o mesmo teste na versão
// anterior (reinstalando-a e depois reinstalando a nova, para o estado final ser o da atualização).
func (e Ambiente) diagnosticar(ctx context.Context, r *Resultado, base string, sp baseSpec, d detectado, inst string, o Opcoes, cmd, volt []string, antes, depois string) {
	r.Diagnostico = Classificar(r.Teste)
	if r.Diagnostico == DiagIndefinido {
		if cmp := e.compararComAnterior(ctx, r, sp, d, inst, o, cmd, volt, antes, depois); cmp != nil {
			r.Comparacao = cmp
			if cmp.OK {
				r.Diagnostico = DiagVersaoNova
				r.Passos = append(r.Passos, fmt.Sprintf("o mesmo teste passou na %s: a falha é da versão nova", antes))
			} else {
				r.Diagnostico = DiagExterna
				r.Passos = append(r.Passos, fmt.Sprintf("o mesmo teste também falhou na %s: a causa não é a versão", antes))
			}
		}
	}
	switch r.Diagnostico {
	case DiagVersaoNova:
		r.Recomendacao = RecVoltar
		r.Passos = append(r.Passos, fmt.Sprintf("diagnóstico: versão nova; recomendado voltar (`openheinerss harness voltar %s`, anterior %s)", base, antes))
	case DiagExterna:
		r.Recomendacao = RecNaoEVersao
		r.Passos = append(r.Passos, "diagnóstico: falhou por login/rede/cota, não pela versão; não é preciso voltar")
	default:
		r.Recomendacao = RecAvaliar
		r.Passos = append(r.Passos, "diagnóstico: indeterminado (sem como comparar com a anterior); avalie a saída do teste antes de voltar")
	}
}

func (e Ambiente) compararComAnterior(ctx context.Context, r *Resultado, sp baseSpec, d detectado, inst string, o Opcoes, cmd, volt []string, antes, depois string) *Comparacao {
	if len(volt) == 0 {
		r.Passos = append(r.Passos, "comparar com a anterior não é possível neste método ("+d.metodo+")")
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 15*time.Minute)
	_, err := e.Executar(c, volt[0], volt[1:]...)
	cancel()
	if v, verr := e.versaoCLI(ctx, sp.cli); err != nil || verr != nil || v != antes {
		r.Passos = append(r.Passos, "comparar com a anterior: não consegui instalar "+antes)
		e.restaurarNova(ctx, r, sp, cmd, depois)
		return nil
	}
	r.Passos = append(r.Passos, "comparação: instalada a "+antes+" só para repetir o teste")
	t := e.testar(ctx, sp, inst, "", o.TesteTimeout)
	e.restaurarNova(ctx, r, sp, cmd, depois)
	return &Comparacao{Versao: antes, OK: t.OK, Detalhe: t.Detalhe}
}

// restaurarNova reinstala a versão nova depois da comparação, para o estado final ser o da atualização.
func (e Ambiente) restaurarNova(ctx context.Context, r *Resultado, sp baseSpec, cmd []string, depois string) {
	c, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	_, err := e.Executar(c, cmd[0], cmd[1:]...)
	v, verr := e.versaoCLI(ctx, sp.cli)
	if err != nil || verr != nil || v != depois {
		r.Passos = append(r.Passos, fmt.Sprintf("AVISO: não consegui reinstalar a %s depois da comparação; o CLI está em %s", depois, v))
		r.Depois = v
		return
	}
	r.Passos = append(r.Passos, "versão nova reinstalada depois da comparação: "+depois)
}

// Voltar instala a versão guardada (ou a pedida em o.Para), confirma, testa e informa. É sempre
// uma ação explícita de quem chama: o openheinerss nunca volta sozinho.
func (e Ambiente) Voltar(ctx context.Context, base string, o Opcoes) (*Resultado, error) {
	sp, ok := specs[base]
	if !ok {
		return nil, fmt.Errorf("harness %q não é uma base embutida (use: %s)", base, strings.Join(Bases(), ", "))
	}
	r := &Resultado{Harness: base, Acao: "voltar", CLI: sp.cli, Seco: o.Seco, Passos: []string{}, Em: e.Agora().UTC().Format(time.RFC3339)}
	d, err := e.detectar(base)
	if err != nil {
		r.Resultado, r.Motivo = ResSemCLI, sp.cli+" não está no PATH"
		return r, nil
	}
	r.Instalacao, r.Pacote = d.metodo, d.pacote
	atual, err := e.versaoCLI(ctx, sp.cli)
	if err != nil {
		r.Resultado, r.Motivo = ResDesconhec, err.Error()
		return r, nil
	}
	r.Antes = atual
	alvo := o.Para
	if alvo == "" {
		g, ok := e.anteriorGuardada(base)
		if !ok {
			r.Resultado, r.Motivo = ResDesconhec, "não há versão anterior guardada para "+base+"; informe a versão: harness voltar "+base+" <versão>"
			return r, nil
		}
		alvo = g
	}
	r.Alvo = alvo
	if Comparar(atual, alvo) == 0 {
		r.Resultado, r.Depois = ResJaNaVersao, atual
		r.Passos = append(r.Passos, "já está em "+atual)
		return r, nil
	}
	cmd, _, _, cerr := e.comandos(d, sp, alvo, "", "")
	relink := ""
	if cerr != nil {
		if dir := ligacaoVersoes(d.caminho); dir != "" {
			relink, cerr = dir, nil
		}
	}
	if cerr != nil {
		r.Resultado, r.Motivo = ResDesconhec, "voltar é manual neste método ("+d.metodo+"): "+cerr.Error()
		return r, nil
	}
	if relink != "" {
		r.Comando = "religar " + sp.cli + " → " + filepath.Join(relink, alvo)
	} else {
		r.Comando = strings.Join(cmd, " ")
	}
	r.Passos = append(r.Passos, "voltar: "+r.Comando)
	inst := ""
	if e.InstanciaGratis != nil {
		inst = e.InstanciaGratis(base)
	}
	r.Ocupado = e.EmUso(base, d.caminho)
	if o.Seco {
		r.Resultado = ResSeco
		if len(r.Ocupado) > 0 {
			r.Motivo = fmt.Sprintf("%d processo(s) usam %s agora; a volta esperaria (--esperar) ou seria recusada", len(r.Ocupado), base)
		}
		return r, nil
	}
	if !e.aguardarLivre(ctx, r, base, d.caminho, o.Esperar, o.Espera) {
		return r, nil
	}
	var saida string
	var ierr error
	if relink != "" {
		ierr = e.religar(sp, relink, alvo)
	} else {
		c, cancel := context.WithTimeout(ctx, 15*time.Minute)
		saida, ierr = e.Executar(c, cmd[0], cmd[1:]...)
		cancel()
	}
	v, verr := e.versaoCLI(ctx, sp.cli)
	r.Depois = v
	switch {
	case ierr != nil && (verr != nil || v != alvo):
		r.Resultado, r.Motivo = ResFalhou, "a volta falhou: "+ierr.Error()+": "+semSegredos(trecho(saida))
		e.registrar(r, o)
		return r, nil
	case verr != nil || v != alvo:
		r.Resultado, r.Motivo = ResFalhou, fmt.Sprintf("a volta não foi confirmada (esperava %s, achei %q)", alvo, v)
		e.registrar(r, o)
		return r, nil
	}
	// A versão que se deixou passa a ser a "anterior": voltar de novo desfaz esta volta.
	e.guardarAnterior(base, atual, v)
	r.Anterior = atual
	r.Passos = append(r.Passos, fmt.Sprintf("volta confirmada: %s --version = %s (anterior guardada: %s)", sp.cli, v, atual))
	t := e.testar(ctx, sp, inst, "", o.TesteTimeout)
	r.Teste = t
	if !t.OK {
		r.Resultado = ResTesteFalhou
		r.Motivo = "teste falhou na versão " + v + ": " + t.Detalhe
		r.Diagnostico = Classificar(t)
		switch r.Diagnostico {
		case DiagExterna:
			r.Recomendacao = RecNaoEVersao
		case DiagVersaoNova:
			r.Recomendacao = RecVoltar
		default:
			r.Recomendacao = RecAvaliar
		}
		e.registrar(r, o)
		return r, nil
	}
	r.Resultado = ResVoltou
	r.Passos = append(r.Passos, "teste OK ("+t.Modo+")")
	e.recalcular(ctx, r, base, sp, v)
	e.registrar(r, o)
	return r, nil
}

// religar aponta o link do CLI (instalador nativo) para a versão guardada ao lado da atual.
func (e Ambiente) religar(sp baseSpec, dir, versao string) error {
	alvo := filepath.Join(dir, versao)
	if !existe(alvo) {
		return fmt.Errorf("%s não existe", alvo)
	}
	p, err := e.LookPath(sp.cli)
	if err != nil {
		return err
	}
	st, err := os.Lstat(p)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s não é um link: não dá para religar", p)
	}
	tmp := p + ".novo"
	_ = os.Remove(tmp)
	if err := os.Symlink(alvo, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// recalcular atualiza o cache de capacidades com a ajuda atual do CLI.
func (e Ambiente) recalcular(ctx context.Context, r *Resultado, base string, sp baseSpec, versao string) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ajuda, _ := e.Executar(c, sp.cli, "--help")
	if strings.TrimSpace(ajuda) == "" {
		return
	}
	_, tinha := capacidades.DoCache(base)
	mudou, err := capacidades.RegistrarAjuda(base, versao, ajuda, e.Agora())
	if err != nil {
		r.Passos = append(r.Passos, "cache de capacidades não gravado: "+err.Error())
		return
	}
	r.AjudaMudou = mudou
	if mudou {
		r.Passos = append(r.Passos, "a ajuda do CLI mudou: cache de `capacidades "+base+"` recalculado (reconfira as células)")
	} else if !tinha {
		r.Passos = append(r.Passos, "ajuda do CLI registrada pela primeira vez no cache de `capacidades`")
	} else {
		r.Passos = append(r.Passos, "a ajuda do CLI não mudou: `capacidades` continua válida")
	}
}

// ArquivoHistorico é onde cada atualização fica registrada (uma linha JSON por evento).
func ArquivoHistorico() (string, error) {
	p, err := capacidades.ArquivoCache()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "harness-atualizacoes.jsonl"), nil
}
