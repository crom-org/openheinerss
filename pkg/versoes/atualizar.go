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
	ResAtualizado  = "atualizado"
	ResVoltou      = "voltou"       // o teste falhou e a versão anterior foi restaurada e confirmada
	ResVoltaFalhou = "volta_falhou" // o teste falhou e a volta não pôde ser confirmada
	ResFalhou      = "falhou"       // a instalação falhou (versão anterior mantida)
	ResOcupado     = "ocupado"
	ResSemCLI      = "sem_cli"
	ResDesconhec   = "desconhecida"
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
	// SemVolta permite atualizar mesmo quando a volta automática não existe para o método.
	SemVolta bool
	// SimularFalhaTeste faz o teste pós-instalação contar como falho (exercita a volta).
	SimularFalhaTeste bool
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
	Detalhe   string `json:"detalhe,omitempty"`
	TempoMS   int64  `json:"tempo_ms"`
	Simulado  bool   `json:"simulado,omitempty"`
}

// Resultado é a saída de `harness atualizar` e do RPC harness.atualizar; também é o payload do
// evento harness.atualizado.
type Resultado struct {
	Harness    string `json:"harness"`
	CLI        string `json:"cli,omitempty"`
	Instalacao string `json:"instalacao,omitempty"`
	Pacote     string `json:"pacote,omitempty"`
	Antes      string `json:"antes"`
	Disponivel string `json:"disponivel,omitempty"`
	Alvo       string `json:"alvo,omitempty"`
	Depois     string `json:"depois,omitempty"`
	// Tentada é a versão que a instalação chegou a colocar antes de o teste falhar e a volta acontecer.
	Tentada      string   `json:"tentada,omitempty"`
	Resultado    string   `json:"resultado"`
	Motivo       string   `json:"motivo,omitempty"`
	Seco         bool     `json:"seco"`
	Comando      string   `json:"comando,omitempty"`
	Volta        string   `json:"volta,omitempty"` // automatica | manual
	ComandoVolta string   `json:"comandoVolta,omitempty"`
	Passos       []string `json:"passos"`
	Ocupado      []Uso    `json:"ocupado,omitempty"`
	Teste        *Teste   `json:"teste,omitempty"`
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
	r := &Resultado{Harness: base, CLI: sp.cli, Seco: o.Seco, Passos: []string{}, Em: e.Agora().UTC().Format(time.RFC3339)}
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
	if volta == "automatica" {
		r.Passos = append(r.Passos, fmt.Sprintf("guardar a versão anterior (%s); se o teste falhar, voltar", antes))
	} else {
		r.Passos = append(r.Passos, "volta automática indisponível para este método ("+d.metodo+"): sem --sem-volta a atualização é recusada")
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
	if len(r.Ocupado) > 0 {
		if !o.Esperar {
			r.Resultado, r.Motivo = ResOcupado, fmt.Sprintf("%d agente(s)/sessão(ões) usam %s; termine-os ou use --esperar", len(r.Ocupado), base)
			return r, nil
		}
		limite := e.Agora().Add(firstDur(o.Espera, 15*time.Minute))
		for len(r.Ocupado) > 0 {
			if e.Agora().After(limite) {
				r.Resultado, r.Motivo = ResOcupado, "tempo de espera esgotado com processos ainda usando "+base
				return r, nil
			}
			select {
			case <-ctx.Done():
				r.Resultado, r.Motivo = ResOcupado, "cancelado enquanto esperava"
				return r, nil
			default:
			}
			e.Dormir(2 * time.Second)
			r.Ocupado = e.EmUso(base, d.caminho)
		}
		r.Passos = append(r.Passos, "espera concluída: ninguém mais usa o harness")
	}
	if volta != "automatica" && !o.SemVolta {
		r.Resultado, r.Motivo = ResFalhou, "sem volta automática para o método "+d.metodo+"; repita com --sem-volta se aceita o risco"
		return r, nil
	}

	// Instalar.
	ictx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	saida, ierr := e.Executar(ictx, cmd[0], cmd[1:]...)
	cancel()
	depois, verr := e.versaoCLI(ctx, sp.cli)
	if ierr != nil {
		r.Motivo = "instalação falhou: " + ierr.Error() + ": " + trecho(saida)
		if verr == nil && depois == antes {
			r.Resultado, r.Depois = ResFalhou, depois
			r.Passos = append(r.Passos, "a instalação falhou; a versão anterior continua instalada")
			e.registrar(r, o)
			return r, nil
		}
		// Instalação pela metade: trata como falha de teste e tenta voltar.
		return e.voltar(ctx, r, o, sp, d, volt, antes, "instalação falhou e deixou o CLI em estado inesperado"), nil
	}
	if verr != nil {
		return e.voltar(ctx, r, o, sp, d, volt, antes, "o CLI novo não responde a --version: "+verr.Error()), nil
	}
	r.Depois = depois
	if depois == antes && Comparar(antes, alvo) != 0 && alvo != "" {
		r.Resultado = ResFalhou
		r.Motivo = fmt.Sprintf("o comando terminou sem erro, mas o CLI continua em %s (esperado %s): %s", depois, alvo, trecho(saida))
		r.Passos = append(r.Passos, "a versão não mudou; nada a testar nem a voltar")
		e.registrar(r, o)
		return r, nil
	}
	r.Tentada = depois
	r.Passos = append(r.Passos, fmt.Sprintf("instalado: %s → %s", antes, depois))

	// Testar.
	t := e.testar(ctx, sp, inst, o)
	r.Teste = t
	if !t.OK {
		return e.voltar(ctx, r, o, sp, d, volt, antes, "teste falhou: "+t.Detalhe), nil
	}
	r.Resultado = ResAtualizado
	r.Passos = append(r.Passos, "teste OK ("+t.Modo+")")
	e.recalcular(ctx, r, base, sp, depois)
	e.registrar(r, o)
	return r, nil
}

func firstDur(a, b time.Duration) time.Duration {
	if a > 0 {
		return a
	}
	return b
}

func (e Ambiente) testar(ctx context.Context, sp baseSpec, inst string, o Opcoes) *Teste {
	ini := e.Agora()
	t := &Teste{Modo: "fumaca"}
	if inst != "" && e.Testar != nil {
		t.Modo, t.Instancia = "real", inst
		t.OK, t.Detalhe = e.Testar(ctx, inst, firstDur(o.TesteTimeout, 3*time.Minute))
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
	if o.SimularFalhaTeste {
		t.Simulado = true
		t.Detalhe = fmt.Sprintf("falha simulada por --simular-falha-teste (teste verdadeiro: ok=%v)", t.OK)
		t.OK = false
	}
	return t
}

// voltar restaura a versão anterior e confirma com --version.
func (e Ambiente) voltar(ctx context.Context, r *Resultado, o Opcoes, sp baseSpec, d detectado, volt []string, antes, motivo string) *Resultado {
	r.Motivo = motivo
	fim := func(res, passo string) *Resultado {
		r.Resultado = res
		r.Passos = append(r.Passos, passo)
		e.registrar(r, o)
		return r
	}
	var saida string
	var err error
	if len(volt) > 0 {
		c, cancel := context.WithTimeout(ctx, 15*time.Minute)
		saida, err = e.Executar(c, volt[0], volt[1:]...)
		cancel()
		r.Passos = append(r.Passos, "voltar: "+strings.Join(volt, " "))
	} else if dir := ligacaoVersoes(d.caminho); dir != "" {
		err = e.religar(sp, dir, antes)
		r.Passos = append(r.Passos, "voltar: religar "+sp.cli+" à versão "+antes)
	} else {
		return fim(ResVoltaFalhou, "volta automática indisponível: reinstale "+antes+" manualmente")
	}
	v, verr := e.versaoCLI(ctx, sp.cli)
	if err != nil && (verr != nil || v != antes) {
		return fim(ResVoltaFalhou, "a volta falhou: "+err.Error()+": "+trecho(saida))
	}
	if verr != nil || v != antes {
		got := v
		if verr != nil {
			got = verr.Error()
		}
		return fim(ResVoltaFalhou, fmt.Sprintf("a volta não foi confirmada (esperava %s, achei %s)", antes, got))
	}
	r.Depois = v
	return fim(ResVoltou, fmt.Sprintf("volta confirmada: %s --version = %s", sp.cli, v))
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
