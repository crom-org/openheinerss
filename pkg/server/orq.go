package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/orchestrator"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// IntervaloObservador é o intervalo de leitura de logs/*.meta.json e *.log dos agentes lançados fora do servidor.
var IntervaloObservador = time.Second

// intervaloProgresso é o mínimo entre dois orq.progresso do mesmo agente.
var intervaloProgresso = 2 * time.Second

type ctxKey struct{}

// conexao é um cliente (WebSocket ou STDIO) que pode assinar eventos.
type conexao struct {
	send func(interface{})
}

func comConexao(ctx context.Context, send func(interface{})) (context.Context, *conexao) {
	c := &conexao{send: send}
	return context.WithValue(ctx, ctxKey{}, c), c
}

type assinatura struct {
	projeto, agente string
}

type job struct {
	id, nome, dir, projeto string
	cancel                 context.CancelFunc
	fimEmitido             bool
}

type decisao struct {
	params protocol.OrqDecisaoParams
	ch     chan decisaoResp
}

type decisaoResp struct {
	allow   bool
	msg     string
	encerra bool
}

type limiteProg struct {
	ultimo  time.Time
	pending *pendente
	timer   *time.Timer
}

type pendente struct {
	n               protocol.Notification
	projeto, agente string
}

type estadoMeta struct {
	inicio    string
	tentativa int
	fim       bool
	logPos    int64
	iniciado  bool
	travou    bool
}

type pasta struct {
	dir, projeto string
	baseline     bool
	estados      map[string]*estadoMeta
}

// Orq é o orquestrador do servidor: executa rodar em segundo plano, observa as pastas de
// agentes e entrega os eventos orq.* para as conexões que chamaram eventos.assinar.
type Orq struct {
	mu        sync.Mutex
	emitMu    sync.Mutex
	scanMu    sync.Mutex // ordem de travas: scanMu, emitMu, mu
	subs      map[*conexao]assinatura
	jobs      map[string]*job
	decisoes  map[string]*decisao
	pastas    map[string]*pasta
	limites   map[string]*limiteProg
	seq       int
	ctx       context.Context
	cancel    context.CancelFunc
	obsOn     bool
	wg        sync.WaitGroup // execuções de rodar em andamento
	agora     func() time.Time
	geracao   string
	maxAgents int
	// negarEncerra é o padrão de serve --negar-encerra; rodar.decidir com "encerrar" vence.
	negarEncerra bool
}

func newOrq(limite ...int) *Orq {
	maxAgents := 0
	if len(limite) > 0 {
		maxAgents = limite[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Orq{subs: map[*conexao]assinatura{}, jobs: map[string]*job{}, decisoes: map[string]*decisao{}, pastas: map[string]*pasta{}, limites: map[string]*limiteProg{}, ctx: ctx, cancel: cancel, agora: time.Now, geracao: novaGeracao(), maxAgents: maxAgents}
}

// Close para as execuções, o observador e os temporizadores.
func (o *Orq) Close() {
	o.cancel()
	// Dá tempo para os agentes fecharem log e meta.json (FIM 130) antes de o processo sair.
	espera := make(chan struct{})
	go func() { o.wg.Wait(); close(espera) }()
	select {
	case <-espera:
	case <-time.After(5 * time.Second):
	}
	o.mu.Lock()
	for _, l := range o.limites {
		if l.timer != nil {
			l.timer.Stop()
		}
	}
	o.mu.Unlock()
}

func (o *Orq) desconectar(c *conexao) {
	o.mu.Lock()
	delete(o.subs, c)
	o.mu.Unlock()
}

// ---------- emissão ----------

func (o *Orq) emitir(method, projeto, agente string, params interface{}) {
	switch p := params.(type) {
	case protocol.OrqInicioParams:
		p.Geracao = o.geracao
		params = p
	case protocol.OrqProgressoParams:
		p.Geracao = o.geracao
		params = p
	case protocol.OrqFimParams:
		p.Geracao = o.geracao
		params = p
	case protocol.OrqErroParams:
		p.Geracao = o.geracao
		params = p
	case protocol.OrqDecisaoParams:
		p.Geracao = o.geracao
		params = p
	case protocol.OrqFilhosOrfaosParams:
		p.Geracao = o.geracao
		params = p
	}
	o.emitMu.Lock()
	defer o.emitMu.Unlock()
	o.entregar(protocol.NewNotification(method, params), projeto, agente)
}

// entregar exige emitMu.
func (o *Orq) entregar(n protocol.Notification, projeto, agente string) {
	n.Geracao = o.geracao
	o.mu.Lock()
	var alvos []*conexao
	for c, a := range o.subs {
		if (a.projeto == "" || a.projeto == projeto) && (a.agente == "" || a.agente == agente) {
			alvos = append(alvos, c)
		}
	}
	o.mu.Unlock()
	for _, c := range alvos {
		c.send(n)
	}
}

// progresso aplica o limite de 1 a cada 2 s por agente; o excedente fica pendente e sai no fim da janela.
func (o *Orq) progresso(p protocol.OrqProgressoParams) {
	p.Geracao = o.geracao
	chave := p.Projeto + "/" + p.Agente
	n := protocol.NewNotification(protocol.EventOrqProgresso, p)
	o.emitMu.Lock()
	defer o.emitMu.Unlock()
	o.mu.Lock()
	l := o.limites[chave]
	if l == nil {
		l = &limiteProg{}
		o.limites[chave] = l
	}
	agora := o.agora()
	if agora.Sub(l.ultimo) >= intervaloProgresso {
		l.ultimo, l.pending = agora, nil
		o.mu.Unlock()
		o.entregar(n, p.Projeto, p.Agente)
		return
	}
	l.pending = &pendente{n, p.Projeto, p.Agente}
	if l.timer == nil {
		l.timer = time.AfterFunc(intervaloProgresso-agora.Sub(l.ultimo), func() { o.descarregar(chave) })
	}
	o.mu.Unlock()
}

func (o *Orq) descarregar(chave string) {
	o.emitMu.Lock()
	defer o.emitMu.Unlock()
	o.mu.Lock()
	l := o.limites[chave]
	if l == nil {
		o.mu.Unlock()
		return
	}
	l.timer = nil
	p := l.pending
	l.pending = nil
	if p != nil {
		l.ultimo = o.agora()
	}
	o.mu.Unlock()
	if p != nil && o.ctx.Err() == nil {
		o.entregar(p.n, p.projeto, p.agente)
	}
}

// esquecerProgresso descarta o progresso pendente do agente (usado antes do fim).
func (o *Orq) esquecerProgresso(projeto, agente string) {
	o.emitMu.Lock()
	defer o.emitMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if l := o.limites[projeto+"/"+agente]; l != nil {
		l.pending = nil
		if l.timer != nil {
			l.timer.Stop()
			l.timer = nil
		}
		l.ultimo = time.Time{}
	}
}

func (o *Orq) fim(p protocol.OrqFimParams) {
	o.esquecerProgresso(p.Projeto, p.Agente)
	o.emitir(protocol.EventOrqFim, p.Projeto, p.Agente, p)
}

// ---------- caminhos ----------

func raizGit(cwd string) string {
	repo, err := orchestrator.RepoRoot(cwd)
	if err != nil {
		return cwd
	}
	return repo
}

// resolverPasta devolve a pasta de agentes e o nome do projeto (nome da raiz git), com as mesmas regras do rodar.
func resolverPasta(cwd, agentes string) (dir, projeto string) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	repo := raizGit(cwd)
	if agentes == "" {
		agentes = os.Getenv("AGENTES")
	}
	if agentes == "" {
		agentes = ".claude/agentes"
	}
	if !filepath.IsAbs(agentes) {
		agentes = filepath.Join(repo, agentes)
	}
	return agentes, filepath.Base(repo)
}

// ---------- métodos ----------

func (o *Orq) assinar(c *conexao, p protocol.EventosAssinarParams) interface{} {
	dir, projeto := resolverPasta(p.CWD, p.Pasta)
	o.mu.Lock()
	o.subs[c] = assinatura{projeto: p.Projeto, agente: p.Agente}
	o.mu.Unlock()
	o.observar(dir, projeto)
	return map[string]interface{}{"assinado": true, "projeto": p.Projeto, "agente": p.Agente, "observando": dir}
}

// observar registra a pasta e faz a leitura inicial (síncrona) antes de devolver.
func (o *Orq) observar(dir, projeto string) {
	o.mu.Lock()
	if _, ok := o.pastas[dir]; !ok {
		o.pastas[dir] = &pasta{dir: dir, projeto: projeto, estados: map[string]*estadoMeta{}}
	}
	ligar := !o.obsOn
	o.obsOn = true
	o.mu.Unlock()
	o.varrer(dir)
	if ligar {
		go o.laco()
	}
}

func (o *Orq) laco() {
	t := time.NewTicker(IntervaloObservador)
	defer t.Stop()
	for {
		select {
		case <-o.ctx.Done():
			return
		case <-t.C:
			o.mu.Lock()
			dirs := make([]string, 0, len(o.pastas))
			for d := range o.pastas {
				dirs = append(dirs, d)
			}
			o.mu.Unlock()
			for _, d := range dirs {
				o.varrer(d)
			}
		}
	}
}

type metaArq struct {
	Motor     string `json:"motor"`
	Modelo    string `json:"modelo"`
	Tentativa int    `json:"tentativa"`
	Inicio    string `json:"inicio"`
	PID       int    `json:"pid"`
	Fim       string `json:"fim"`
	Codigo    *int   `json:"codigo"`
	Motivo    string `json:"motivo"`
	// Filhos que falharam ou ficaram órfãos no fim do pai (ver orchestrator.meta).
	FilhosFalhos []string `json:"filhos_falhos"`
	FilhosOrfaos []string `json:"filhos_orfaos"`
}

func lerMeta(path string) (metaArq, bool) {
	var m metaArq
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}

func vivo(pid int) bool {
	if pid <= 0 {
		return false
	}
	return processAlive(pid)
}

// varrer compara logs/*.meta.json e *.log com o último estado visto e emite o que mudou.
// Agentes lançados por este servidor (jobs ativos) são ignorados: eles emitem direto.
func (o *Orq) varrer(dir string) {
	o.scanMu.Lock()
	defer o.scanMu.Unlock()
	o.mu.Lock()
	p := o.pastas[dir]
	o.mu.Unlock()
	if p == nil {
		return
	}
	logs := filepath.Join(dir, "logs")
	entries, err := os.ReadDir(logs)
	if err != nil {
		o.mu.Lock()
		p.baseline = true
		o.mu.Unlock()
		return
	}
	o.mu.Lock()
	conhecida := p.baseline
	p.baseline = true
	o.mu.Unlock()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		nome := strings.TrimSuffix(e.Name(), ".meta.json")
		o.varrerAgente(p, conhecida, nome, filepath.Join(logs, nome))
	}
}

func (o *Orq) proprio(dir, nome string) bool {
	for _, j := range o.jobs {
		if j.dir == dir && j.nome == nome {
			return true
		}
	}
	return false
}

func (o *Orq) varrerAgente(p *pasta, conhecida bool, nome, prefixo string) {
	m, ok := lerMeta(prefixo + ".meta.json")
	if !ok {
		return
	}
	logPath := prefixo + ".log"
	o.mu.Lock()
	if o.proprio(p.dir, nome) {
		o.mu.Unlock()
		return
	}
	st := p.estados[nome]
	if st == nil {
		st = &estadoMeta{}
		p.estados[nome] = st
	}
	o.mu.Unlock()
	rodando := m.Fim == "" && vivo(m.PID)
	morto := m.Fim == "" && !rodando
	if !st.iniciado && !conhecida && !rodando {
		// Primeira leitura da pasta: o que já terminou (ou morreu) é histórico e não gera evento.
		st.inicio, st.tentativa, st.fim, st.iniciado = m.Inicio, m.Tentativa, true, true
		st.logPos = max64(0, tamanho(logPath))
		return
	}
	if !st.iniciado && !conhecida {
		// Já estava rodando quando começamos a observar: anuncia, mas só o log novo vira progresso.
		st.logPos = max64(0, tamanho(logPath))
	}
	if !st.iniciado || m.Inicio != st.inicio || m.Tentativa != st.tentativa {
		if st.iniciado && m.Inicio != st.inicio {
			st.logPos = 0
		}
		st.inicio, st.tentativa, st.fim, st.iniciado = m.Inicio, m.Tentativa, false, true
		o.emitir(protocol.EventOrqInicio, p.projeto, nome, protocol.OrqInicioParams{Agente: nome, Projeto: p.projeto, Motor: m.Motor, Modelo: m.Modelo, Tentativa: m.Tentativa, Worktree: filepath.Join(p.dir, nome)})
	}
	// Progresso: últimas linhas novas do log.
	if sz := tamanho(logPath); sz >= 0 && !st.fim {
		if sz < st.logPos {
			st.logPos = 0
		}
		if sz > st.logPos {
			if linha := ultimaLinha(logPath, st.logPos, sz); linha != "" {
				o.progresso(protocol.OrqProgressoParams{Agente: nome, Projeto: p.projeto, Resumo: linha})
			}
			st.logPos = sz
		}
	}
	if st.fim {
		return
	}
	if m.Fim != "" {
		st.fim = true
		cod := 0
		if m.Codigo != nil {
			cod = *m.Codigo
		}
		var dur float64
		if i, e1 := time.Parse(time.RFC3339, m.Inicio); e1 == nil {
			if f, e2 := time.Parse(time.RFC3339, m.Fim); e2 == nil {
				dur = f.Sub(i).Seconds()
			}
		}
		rel := filepath.Join(p.dir, nome, "RELATORIO-AGENTE.md")
		if _, err := os.Stat(rel); err != nil {
			rel = ""
		}
		if len(m.FilhosOrfaos) > 0 {
			o.emitir(protocol.EventOrqFilhosOrfaos, p.projeto, nome, protocol.OrqFilhosOrfaosParams{Agente: nome, Projeto: p.projeto, Filhos: m.FilhosOrfaos, Mensagem: orchestrator.MensagemOrfaos(m.FilhosOrfaos)})
		}
		o.fim(protocol.OrqFimParams{Agente: nome, Projeto: p.projeto, Codigo: cod, Tentativas: m.Tentativa, Duracao: dur, Relatorio: rel, Motivo: m.Motivo, Filhos: filhosDoFim(m)})
		return
	}
	if !st.travou && orchestrator.LogParado(logPath, o.agora()) {
		st.travou = true
		o.emitir(protocol.EventOrqErro, p.projeto, nome, protocol.OrqErroParams{Agente: nome, Projeto: p.projeto, Mensagem: fmt.Sprintf("agente %s travou: sem escrever no log pelo prazo configurado", nome)})
	}
	if morto {
		st.fim = true
		o.emitir(protocol.EventOrqErro, p.projeto, nome, protocol.OrqErroParams{Agente: nome, Projeto: p.projeto, Mensagem: fmt.Sprintf("processo %d terminou sem registrar o fim", m.PID)})
		o.fim(protocol.OrqFimParams{Agente: nome, Projeto: p.projeto, Codigo: 1, Tentativas: m.Tentativa})
	}
}

func tamanho(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return fi.Size()
}

// ultimaLinha devolve a última linha não vazia entre as posições, resumida.
func ultimaLinha(path string, de, ate int64) string {
	if ate-de > 8192 {
		de = ate - 8192
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, ate-de)
	n, _ := f.ReadAt(buf, de)
	linhas := strings.Split(string(buf[:n]), "\n")
	for i := len(linhas) - 1; i >= 0; i-- {
		if l := strings.Join(strings.Fields(linhas[i]), " "); l != "" && !strings.HasPrefix(l, "### ") {
			r := []rune(l)
			if len(r) > 160 {
				return "…" + string(r[len(r)-160:])
			}
			return l
		}
	}
	return ""
}

// ---------- rodar ----------

func (o *Orq) iniciar(p protocol.RodarIniciarParams) (protocol.RodarIniciarResult, error) {
	if p.Nome == "" || p.Motor == "" {
		return protocol.RodarIniciarResult{}, fmt.Errorf("rodar.iniciar exige nome e motor")
	}
	p.MaxAgentes = o.limitarAgentes(p.MaxAgentes)
	dir, projeto := resolverPasta(p.CWD, p.Pasta)
	if p.Projeto != "" {
		projeto = p.Projeto
	}
	cwd := p.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	o.mu.Lock()
	if o.ctx.Err() != nil {
		o.mu.Unlock()
		return protocol.RodarIniciarResult{}, fmt.Errorf("servidor encerrando")
	}
	if o.proprio(dir, p.Nome) {
		o.mu.Unlock()
		return protocol.RodarIniciarResult{}, fmt.Errorf("agente %q já está rodando neste servidor", p.Nome)
	}
	o.seq++
	j := &job{id: fmt.Sprintf("rodar-%d", o.seq), nome: p.Nome, dir: dir, projeto: projeto}
	ctx, cancel := context.WithCancel(o.ctx)
	j.cancel = cancel
	o.jobs[j.id] = j
	o.wg.Add(1)
	o.mu.Unlock()
	o.observarSemVarrer(dir, projeto)

	opts := orchestrator.Options{Name: p.Nome, Motor: p.Motor, Model: p.Modelo, Effort: p.Esforco, PromptFile: p.Prompt, PromptText: p.Texto, Retomar: p.Retomar.Continuar, SessaoNativa: p.Retomar.ID, SemTrocaConta: p.SemTrocaConta, AgentsDir: p.Pasta, BranchBase: p.BranchBase, MaxLoad: p.CargaMax, MaxAgents: p.MaxAgentes, Attempts: p.Tentativas, QuotaMax: p.CotaMax, PastasPermitidas: p.PastasPermitidas, FilhosObrigatorios: p.FilhosObrigatorios, ViaServidor: true}
	opts.OnEvent = func(e orchestrator.Evento) { o.deJob(j, e) }
	opts.DecidirFim = func(ctx context.Context, q orchestrator.Pergunta) (bool, string, bool) { return o.perguntar(ctx, j, q) }
	go func() {
		defer o.wg.Done()
		defer cancel()
		res, err := orchestrator.Run(ctx, cwd, opts)
		if err != nil && !j.fimEmitido {
			o.emitir(protocol.EventOrqErro, projeto, p.Nome, protocol.OrqErroParams{ID: j.id, Agente: p.Nome, Projeto: projeto, Mensagem: err.Error()})
		}
		if !j.fimEmitido {
			code := res.Code
			if err != nil && code == 0 {
				code = 1
			}
			o.fim(protocol.OrqFimParams{ID: j.id, Agente: p.Nome, Projeto: projeto, Codigo: code, Tentativas: res.Attempts})
		}
		// Sincroniza o observador com o estado final antes de liberar o nome.
		o.scanMu.Lock()
		defer o.scanMu.Unlock()
		o.mu.Lock()
		if pa := o.pastas[dir]; pa != nil {
			if m, ok := lerMeta(filepath.Join(dir, "logs", p.Nome+".meta.json")); ok {
				pa.estados[p.Nome] = &estadoMeta{inicio: m.Inicio, tentativa: m.Tentativa, fim: true, iniciado: true, logPos: max64(0, tamanho(filepath.Join(dir, "logs", p.Nome+".log")))}
			}
		}
		delete(o.jobs, j.id)
		o.mu.Unlock()
	}()
	return protocol.RodarIniciarResult{Geracao: o.geracao, ID: j.id, Agente: p.Nome, Projeto: projeto}, nil
}

// limitarAgentes aplica o teto de serve --max-agentes ao maxAgentes pedido pelo cliente.
func (o *Orq) limitarAgentes(pedido int) int {
	if o.maxAgents > 0 && (pedido <= 0 || pedido > o.maxAgents) {
		return o.maxAgents
	}
	return pedido
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// observarSemVarrer registra a pasta para o observador sem leitura inicial (o job emite por conta própria).
func (o *Orq) observarSemVarrer(dir, projeto string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.pastas[dir]; !ok {
		o.pastas[dir] = &pasta{dir: dir, projeto: projeto, baseline: true, estados: map[string]*estadoMeta{}}
	}
}

func (o *Orq) deJob(j *job, e orchestrator.Evento) {
	switch e.Tipo {
	case orchestrator.EvInicio:
		o.emitir(protocol.EventOrqInicio, j.projeto, j.nome, protocol.OrqInicioParams{ID: j.id, Agente: j.nome, Projeto: j.projeto, Motor: e.Motor, Modelo: e.Modelo, Tentativa: e.Tentativa, Worktree: e.Worktree, TrocaDe: e.TrocaDe, TrocaMotivo: e.TrocaMotivo})
	case orchestrator.EvProgresso:
		o.progresso(protocol.OrqProgressoParams{ID: j.id, Agente: j.nome, Projeto: j.projeto, Resumo: e.Resumo})
	case orchestrator.EvErro:
		o.emitir(protocol.EventOrqErro, j.projeto, j.nome, protocol.OrqErroParams{ID: j.id, Agente: j.nome, Projeto: j.projeto, Mensagem: e.Mensagem, Cota: e.Cota})
	case orchestrator.EvFilhosOrfaos:
		o.emitir(protocol.EventOrqFilhosOrfaos, j.projeto, j.nome, protocol.OrqFilhosOrfaosParams{ID: j.id, Agente: j.nome, Projeto: j.projeto, Filhos: e.Filhos, Mensagem: e.Mensagem})
	case orchestrator.EvFim:
		o.mu.Lock()
		j.fimEmitido = true
		o.mu.Unlock()
		o.fim(protocol.OrqFimParams{ID: j.id, Agente: j.nome, Projeto: j.projeto, Codigo: e.Codigo, Tentativas: e.Tentativas, Duracao: e.Duracao.Seconds(), Relatorio: e.Relatorio, Motivo: e.Motivo, Filhos: e.Filhos})
	}
}

// filhosDoFim escolhe a lista de filhos do orq.fim conforme o motivo gravado no meta.json.
func filhosDoFim(m metaArq) []string {
	if m.Motivo == orchestrator.MotivoFilhoFalhou {
		return m.FilhosFalhos
	}
	return m.FilhosOrfaos
}

func (o *Orq) perguntar(ctx context.Context, j *job, q orchestrator.Pergunta) (bool, string, bool) {
	o.mu.Lock()
	o.seq++
	d := &decisao{ch: make(chan decisaoResp, 1), params: protocol.OrqDecisaoParams{Geracao: o.geracao, ID: fmt.Sprintf("dec-%d", o.seq), Run: j.id, Agente: j.nome, Projeto: j.projeto, Pergunta: q.Pergunta, Opcoes: q.Opcoes}}
	o.decisoes[d.params.ID] = d
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.decisoes, d.params.ID)
		o.mu.Unlock()
	}()
	o.emitir(protocol.EventOrqPrecisaDecisao, j.projeto, j.nome, d.params)
	select {
	case r := <-d.ch:
		return r.allow, r.msg, r.encerra
	case <-ctx.Done():
		return false, "cancelado", false
	}
}

func (o *Orq) decidir(p protocol.RodarDecidirParams) error {
	if p.Geracao != "" && p.Geracao != o.geracao {
		return fmt.Errorf("geração %q não corresponde a este servidor", p.Geracao)
	}
	var allow bool
	switch strings.ToLower(strings.TrimSpace(p.Resposta)) {
	case "permitir", "sim", "s", "ok", "allow", "aprovar", "true":
		allow = true
	case "negar", "nao", "não", "n", "deny", "recusar", "false":
	default:
		return fmt.Errorf("resposta %q inválida: use \"permitir\" ou \"negar\"", p.Resposta)
	}
	// Confere o run antes de remover, sob a mesma trava: uma resposta com run errado
	// não consome a decisão de outra execução. Run vazio (cliente antigo) é aceito.
	o.mu.Lock()
	d := o.decisoes[p.ID]
	encerra := o.negarEncerra
	if d != nil && p.Run != "" && d.params.Run != p.Run {
		o.mu.Unlock()
		return fmt.Errorf("decisão %q não pertence à execução %q", p.ID, p.Run)
	}
	delete(o.decisoes, p.ID)
	o.mu.Unlock()
	if d == nil {
		return fmt.Errorf("decisão %q não encontrada (já respondida ou expirada)", p.ID)
	}
	if p.Encerrar != nil {
		encerra = *p.Encerrar
	}
	d.ch <- decisaoResp{allow, p.Mensagem, !allow && encerra}
	return nil
}

func (o *Orq) parar(p protocol.RodarPararParams) error {
	if p.ID == "" && p.Agente == "" {
		return fmt.Errorf("rodar.parar exige id ou agente")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, j := range o.jobs {
		if (p.ID != "" && j.id == p.ID) || (p.ID == "" && j.nome == p.Agente) {
			j.cancel()
			return nil
		}
	}
	return fmt.Errorf("nenhuma execução lançada por este servidor com esse id/agente")
}

func (o *Orq) listar(p protocol.RodarListarParams) protocol.RodarListarResult {
	dir, projeto := resolverPasta(p.CWD, p.Pasta)
	res := protocol.RodarListarResult{Agentes: []protocol.AgenteInfo{}, Decisoes: []protocol.OrqDecisaoParams{}}
	vistos := map[string]bool{}
	dirs := map[string]string{dir: projeto}
	o.mu.Lock()
	for d, pa := range o.pastas {
		if p.CWD == "" && p.Pasta == "" {
			dirs[d] = pa.projeto
		}
	}
	jobs := map[string]*job{}
	for _, j := range o.jobs {
		jobs[j.dir+"|"+j.nome] = j
	}
	for _, d := range o.decisoes {
		if p.Projeto == "" || d.params.Projeto == p.Projeto {
			res.Decisoes = append(res.Decisoes, d.params)
		}
	}
	o.mu.Unlock()
	for d, proj := range dirs {
		if p.Projeto != "" && proj != p.Projeto {
			continue
		}
		entries, _ := os.ReadDir(filepath.Join(d, "logs"))
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".meta.json") {
				continue
			}
			nome := strings.TrimSuffix(e.Name(), ".meta.json")
			m, ok := lerMeta(filepath.Join(d, "logs", e.Name()))
			if !ok {
				continue
			}
			info := protocol.AgenteInfo{Agente: nome, Projeto: proj, Motor: m.Motor, Modelo: m.Modelo, Tentativa: m.Tentativa, Inicio: m.Inicio, Fim: m.Fim, Codigo: m.Codigo, PID: m.PID, Log: filepath.Join(d, "logs", nome+".log")}
			switch {
			case m.Fim == "" && (vivo(m.PID) || jobs[d+"|"+nome] != nil):
				info.Estado = "rodando"
			case m.Fim == "":
				info.Estado = "interrompido"
			case m.Codigo != nil && *m.Codigo == 0:
				info.Estado = "concluido"
			default:
				info.Estado = "falhou"
			}
			if j := jobs[d+"|"+nome]; j != nil {
				info.ID = j.id
			}
			vistos[d+"|"+nome] = true
			res.Agentes = append(res.Agentes, info)
		}
	}
	for k, j := range jobs {
		if !vistos[k] && (p.Projeto == "" || j.projeto == p.Projeto) {
			res.Agentes = append(res.Agentes, protocol.AgenteInfo{ID: j.id, Agente: j.nome, Projeto: j.projeto, Estado: "aguardando"})
		}
	}
	sort.Slice(res.Agentes, func(a, b int) bool { return res.Agentes[a].Agente < res.Agentes[b].Agente })
	return res
}
