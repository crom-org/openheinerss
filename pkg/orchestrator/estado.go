package orchestrator

// Arquivo de estado neutro: logs/<nome>.estado.md. O rodar o reescreve no fim de cada turno/tentativa, sem
// custo de modelo (só git e o log local), e o manda no lugar do texto fixo `continuation` quando o agente é
// retomado, troca de motor ou recomeça em sessão nova por passar do limite de contexto. É texto puro, então
// serve a qualquer harness.
//
// Conteúdo: prompt original (cortado), objetivo, HEAD, `git diff --stat` contra a base da worktree, os
// últimos eventos de texto do log e, se estiver em dia, o "resumo do agente" (RELATORIO-AGENTE.md).
// O git manda; o resumo é só contexto. Regra do resumo velho: ele é descartado quando o HEAD mudou depois
// dele (commits mais novos que o relatório) ou quando está mais de OPENHEINERSS_RESUMO_MAX_TURNOS turnos
// atrasado (padrão 2); no lugar entra o histórico (diff --stat + últimos eventos + prompt original).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/comandos"
)

const (
	// EnvResumoMaxTurnos é a variável que muda quantos turnos de atraso um resumo aguenta.
	EnvResumoMaxTurnos = "OPENHEINERSS_RESUMO_MAX_TURNOS"

	resumoMaxTurnosPadrao = 2
	estadoPromptMax       = 4000
	estadoRelatorioMax    = 4000
	estadoEventosN        = 6
	estadoEventoMax       = 600
	estadoStatMax         = 40
	estadoLeituraLog      = 128 << 10
)

// estadoCab é o cabeçalho do arquivo (guarda o que a próxima gravação precisa lembrar).
type estadoCab struct {
	Em          string
	Turno       int
	Head        string
	HeadResumo  string
	ResumoMtime string // RFC3339 do RELATORIO-AGENTE.md quando foi lido pela primeira vez assim
	TurnoResumo int    // turno em que o relatório mudou pela última vez
}

// estadoAgente monta e grava o estado de um agente.
type estadoAgente struct {
	nome, work, repo, base, logPath, path, prompt string
	now                                           func() time.Time
	cab                                           estadoCab
}

func novoEstadoAgente(logsDir, nome, work, repo, branchBase, prompt string, now func() time.Time) *estadoAgente {
	envio, _ := separarRegras(prompt)
	e := &estadoAgente{nome: nome, work: work, repo: repo, base: branchBase, prompt: envio, now: now,
		logPath: filepath.Join(logsDir, nome+".log"), path: filepath.Join(logsDir, nome+".estado.md")}
	e.cab = lerCabEstado(e.path)
	return e
}

func resumoMaxTurnos() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvResumoMaxTurnos))); err == nil && n >= 0 {
		return n
	}
	return resumoMaxTurnosPadrao
}

// FimDeTurno regrava o estado depois de um turno/tentativa (conta mais um turno).
func (e *estadoAgente) FimDeTurno() {
	if e == nil {
		return
	}
	e.cab.Turno++
	if txt := e.montar(); txt != "" {
		_ = os.WriteFile(e.path, []byte(txt), 0644)
	}
}

// Continuacao devolve o texto a mandar ao motor ao retomar: aviso curto + estado atual. Vazio quando não há
// estado nenhum (sem git e sem log); aí quem chama usa o `continuation` fixo.
func (e *estadoAgente) Continuacao() string {
	if e == nil {
		return ""
	}
	txt := e.montar()
	if txt == "" {
		return ""
	}
	_ = os.WriteFile(e.path, []byte(txt), 0644)
	return "\n\n--- CONTINUAÇÃO ---\nUma execução anterior desta MESMA tarefa foi interrompida (erro, cota ou limite de contexto). NÃO recomece do zero. Abaixo está o estado gravado pelo openheinerss. O git (HEAD e diff) é a fonte da verdade; o resumo do agente, quando existe, é só contexto e pode estar desatualizado: confira com `git status` e `git log --oneline -10` e termine SOMENTE o que falta, depois finalize como a tarefa pede.\n\n" + txt
}

// montar atualiza o cabeçalho e devolve o texto do estado ("" sem git e sem log).
func (e *estadoAgente) montar() string {
	emGit := e.work != "" && ehRepositorioGit(e.work)
	head, stat, status := "", "", ""
	if emGit {
		head = headDe(e.work)
		stat, status = e.mudancas()
	}
	eventos := ultimosTextos(e.logPath, estadoEventosN)
	if !emGit && len(eventos) == 0 {
		return ""
	}
	agora := e.now()
	rel, relMtime := lerRelatorio(e.work)
	velho := ""
	if rel != "" {
		if mt := relMtime.Format(time.RFC3339); mt != e.cab.ResumoMtime {
			// Relatório novo ou mudado: este é o turno e o HEAD dele.
			e.cab.ResumoMtime, e.cab.HeadResumo, e.cab.TurnoResumo = mt, head, e.cab.Turno
		}
		velho = e.resumoVelho(head, relMtime, emGit)
	}
	e.cab.Em, e.cab.Head = agora.Format(time.RFC3339), head

	var b strings.Builder
	fmt.Fprintf(&b, "---\nem: %s\nturno: %d\nhead: %s\nhead_resumo: %s\nresumo_mtime: %s\nturno_resumo: %d\n---\n", e.cab.Em, e.cab.Turno, head, e.cab.HeadResumo, e.cab.ResumoMtime, e.cab.TurnoResumo)
	fmt.Fprintf(&b, "# Estado do agente %s\n\n## Objetivo\n%s\n", e.nome, mascarar(objetivo(e.prompt)))
	if head != "" {
		fmt.Fprintf(&b, "\n## Git\nHEAD: %s\n", head)
		if stat != "" {
			fmt.Fprintf(&b, "\n`git diff --stat` contra a base da worktree:\n```\n%s\n```\n", mascarar(stat))
		}
		if status != "" {
			fmt.Fprintf(&b, "\nSem commit (`git status --short`):\n```\n%s\n```\n", mascarar(status))
		}
	}
	if len(eventos) > 0 {
		fmt.Fprintf(&b, "\n## Últimos %d trechos de texto do agente\n", len(eventos))
		for _, ev := range eventos {
			fmt.Fprintf(&b, "- %s\n", mascarar(ev))
		}
	}
	switch {
	case rel == "":
	case velho == "":
		fmt.Fprintf(&b, "\n## Resumo do agente (RELATORIO-AGENTE.md, contexto: o git manda)\n%s\n", mascarar(cortarFim(rel, estadoRelatorioMax)))
	default:
		fmt.Fprintf(&b, "\n## Resumo do agente descartado\n%s. Segue o histórico (diff, eventos e prompt original).\n", velho)
	}
	fmt.Fprintf(&b, "\n## Prompt original (cortado)\n%s\n", mascarar(cortarFim(e.prompt, estadoPromptMax)))
	return b.String()
}

// resumoVelho devolve o motivo de descartar o resumo, ou "" se ele está em dia.
func (e *estadoAgente) resumoVelho(head string, mtime time.Time, emGit bool) string {
	if atraso := e.cab.Turno - e.cab.TurnoResumo; atraso > resumoMaxTurnos() {
		return fmt.Sprintf("O resumo está %d turnos atrasado (máximo %d)", atraso, resumoMaxTurnos())
	}
	if !emGit {
		return ""
	}
	if head != "" && e.cab.HeadResumo != "" && head != e.cab.HeadResumo {
		return "O HEAD mudou depois do resumo"
	}
	// Commits mais novos que o relatório (fora o próprio relatório) também o tornam velho.
	out, err := gitSaida(e.work, nil, "log", "--format=%H", "--since="+strconv.FormatInt(mtime.Unix()+1, 10)+" +0000", "--", ".", ":(exclude)RELATORIO-AGENTE.md")
	if err == nil && strings.TrimSpace(out) != "" {
		return "Há commits mais novos que o resumo"
	}
	return ""
}

// mudancas devolve o diff --stat contra a base e o status curto da worktree.
func (e *estadoAgente) mudancas() (stat, status string) {
	ref := "HEAD"
	if b := e.mergeBase(); b != "" {
		ref = b
	}
	stat, _ = gitSaida(e.work, nil, "diff", "--stat", ref)
	status, _ = gitSaida(e.work, nil, "status", "--short")
	return limitarLinhas(stat, estadoStatMax), limitarLinhas(status, estadoStatMax)
}

func (e *estadoAgente) mergeBase() string {
	cand := []string{}
	if e.repo != "" {
		if b, err := resolveBase(e.repo, e.base); err == nil {
			cand = append(cand, b, "origin/"+b)
		}
	}
	for _, c := range cand {
		if out, err := gitSaida(e.work, nil, "merge-base", "HEAD", c); err == nil && out != "" {
			return out
		}
	}
	return ""
}

func lerRelatorio(work string) (string, time.Time) {
	if work == "" {
		return "", time.Time{}
	}
	p := filepath.Join(work, "RELATORIO-AGENTE.md")
	st, err := os.Stat(p)
	if err != nil {
		return "", time.Time{}
	}
	b, err := os.ReadFile(p)
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "", time.Time{}
	}
	return strings.TrimSpace(string(b)), st.ModTime().Truncate(time.Second)
}

// ultimosTextos devolve até n trechos de texto livre do agente no fim do log (linhas que não são marcas
// do rodar nem eventos "[tipo] ...").
func ultimosTextos(logPath string, n int) []string {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := int64(0)
	if st.Size() > estadoLeituraLog {
		off = st.Size() - estadoLeituraLog
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && len(buf) > 0 {
		return nil
	}
	var blocos []string
	var atual []string
	fecha := func() {
		if t := strings.TrimSpace(strings.Join(atual, " ")); t != "" {
			blocos = append(blocos, t)
		}
		atual = nil
	}
	for _, l := range strings.Split(string(buf), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "[") || strings.HasPrefix(l, "###") || strings.HasPrefix(l, "FIM ") || strings.HasPrefix(l, "ERRO:") || strings.HasPrefix(l, "AVISO:") {
			fecha()
			continue
		}
		atual = append(atual, l)
	}
	fecha()
	if len(blocos) > n {
		blocos = blocos[len(blocos)-n:]
	}
	for i, t := range blocos {
		blocos[i] = curto(t, estadoEventoMax)
	}
	return blocos
}

func lerCabEstado(path string) estadoCab {
	var c estadoCab
	b, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(b), "---\n") {
		return c
	}
	cab, _, ok := strings.Cut(string(b)[4:], "\n---\n")
	if !ok {
		return c
	}
	for _, l := range strings.Split(cab, "\n") {
		k, v, _ := strings.Cut(l, ": ")
		v = strings.TrimSpace(v)
		switch k {
		case "turno":
			c.Turno, _ = strconv.Atoi(v)
		case "head_resumo":
			c.HeadResumo = v
		case "resumo_mtime":
			c.ResumoMtime = v
		case "turno_resumo":
			c.TurnoResumo, _ = strconv.Atoi(v)
		}
	}
	return c
}

var reTitulo = regexp.MustCompile(`^#+\s*`)

// objetivo é a primeira linha não vazia do prompt (sem os # de título).
func objetivo(prompt string) string {
	for _, l := range strings.Split(prompt, "\n") {
		if l = strings.TrimSpace(reTitulo.ReplaceAllString(strings.TrimSpace(l), "")); l != "" {
			return curto(l, 300)
		}
	}
	return "(prompt vazio)"
}

func mascarar(s string) string { return comandos.Mascarar(s) }

// cortarFim limita s a n runas, mantendo o começo.
func cortarFim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n[…cortado]"
}

func limitarLinhas(s string, n int) string {
	ls := strings.Split(s, "\n")
	if len(ls) <= n {
		return s
	}
	return strings.Join(ls[:n], "\n") + fmt.Sprintf("\n[…mais %d linhas]", len(ls)-n)
}

// ContinuacaoOu devolve a continuação com o estado ou, sem estado nenhum, o texto fixo alternativo.
func (e *estadoAgente) ContinuacaoOu(fixo string) string {
	if t := e.Continuacao(); t != "" {
		return t
	}
	return fixo
}
