package protocol

// Métodos de orquestração (rodar, limites, harnesses e assinatura de eventos).
const (
	MethodRodarIniciar   = "rodar.iniciar"
	MethodRodarListar    = "rodar.listar"
	MethodRodarParar     = "rodar.parar"
	MethodRodarDecidir   = "rodar.decidir"
	MethodRodarSeco      = "rodar.seco"
	MethodRodarMensagem  = "rodar.mensagem"
	MethodLimitesObter   = "limites.obter"
	MethodHarnessListar  = "harness.listar"
	MethodEventosAssinar = "eventos.assinar"

	MethodHarnessCapacidades       = "harness.capacidades"
	MethodHarnessVersoes           = "harness.versoes"
	MethodHarnessAtualizar         = "harness.atualizar"
	MethodHarnessVoltar            = "harness.voltar"
	MethodHarnessComandos          = "harness.comandos"
	MethodHarnessComandosAnotar    = "harness.comandos.anotar"
	MethodHarnessComandosConfirmar = "harness.comandos.confirmar"
)

// HarnessCapacidadesParams pede a matriz de capacidades de um harness ou instância; sem Harness, devolve as bases.
type HarnessCapacidadesParams struct {
	Harness string `json:"harness,omitempty"`
}

// HarnessVersoesParams pede a versão instalada/disponível de uma base; sem Harness, devolve todas.
type HarnessVersoesParams struct {
	Harness string `json:"harness,omitempty"`
}

// HarnessAtualizarParams pede a atualização de uma base (ou instância dela). Seco só mostra o plano.
// Esperar espera os usos terminarem (até EsperaSeg segundos); Para fixa a versão; SimularFalhaTeste
// ("versao" ou "login") força a falha do teste para exercitar o diagnóstico. A resposta chega quando
// tudo termina e traz diagnostico/recomendacao; o openheinerss nunca volta sozinho (veja harness.voltar).
type HarnessAtualizarParams struct {
	Harness           string `json:"harness"`
	Seco              bool   `json:"seco,omitempty"`
	Esperar           bool   `json:"esperar,omitempty"`
	EsperaSeg         int    `json:"esperaSeg,omitempty"`
	Para              string `json:"para,omitempty"`
	Forcar            bool   `json:"forcar,omitempty"`
	SimularFalhaTeste string `json:"simularFalhaTeste,omitempty"`
}

// HarnessVoltarParams pede a volta explícita de uma base para a versão anterior guardada (ou para
// Versao). Roda o teste depois e informa o resultado.
type HarnessVoltarParams struct {
	Harness   string `json:"harness"`
	Versao    string `json:"versao,omitempty"`
	Seco      bool   `json:"seco,omitempty"`
	Esperar   bool   `json:"esperar,omitempty"`
	EsperaSeg int    `json:"esperaSeg,omitempty"`
}

// HarnessComandosParams pede os comandos nativos de um harness ou instância. CWD (opcional) é a
// pasta do projeto onde procurar comandos/skills do harness (ex.: .claude/commands).
// Comando e Anotacao valem para harness.comandos.anotar/confirmar.
type HarnessComandosParams struct {
	Harness  string `json:"harness"`
	CWD      string `json:"cwd,omitempty"`
	Comando  string `json:"comando,omitempty"`
	Anotacao string `json:"anotacao,omitempty"`
}

// Eventos de orquestração emitidos para quem chamou eventos.assinar.
const (
	EventOrqInicio         = "orq.inicio"
	EventOrqProgresso      = "orq.progresso"
	EventOrqFim            = "orq.fim"
	EventOrqErro           = "orq.erro"
	EventOrqPrecisaDecisao = "orq.precisa_decisao"
	EventOrqFilhosOrfaos   = "orq.filhos_orfaos"
	EventOrqMensagem       = "orq.mensagem"
	EventLimitesAtualizado = "limites.atualizado"
	// EventHarnessAtualizado é emitido ao fim de cada harness.atualizar/harness.voltar real (não na
	// prévia); o payload é o próprio resultado (acao, antes, depois, resultado, teste, diagnostico,
	// recomendacao, anterior).
	EventHarnessAtualizado = "harness.atualizado"
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
	// TrocaDe e TrocaMotivo aparecem quando a tentativa começou por troca de conta por passar do limiar de cota.
	TrocaDe     string `json:"trocaDe,omitempty"`
	TrocaMotivo string `json:"trocaMotivo,omitempty"`
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
	Motivo     string  `json:"motivo,omitempty"` // "negado", "filho falhou" ou "filhos órfãos"
	// Filhos lista os agentes filhos que falharam (motivo "filho falhou") ou ficaram rodando
	// depois do fim do pai (motivo "filhos órfãos").
	Filhos []string `json:"filhos,omitempty"`
}

// OrqFilhosOrfaosParams payload de orq.filhos_orfaos: o pai terminou com filhos ainda rodando.
type OrqFilhosOrfaosParams struct {
	Geracao  string   `json:"geracao"`
	ID       string   `json:"id,omitempty"`
	Agente   string   `json:"agente"`
	Projeto  string   `json:"projeto"`
	Filhos   []string `json:"filhos"`
	Mensagem string   `json:"mensagem"`
}

// OrqMensagemParams payload de orq.mensagem: Estado "recebida", "entregue" (com Modo "vivo" ou "retomada")
// ou "nao_entregue" (o agente terminou antes da entrega). Mensagem é o id devolvido por rodar.mensagem.
type OrqMensagemParams struct {
	Geracao  string `json:"geracao"`
	ID       string `json:"id,omitempty"`
	Agente   string `json:"agente"`
	Projeto  string `json:"projeto"`
	Mensagem string `json:"mensagem"`
	Estado   string `json:"estado"`
	Modo     string `json:"modo,omitempty"`
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

// RodarMensagemParams entrega Texto ao agente VIVO lançado por `rodar` (CWD/Pasta como em rodar.listar).
type RodarMensagemParams struct {
	Agente  string `json:"agente"`
	Texto   string `json:"texto"`
	CWD     string `json:"cwd,omitempty"`
	Pasta   string `json:"pasta,omitempty"`
	Projeto string `json:"projeto,omitempty"`
}

// RodarMensagemResult: Status e Recibo valem "entregue" ou "pendente" (ficam pendentes até o runner
// entregar; o orq.mensagem avisa quando isso acontece). Modo é "vivo" ou "retomada" quando entregue.
type RodarMensagemResult struct {
	ID      string `json:"id"`
	Agente  string `json:"agente"`
	Projeto string `json:"projeto"`
	Status  string `json:"status"`
	Recibo  string `json:"recibo"`
	Modo    string `json:"modo,omitempty"`
	Em      string `json:"em"`
}

// RodarDecidirParams responde um orq.precisa_decisao. Resposta: "permitir" ou "negar".
type RodarDecidirParams struct {
	Geracao  string `json:"geracao,omitempty"`
	Run      string `json:"run,omitempty"` // id da execução; se vier, precisa bater com o da decisão
	ID       string `json:"id"`
	Resposta string `json:"resposta"`
	Mensagem string `json:"mensagem,omitempty"`
	// Encerrar, numa negação, termina a execução (código 3, motivo "negado") sem nova tentativa.
	// Ausente: vale `serve --negar-encerra`.
	Encerrar *bool `json:"encerrar,omitempty"`
}

// EventosAssinarParams filtra os eventos; vazio recebe todos. Pasta/CWD indicam o que observar.
type EventosAssinarParams struct {
	Projeto string `json:"projeto,omitempty"`
	Agente  string `json:"agente,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Pasta   string `json:"pasta,omitempty"`
}
