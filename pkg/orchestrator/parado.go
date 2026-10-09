package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
)

// CodigoParado é o código de fim quando o detector interrompe um agente parado (retomável com --retomar).
const CodigoParado = 5

// MotivoParado é o motivo gravado no meta.json e no orq.fim de um agente interrompido pelo detector.
const MotivoParado = "parado"

// EvParado: o detector avisou (ou interrompeu) um agente que parece parado (Mensagem traz a linha do aviso).
const EvParado = "parado"

// IntervaloParado é o intervalo padrão entre as checagens do detector.
const IntervaloParado = 60 * time.Second

// limiteCPU é a fração de um núcleo, entre duas leituras, a partir da qual o agente conta como "CPU alta" (S4).
const limiteCPU = 0.05

// Níveis de um veredito do detector.
const (
	nivelNenhum = ""
	nivelAviso  = "aviso"
	nivelLaco   = "laco"
	nivelParar  = "parar"
)

// fontesParado são as leituras do mundo de que o detector precisa; os testes trocam por falsas.
type fontesParado struct {
	logMtime  func() (time.Time, bool) // S1
	worktree  func() string            // S2: assinatura da worktree
	logUltimo func() string            // S3: último trecho do log
	cpu       func() (int64, bool)     // S4: ticks acumulados do motor e filhos (ok=false: indisponível)
}

// Veredito é o resultado de uma checagem.
type Veredito struct {
	Nivel      string
	Novo       bool // o nível mudou desde a checagem anterior: vale avisar
	Mensagem   string
	S1, S2, S3 time.Duration // há quanto tempo cada sinal está parado
	CPUAlto    *bool         // nil = sem leitura
}

// DetectorParado cruza S1 (log sem escrita), S2 (worktree sem mudança), S3 (mesmo último evento) e
// S4 (CPU alta) e decide entre nada, aviso, laço e parar. O estado fica entre as leituras.
type DetectorParado struct {
	cfg config.Parado
	now func() time.Time
	f   fontesParado

	ini     time.Time
	logRef  time.Time
	proprio time.Time // mtime deixado pela escrita do próprio detector no log: não conta como atividade
	workSig string
	workDes time.Time
	evSig   string
	evDes   time.Time
	cpuPrev int64
	cpuEm   time.Time
	cpuTem  bool
	cpuAlto *bool
	nivel   string
}

func novoDetectorParado(cfg config.Parado, now func() time.Time, f fontesParado) *DetectorParado {
	d := &DetectorParado{cfg: cfg, now: now, f: f}
	d.Reiniciar()
	return d
}

// Reiniciar zera o estado entre leituras: S2/S3 passam a contar desde agora e S1 desde o log atual.
func (d *DetectorParado) Reiniciar() {
	agora := d.now()
	d.ini, d.workDes, d.evDes = agora, agora, agora
	d.workSig, d.evSig = "", ""
	d.logRef, d.cpuTem, d.cpuAlto, d.nivel = time.Time{}, false, nil, nivelNenhum
	d.workSig = d.f.worktree()
	d.evSig = hashTexto(d.f.logUltimo())
	if m, ok := d.f.logMtime(); ok && !m.Equal(d.proprio) {
		d.logRef = m
	}
	if d.logRef.IsZero() {
		d.logRef = agora
	}
}

// MarcarEscritaPropria diz ao detector que o mtime atual do log veio dele mesmo (um aviso `[parado]`).
func (d *DetectorParado) MarcarEscritaPropria() {
	if m, ok := d.f.logMtime(); ok {
		d.proprio = m
	}
}

// Verificar lê os sinais e devolve o veredito.
func (d *DetectorParado) Verificar() Veredito {
	agora := d.now()
	if m, ok := d.f.logMtime(); ok && !m.Equal(d.proprio) {
		d.logRef = m
	}
	if sig := d.f.worktree(); sig != d.workSig {
		d.workSig, d.workDes = sig, agora
	}
	if sig := hashTexto(d.f.logUltimo()); sig != d.evSig {
		d.evSig, d.evDes = sig, agora
	}
	if ticks, ok := d.f.cpu(); ok {
		if d.cpuTem {
			if dt := agora.Sub(d.cpuEm).Seconds(); dt > 0 {
				alto := float64(ticks-d.cpuPrev)/clkTck/dt >= limiteCPU
				d.cpuAlto = &alto
			}
		}
		d.cpuPrev, d.cpuEm, d.cpuTem = ticks, agora, true
	} else {
		d.cpuAlto, d.cpuTem = nil, false
	}
	v := Veredito{S1: agora.Sub(d.logRef), S2: agora.Sub(d.workDes), S3: agora.Sub(d.evDes), CPUAlto: d.cpuAlto}
	v.Nivel, v.Mensagem = d.decidir(v)
	v.Novo = v.Nivel != nivelNenhum && v.Nivel != d.nivel
	d.nivel = v.Nivel
	return v
}

func (d *DetectorParado) decidir(v Veredito) (string, string) {
	av := d.cfg.Aviso
	if !d.cfg.Ligado() {
		return nivelNenhum, ""
	}
	s1, s2, s3 := v.S1 >= av, v.S2 >= av, v.S3 >= av
	switch {
	case s1 && s2 && s3:
		menor := minDur(v.S1, minDur(v.S2, v.S3))
		if v.CPUAlto != nil && *v.CPUAlto {
			return nivelAviso, fmt.Sprintf("sem escrita no log nem mudança na worktree há %s, mas a CPU está ativa (build ou teste longo?); só aviso", arred(menor))
		}
		if d.cfg.PodeParar() && menor >= d.cfg.Parar {
			return nivelParar, fmt.Sprintf("sem escrita no log, sem mudança na worktree e mesmo último evento há %s; interrompendo (retomável, código %d)", arred(menor), CodigoParado)
		}
		return nivelAviso, fmt.Sprintf("sem escrita no log, sem mudança na worktree e mesmo último evento há %s", arred(menor))
	case s3 && !s1:
		return nivelLaco, fmt.Sprintf("o log segue ativo, mas o último evento é o mesmo há %s (laço?): mude de abordagem", arred(v.S3))
	}
	return nivelNenhum, ""
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func arred(d time.Duration) time.Duration { return d.Round(time.Second) }

func hashTexto(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// ---- leituras reais ----

// ultimaLinhaLog devolve a última linha útil do fim do log, sem as marcas do próprio runner
// ([parado], FIM, ### tentativa), que não são trabalho do agente.
func ultimaLinhaLog(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	const cauda = 8192
	off := info.Size() - cauda
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	n, _ := f.ReadAt(buf, off)
	linhas := strings.Split(string(buf[:n]), "\n")
	for i := len(linhas) - 1; i >= 0; i-- {
		l := strings.TrimSpace(linhas[i])
		if l == "" || strings.HasPrefix(l, "[parado]") || strings.HasPrefix(l, "FIM ") || strings.HasPrefix(l, "### ") {
			continue
		}
		return l
	}
	return ""
}

// assinaturaWorktree resume o estado da worktree: HEAD, hash do `git status --porcelain` e o maior
// mtime entre os arquivos alterados (pega edições repetidas no mesmo arquivo já modificado).
func assinaturaWorktree(dir string) string {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "-z", "--untracked-files=all")
	out, err := cmd.Output()
	if err != nil {
		return "erro"
	}
	var maior int64
	entradas := strings.Split(string(out), "\x00")
	for i := 0; i < len(entradas) && i < 5000; i++ {
		e := entradas[i]
		if len(e) < 4 {
			continue
		}
		if e[0] == 'R' || e[0] == 'C' || e[1] == 'R' || e[1] == 'C' {
			i++ // o próximo é o nome antigo
		}
		if st, err := os.Lstat(filepath.Join(dir, e[3:])); err == nil && st.ModTime().UnixNano() > maior {
			maior = st.ModTime().UnixNano()
		}
	}
	h := sha256.Sum256(out)
	return headDe(dir) + "|" + hex.EncodeToString(h[:8]) + "|" + strconv.FormatInt(maior, 10)
}

// clkTck é o CLK_TCK do Linux (100 em todas as plataformas comuns).
const clkTck = 100.0

type statProc struct {
	pid, ppid, pgid    int
	utime, stime, cuts int64
}

// lerStatProc lê /proc/<pid>/stat. Fora do Linux (sem /proc) devolve ok=false.
func lerStatProc(pid string) (statProc, bool) {
	b, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return statProc{}, false
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return statProc{}, false
	}
	c := strings.Fields(s[i+1:]) // começa no campo 3
	if len(c) < 15 {
		return statProc{}, false
	}
	n := func(k int) int64 { v, _ := strconv.ParseInt(c[k], 10, 64); return v }
	p, _ := strconv.Atoi(pid)
	return statProc{pid: p, ppid: int(n(1)), pgid: int(n(2)), utime: n(11), stime: n(12), cuts: n(13) + n(14)}, true
}

func procsPorPai() (map[int][]statProc, bool) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	por := map[int][]statProc{}
	for _, e := range ents {
		if e.Name() == "" || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		if st, ok := lerStatProc(e.Name()); ok {
			por[st.ppid] = append(por[st.ppid], st)
		}
	}
	return por, true
}

// cpuDescendentes soma utime+stime (e o já recolhido dos filhos que terminaram) de todos os
// descendentes de raiz: o motor e o que ele lança. Só Linux; ok=false nos outros sistemas.
func cpuDescendentes(raiz int) (int64, bool) {
	por, ok := procsPorPai()
	if !ok {
		return 0, false
	}
	var total int64
	fila := []int{raiz}
	for len(fila) > 0 {
		pai := fila[0]
		fila = fila[1:]
		for _, st := range por[pai] {
			total += st.utime + st.stime + st.cuts
			fila = append(fila, st.pid)
		}
	}
	return total, true
}

// pararMotor manda SIGTERM ao grupo de cada filho direto de raiz (o motor); devolve quantos sinalizou.
func pararMotor(raiz int) int {
	por, ok := procsPorPai()
	if !ok {
		return 0
	}
	n := 0
	for _, st := range por[raiz] {
		if stopAgentProcess(st.pid) == nil {
			n++
		}
	}
	return n
}

// ---- estado em disco (agentes listar) ----

// EstadoParado é o logs/<nome>.parado.json: a última checagem do detector, lida pelo `agentes listar`.
type EstadoParado struct {
	VerificadoEm string `json:"verificado_em"`
	LogParadoS   int64  `json:"log_parado_s"`
	WorktreeS    int64  `json:"worktree_parada_s"`
	EventoS      int64  `json:"evento_parado_s"`
	AvisoS       int64  `json:"aviso_s"`
	CPUAlto      *bool  `json:"cpu_alta,omitempty"`
	Nivel        string `json:"nivel,omitempty"`
}

func caminhoEstadoParado(logs, nome string) string { return filepath.Join(logs, nome+".parado.json") }

func gravarEstadoParado(path string, agora time.Time, aviso time.Duration, v Veredito) {
	b, err := json.Marshal(EstadoParado{VerificadoEm: agora.Format(time.RFC3339), LogParadoS: int64(v.S1.Seconds()), WorktreeS: int64(v.S2.Seconds()), EventoS: int64(v.S3.Seconds()), AvisoS: int64(aviso.Seconds()), CPUAlto: v.CPUAlto, Nivel: v.Nivel})
	if err == nil {
		_ = os.WriteFile(path, append(b, '\n'), 0644)
	}
}

// estadoVivo classifica um agente vivo: rodando; lento (só S1: log sem escrita, sem prova de que a
// worktree também parou); parado (S1 + S2, e S3 quando o detector do rodar gravou o estado).
func estadoVivo(logs, nome, logPath string, now time.Time) string {
	if !staleLog(logPath, now) {
		return "rodando"
	}
	b, err := os.ReadFile(caminhoEstadoParado(logs, nome))
	if err != nil {
		return "lento"
	}
	var e EstadoParado
	if json.Unmarshal(b, &e) != nil {
		return "lento"
	}
	em, errT := time.Parse(time.RFC3339, e.VerificadoEm)
	if errT != nil || now.Sub(em) > 5*time.Minute || e.AvisoS <= 0 {
		return "lento" // estado velho: o detector não está olhando mais
	}
	extra := int64(now.Sub(em).Seconds())
	if e.WorktreeS+extra >= e.AvisoS && e.EventoS+extra >= e.AvisoS {
		return "parado"
	}
	return "lento"
}

// ---- monitor no rodar ----

type monitorParado struct {
	det    *DetectorParado
	ativo  atomic.Bool
	parar  chan string
	statep string
	cancel context.CancelFunc
}

// Fechar para a goroutine e apaga o arquivo de estado (nil-safe).
func (m *monitorParado) Fechar() {
	if m != nil {
		m.cancel()
		_ = os.Remove(m.statep)
	}
}

// Ativo liga/desliga as checagens (só valem com o motor rodando; desligado enquanto espera filhos etc.).
func (m *monitorParado) Ativo(v bool) {
	if m != nil {
		m.ativo.Store(v)
	}
}

// Parar entrega a mensagem quando o detector interrompeu o motor.
func (m *monitorParado) Parar() <-chan string {
	if m == nil {
		return nil
	}
	return m.parar
}

// iniciarParado resolve a regra efetiva e, se estiver ligada, lança a goroutine de checagem.
// Devolve nil quando o detector está desligado.
func iniciarParado(ctx context.Context, o Options, repo, work, logPath, logs string, write func(string)) (*monitorParado, error) {
	cfg, err := config.ParadoEfetivo(repo)
	if err != nil {
		return nil, err
	}
	if cfg, err = config.AplicarFlagsParado(cfg, o.ParadoAviso, o.ParadoParar); err != nil {
		return nil, err
	}
	if !cfg.Ligado() {
		return nil, nil
	}
	f := fontesParado{
		logMtime: func() (time.Time, bool) {
			st, err := os.Stat(logPath)
			if err != nil {
				return time.Time{}, false
			}
			return st.ModTime(), true
		},
		worktree:  func() string { return assinaturaWorktree(work) },
		logUltimo: func() string { return ultimaLinhaLog(logPath) },
		cpu:       func() (int64, bool) { return cpuDescendentes(os.Getpid()) },
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &monitorParado{cancel: cancel, det: novoDetectorParado(cfg, o.Now, f), parar: make(chan string, 1), statep: caminhoEstadoParado(logs, o.Name)}
	intervalo := o.ParadoIntervalo
	if intervalo <= 0 {
		intervalo = IntervaloParado
	}
	go func() {
		tick := time.NewTicker(intervalo)
		defer tick.Stop()
		estavaAtivo := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			if !m.ativo.Load() {
				estavaAtivo = false
				continue
			}
			if !estavaAtivo { // motor (re)começou: contagem nova
				estavaAtivo = true
				m.det.Reiniciar()
				continue
			}
			v := m.det.Verificar()
			gravarEstadoParado(m.statep, o.Now(), cfg.Aviso, v)
			if !v.Novo {
				continue
			}
			linha := "[parado] " + v.Mensagem
			write("\n" + linha + "\n")
			m.det.MarcarEscritaPropria()
			o.emit(Evento{Tipo: EvParado, Motor: o.Motor, Mensagem: linha})
			if err := appendEventLogParado(o, filepath.Base(repo), o.Motor, v); err != nil {
				write("AVISO: não gravei o aviso no log de eventos: " + err.Error() + "\n")
				m.det.MarcarEscritaPropria()
			}
			if v.Nivel == nivelParar {
				pararMotor(os.Getpid())
				m.parar <- v.Mensagem
				return
			}
		}
	}()
	return m, nil
}

// appendEventLogParado grava a linha orq.parado no log de eventos (--eventos-log / eventos_log), se houver.
func appendEventLogParado(o Options, projeto, motor string, v Veredito) error {
	path := o.EventLog
	if path == "" {
		if spec, ok := harness.CustomSpecFor(motor); ok {
			path = spec.EventLog
		}
	}
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "[%s] orq.parado %s nivel=%s log=%ds worktree=%ds evento=%ds\n", projeto, o.Name, v.Nivel, int(v.S1.Seconds()), int(v.S2.Seconds()), int(v.S3.Seconds()))
	return err
}
