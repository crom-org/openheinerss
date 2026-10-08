package protocol

// Métodos de orquestração (rodar, limites, harnesses e assinatura de eventos).
const (
	MethodRodarIniciar   = "rodar.iniciar"
	MethodRodarListar    = "rodar.listar"
	MethodRodarParar     = "rodar.parar"
	MethodRodarDecidir   = "rodar.decidir"
	MethodLimitesObter   = "limites.obter"
	MethodHarnessListar  = "harness.listar"
	MethodEventosAssinar = "eventos.assinar"
)

// Eventos de orquestração emitidos para quem chamou eventos.assinar.
const (
	EventOrqInicio         = "orq.inicio"
	EventOrqProgresso      = "orq.progresso"
	EventOrqFim            = "orq.fim"
	EventOrqErro           = "orq.erro"
	EventOrqPrecisaDecisao = "orq.precisa_decisao"
)

// OrqInicioParams payload de orq.inicio (um por tentativa).
type OrqInicioParams struct {
	Geracao   string `json:"geracao"`
	ID        string `json:"id,omitempty"`
	Agente    string `json:"agente"`
	Projeto   string `json:"projeto"`
	Motor     string `json:"motor"`
	Modelo    string `json:"modelo"`
	Tentativa int    `json:"tentativa"`
	Worktree  string `json:"worktree"`
}

// OrqProgressoParams payload de orq.progresso.
type OrqProgressoParams struct {
	Geracao string `json:"geracao"`
	ID      string `json:"id,omitempty"`
	Agente  string `json:"agente"`
	Projeto string `json:"projeto"`
	Resumo  string `json:"resumo"`
}

// OrqFimParams payload de orq.fim. Duracao em segundos.
type OrqFimParams struct {
	Geracao    string  `json:"geracao"`
	ID         string  `json:"id,omitempty"`
	Agente     string  `json:"agente"`
	Projeto    string  `json:"projeto"`
	Codigo     int     `json:"codigo"`
	Tentativas int     `json:"tentativas"`
	Duracao    float64 `json:"duracao"`
	Relatorio  string  `json:"relatorio,omitempty"`
}

// OrqErroParams payload de orq.erro.
type OrqErroParams struct {
	Geracao  string `json:"geracao"`
	ID       string `json:"id,omitempty"`
	Agente   string `json:"agente"`
	Projeto  string `json:"projeto"`
	Mensagem string `json:"mensagem"`
	Cota     bool   `json:"cota"`
}

// OrqDecisaoParams payload de orq.precisa_decisao.
type OrqDecisaoParams struct {
	Geracao  string   `json:"geracao"`
	ID       string   `json:"id"`
	Run      string   `json:"run"`
	Agente   string   `json:"agente"`
	Projeto  string   `json:"projeto"`
	Pergunta string   `json:"pergunta"`
	Opcoes   []string `json:"opcoes"`
}

// RodarIniciarParams usa as mesmas opções do CLI rodar, mais o nome do projeto (padrão: nome da raiz git).
type RodarIniciarParams struct {
	RunParams
	Projeto string `json:"projeto,omitempty"`
}

// RodarIniciarResult devolve o id da execução, que vale nos eventos e em rodar.parar.
type RodarIniciarResult struct {
	Geracao string `json:"geracao"`
	ID      string `json:"id"`
	Agente  string `json:"agente"`
	Projeto string `json:"projeto"`
}

// RodarListarParams filtra e aponta a pasta de agentes (padrão: raiz git de cwd + .claude/agentes).
type RodarListarParams struct {
	CWD     string `json:"cwd,omitempty"`
	Pasta   string `json:"pasta,omitempty"`
	Projeto string `json:"projeto,omitempty"`
}

// AgenteInfo estado de um agente. Estado: aguardando, rodando, concluido, falhou ou interrompido.
type AgenteInfo struct {
	ID        string `json:"id,omitempty"`
	Agente    string `json:"agente"`
	Projeto   string `json:"projeto"`
	Estado    string `json:"estado"`
	Motor     string `json:"motor,omitempty"`
	Modelo    string `json:"modelo,omitempty"`
	Tentativa int    `json:"tentativa,omitempty"`
	Inicio    string `json:"inicio,omitempty"`
	Fim       string `json:"fim,omitempty"`
	Codigo    *int   `json:"codigo,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Log       string `json:"log,omitempty"`
}

// RodarListarResult lista agentes e decisões pendentes.
type RodarListarResult struct {
	Agentes  []AgenteInfo       `json:"agentes"`
	Decisoes []OrqDecisaoParams `json:"decisoes"`
}

// RodarPararParams identifica a execução por id ou por agente.
type RodarPararParams struct {
	ID     string `json:"id,omitempty"`
	Agente string `json:"agente,omitempty"`
}

// RodarDecidirParams responde um orq.precisa_decisao. Resposta: "permitir" ou "negar".
type RodarDecidirParams struct {
	Geracao  string `json:"geracao,omitempty"`
	Run      string `json:"run"`
	ID       string `json:"id"`
	Resposta string `json:"resposta"`
	Mensagem string `json:"mensagem,omitempty"`
}

// EventosAssinarParams filtra os eventos; vazio recebe todos. Pasta/CWD indicam o que observar.
type EventosAssinarParams struct {
	Projeto string `json:"projeto,omitempty"`
	Agente  string `json:"agente,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Pasta   string `json:"pasta,omitempty"`
}
