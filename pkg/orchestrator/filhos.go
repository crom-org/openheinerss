package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Variáveis de ambiente que um `rodar` passa ao harness: um `openheinerss rodar` lançado de dentro
// dele (agente filho) grava o pai no meta.json e se registra em <logs do pai>/<pai>.filhos/<nome>.
const (
	EnvPai     = "OPENHEINERSS_PAI"
	EnvPaiLogs = "OPENHEINERSS_PAI_LOGS"
)

// Padrões das rodadas de retomada quando há agentes filhos.
const (
	rodadasFilhosPadrao   = 5
	esperaFilhosPadrao    = 2 * time.Hour
	intervaloFilhosPadrao = 2 * time.Second
)

// esperaPattern reconhece o fim de turno de quem diz que vai esperar algo terminar.
var esperaPattern = regexp.MustCompile(`(?i)(aguardand|vou aguardar|vou esperar|esperando (os|o|a|as|pel|que|o fim|terminar)|em segundo plano|waiting for|will wait|in the background)`)

// filho é um agente lançado por este run: nome e caminho absoluto do meta.json dele.
type filho struct{ Nome, Meta string }

type estadoFilho struct {
	filho
	Vivo     bool
	PID      int
	Fim      string
	Codigo   *int
	Motivo   string
	Branch   string
	Worktree string
}

// rodadasFilhos guarda o estado das retomadas de um run.
type rodadasFilhos struct {
	feitas     int
	reportados map[string]bool
}

func filhosDir(logs, pai string) string { return filepath.Join(logs, pai+".filhos") }

// registrarFilho anota o filho na pasta de logs do pai (que pode estar em outro repositório).
func registrarFilho(paiLogs, pai, nome, metaPath string) error {
	if abs, err := filepath.Abs(metaPath); err == nil {
		metaPath = abs
	}
	dir := filhosDir(paiLogs, pai)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, nome), []byte(metaPath+"\n"), 0644)
}

func listarFilhos(logs, pai string) []filho {
	entries, err := os.ReadDir(filhosDir(logs, pai))
	if err != nil {
		return nil
	}
	var out []filho
	for _, e := range entries {
		if e.IsDir() || !nomeValido.MatchString(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(filhosDir(logs, pai), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, filho{Nome: e.Name(), Meta: strings.TrimSpace(string(b))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nome < out[j].Nome })
	return out
}

func lerFilho(f filho) estadoFilho {
	e := estadoFilho{filho: f}
	b, err := os.ReadFile(f.Meta)
	if err != nil {
		e.Motivo = "sem meta.json"
		return e
	}
	var m meta
	if json.Unmarshal(b, &m) != nil {
		e.Motivo = "meta.json inválido"
		return e
	}
	e.PID, e.Branch, e.Worktree = m.PID, m.Branch, m.Worktree
	if m.Fim != "" {
		e.Fim, e.Codigo, e.Motivo = m.Fim, m.Codigo, m.Motivo
		return e
	}
	if processAlive(m.PID) {
		e.Vivo = true
		return e
	}
	e.Motivo = "processo morreu sem FIM"
	return e
}

// esperarFilhos espera (sem consumir CPU: lê os meta.json e testa o PID a cada intervalo) até
// todos os filhos darem FIM ou o limite vencer. Devolve o último estado de cada um.
func esperarFilhos(ctx context.Context, o Options, fs []filho, limite time.Duration, write func(string)) ([]estadoFilho, error) {
	inicio := o.Now()
	avisados := map[string]bool{}
	for {
		estados := make([]estadoFilho, 0, len(fs))
		vivos := 0
		for _, f := range fs {
			e := lerFilho(f)
			estados = append(estados, e)
			if e.Vivo {
				vivos++
			} else if !avisados[f.Nome] {
				avisados[f.Nome] = true
				write(fmt.Sprintf("filho %s terminou: %s\n", f.Nome, descreverFilho(e)))
			}
		}
		if vivos == 0 {
			return estados, nil
		}
		if limite > 0 && o.Now().Sub(inicio) >= limite {
			write(fmt.Sprintf("espera pelos filhos venceu (%s); %d ainda rodando\n", limite, vivos))
			return estados, nil
		}
		if err := esperar(ctx, o, o.IntervaloFilhos); err != nil {
			return estados, err
		}
	}
}

// pararFilhos encerra os filhos ainda vivos quando o pai é interrompido.
func pararFilhos(logs, pai string, now time.Time) {
	for _, f := range listarFilhos(logs, pai) {
		if e := lerFilho(f); e.Vivo {
			_ = StopAgent(filepath.Dir(filepath.Dir(f.Meta)), f.Nome, now)
		}
	}
}

func descreverFilho(e estadoFilho) string {
	var partes []string
	switch {
	case e.Vivo:
		partes = append(partes, fmt.Sprintf("AINDA RODANDO (PID %d)", e.PID))
	case e.Fim != "":
		hora := e.Fim
		if t, err := time.Parse(time.RFC3339, e.Fim); err == nil {
			hora = t.Local().Format("15:04")
		}
		partes = append(partes, fmt.Sprintf("FIM %s código %d", hora, valueOr(e.Codigo, 0)))
		if e.Motivo != "" {
			partes = append(partes, "motivo "+e.Motivo)
		}
	default:
		partes = append(partes, "sem FIM ("+e.Motivo+")")
	}
	if e.Branch != "" {
		partes = append(partes, "branch "+e.Branch)
	}
	if e.Worktree != "" {
		partes = append(partes, "worktree "+e.Worktree)
		if _, err := os.Stat(filepath.Join(e.Worktree, "RELATORIO-AGENTE.md")); err == nil {
			partes = append(partes, "relatório "+filepath.Join(e.Worktree, "RELATORIO-AGENTE.md"))
		}
	}
	partes = append(partes, "log "+strings.TrimSuffix(e.Meta, ".meta.json")+".log")
	return strings.Join(partes, "; ")
}

func worktreeSuja(work string) bool {
	if work == "" {
		return false
	}
	out, err := exec.Command("git", "-C", work, "status", "--porcelain").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func branchAtual(work string) string {
	out, err := exec.Command("git", "-C", work, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// posTurno decide, depois de um turno bem-sucedido do pai, se o run deve esperar filhos e retomar a
// sessão. Devolve a mensagem de continuação ("" = encerrar como antes).
func posTurno(ctx context.Context, o Options, logs, work, ultimoTexto string, st *rodadasFilhos, write func(string)) (string, error) {
	if o.EsperarFilhos < 0 {
		return "", nil
	}
	diz := esperaPattern.MatchString(ultimoTexto)
	novos := filhosNovos(logs, o.Name, st)
	if len(novos) == 0 && diz {
		// Um filho lançado no fim do turno pode ainda não ter se registrado: confere de novo uma vez.
		if err := esperar(ctx, o, o.IntervaloFilhos); err != nil {
			return "", err
		}
		novos = filhosNovos(logs, o.Name, st)
	}
	vivos := 0
	estados := make([]estadoFilho, 0, len(novos))
	for _, f := range novos {
		e := lerFilho(f)
		estados = append(estados, e)
		if e.Vivo {
			vivos++
		}
	}
	if vivos == 0 && !diz {
		for _, f := range novos {
			st.reportados[f.Nome] = true // terminaram durante o turno: o pai já viu
		}
		return "", nil
	}
	if vivos == 0 && len(novos) == 0 && !worktreeSuja(work) {
		return "", nil // disse que ia esperar, mas não há nada pendente nem trabalho sem commit
	}
	if st.feitas >= o.RodadasFilhos {
		write(fmt.Sprintf("limite de %d rodada(s) de retomada atingido; %d filho(s) ainda rodando\n", o.RodadasFilhos, vivos))
		return "", nil
	}
	st.feitas++
	if vivos > 0 {
		nomes := make([]string, 0, vivos)
		for _, e := range estados {
			if e.Vivo {
				nomes = append(nomes, e.Nome)
			}
		}
		write(fmt.Sprintf("\naguardando %d agente(s) filho(s): %s\n", vivos, strings.Join(nomes, ", ")))
		var err error
		if estados, err = esperarFilhos(ctx, o, novos, o.EsperarFilhos, write); err != nil {
			return "", err
		}
	}
	for _, f := range novos {
		st.reportados[f.Nome] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- RETOMADA AUTOMÁTICA (rodada %d de %d) ---\n", st.feitas, o.RodadasFilhos)
	if len(estados) == 0 {
		b.WriteString("Seu turno terminou dizendo que ia esperar, mas nenhum agente filho está rodando e comandos em segundo plano não sobrevivem ao fim do turno: nada vai te acordar. ")
		b.WriteString("Rode agora, em PRIMEIRO plano, o que falta (verificação, testes, comandos com trava como flock), termine o trabalho, faça o commit e só então encerre.")
		if worktreeSuja(work) {
			b.WriteString("\nA worktree tem mudanças sem commit.")
		}
		return b.String(), nil
	}
	b.WriteString("Os agentes filhos que você lançou terminaram (ou a espera venceu):\n")
	for _, e := range estados {
		fmt.Fprintf(&b, "- %s: %s\n", e.Nome, descreverFilho(e))
	}
	b.WriteString("Junte o trabalho deles (revise; merge ou cherry-pick das branches), rode a verificação e finalize como a tarefa pede. ")
	b.WriteString("Se lançar outros agentes ou comandos em segundo plano, o openheinerss te acorda de novo quando eles terminarem.")
	return b.String(), nil
}

func filhosNovos(logs, pai string, st *rodadasFilhos) []filho {
	var novos []filho
	for _, f := range listarFilhos(logs, pai) {
		if !st.reportados[f.Nome] {
			novos = append(novos, f)
		}
	}
	return novos
}

// esperaFilhosEnv lê OPENHEINERSS_ESPERAR_FILHOS: "0"/"nao"/"false" desliga; duração ("30m") ou minutos ("45") limita.
func esperaFilhosEnv(v string) (time.Duration, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "":
		return 0, false
	case "0", "nao", "não", "false", "off", "no":
		return -1, true
	case "1", "sim", "true", "on", "yes":
		return 0, true
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d, true
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Minute, true
	}
	return 0, false
}

// ParseEsperarFilhos converte o valor de --esperar-filhos para Options.EsperarFilhos.
func ParseEsperarFilhos(v string) (time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return 0, nil
	}
	if d, ok := esperaFilhosEnv(v); ok {
		return d, nil
	}
	return 0, fmt.Errorf("valor inválido para --esperar-filhos %q: use sim, nao ou uma duração (ex.: 30m, 2h)", v)
}

// MensagemOrfaos é o aviso padrão de filhos que continuam rodando depois do fim do pai.
func MensagemOrfaos(nomes []string) string {
	return fmt.Sprintf("o pai terminou com %d agente(s) filho(s) ainda rodando (órfãos): %s; acompanhe com `openheinerss agentes`", len(nomes), strings.Join(nomes, ", "))
}

// balancoFilhos confere os filhos no fim do pai. Filho vivo vira órfão: aviso no log e no stderr,
// evento EvFilhosOrfaos, lista e motivo no meta.json. Com FilhosObrigatorios, um pai que terminaria
// com 0 termina com CodigoFilhoFalhou se algum filho falhou (código ≠ 0, morreu sem FIM) ou ficou órfão.
func balancoFilhos(o Options, logs string, cur *meta, code int, causa string, write func(string)) (int, string) {
	var orfaos, falhos []string
	var detalhes []string
	for _, f := range listarFilhos(logs, o.Name) {
		e := lerFilho(f)
		switch {
		case e.Vivo:
			orfaos = append(orfaos, e.Nome)
			detalhes = append(detalhes, fmt.Sprintf("%s (ainda rodando, PID %d)", e.Nome, e.PID))
		case e.Fim == "":
			falhos = append(falhos, e.Nome)
			detalhes = append(detalhes, fmt.Sprintf("%s (%s)", e.Nome, e.Motivo))
		case valueOr(e.Codigo, 0) != 0:
			falhos = append(falhos, e.Nome)
			detalhes = append(detalhes, fmt.Sprintf("%s (código %d)", e.Nome, valueOr(e.Codigo, 0)))
		}
	}
	if len(orfaos) > 0 {
		msg := MensagemOrfaos(orfaos)
		write("AVISO: " + msg + "\n")
		fmt.Fprintln(os.Stderr, "AVISO: "+msg)
		cur.FilhosOrfaos = orfaos
		if cur.Motivo == "" {
			cur.Motivo = MotivoFilhosOrfaos
		}
		o.emit(Evento{Tipo: EvFilhosOrfaos, Filhos: orfaos, Mensagem: msg})
	}
	if !o.FilhosObrigatorios || code != 0 || len(detalhes) == 0 {
		return code, causa
	}
	cur.Motivo = MotivoFilhoFalhou
	cur.FilhosFalhos = append(append([]string{}, falhos...), orfaos...)
	lista := strings.Join(detalhes, "; ")
	write(fmt.Sprintf("filho falhou (--filhos-obrigatorios): %s\n", lista))
	return CodigoFilhoFalhou, curto("filho falhou: "+lista, 200)
}

// filhosDoMotivo devolve os filhos que explicam o motivo do fim (para o orq.fim).
func filhosDoMotivo(m meta) []string {
	switch m.Motivo {
	case MotivoFilhoFalhou:
		return m.FilhosFalhos
	case MotivoFilhosOrfaos:
		return m.FilhosOrfaos
	}
	return nil
}
