package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/crom-org/openheinerss/pkg/capacidades"
	"github.com/crom-org/openheinerss/pkg/comandos"
	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/contas"
	"github.com/crom-org/openheinerss/pkg/doctor"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/orchestrator"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

func novaGeracao() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("geracao-%d", os.Getpid())
	}
	return fmt.Sprintf("geracao-%x", b)
}

// IniciarAtualizacaoLimites mantém limites frescos para clientes inscritos.
func (r *Router) IniciarAtualizacaoLimites(ctx context.Context, intervalo time.Duration) {
	if intervalo <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(intervalo)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if res, err := limites.Atualizar(ctx, limites.AtualizarOpcoes{}); err == nil {
					r.orq.emitir(protocol.EventLimitesAtualizado, "", "", res)
				}
			}
		}
	}()
}

// Router processa requisições JSON-RPC e delega para o SessionManager ou subsistemas
type Router struct {
	manager *session.Manager
	orq     *Orq
	geracao string
}

func contasRPCDir(cwd string) (string, error) {
	if d, err := config.ConfigDir(); err != nil {
		return "", err
	} else if d != "" {
		return filepath.Join(d, "harnesses"), nil
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, ".openheinerss", "harnesses"), nil
}

func contasRPCDirs(cwd string) ([]string, string, error) {
	if d, err := config.ConfigDir(); err != nil {
		return nil, "", err
	} else if d != "" {
		h := filepath.Join(d, "harnesses")
		return []string{h}, h, nil
	}
	globais, err := config.UserHarnessDirs()
	if err != nil {
		return nil, "", err
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return globais, filepath.Join(cwd, ".openheinerss", "harnesses"), nil
}

// NewRouter cria um novo despachante de métodos
func NewRouter(m *session.Manager) *Router {
	return NewRouterWithMaxAgents(m, 0)
}

// NewRouterWithMaxAgents cria um servidor com uma geração nova e limite opcional.
func NewRouterWithMaxAgents(m *session.Manager, maxAgents int) *Router {
	o := newOrq(maxAgents)
	return &Router{manager: m, orq: o, geracao: o.geracao}
}

// SetNegarEncerra liga serve --negar-encerra: uma negação em rodar.decidir sem "encerrar" termina a execução.
func (r *Router) SetNegarEncerra(v bool) {
	r.orq.mu.Lock()
	r.orq.negarEncerra = v
	r.orq.mu.Unlock()
}

// HandleRequest executa a lógica do método solicitado e devolve a resposta JSON-RPC
func (r *Router) HandleRequest(ctx context.Context, req protocol.Request) (response protocol.Response) {
	defer func() { response.Geracao = r.geracao }()
	switch req.Method {
	case protocol.MethodInstanceIdentity:
		var p protocol.InstanceIdentityParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Instancia == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para instancia.identidade", nil)
		}
		res, err := identidade.Para(p.Instancia, nil)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)
	case protocol.MethodContasListar:
		var p protocol.ContasListarParams
		_ = json.Unmarshal(req.Params, &p)
		globais, projeto, err := contasRPCDirs(p.CWD)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		res, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)
	case protocol.MethodContasAdicionar:
		var p protocol.ContasAdicionarParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Harness == "" || p.Nome == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para contas.adicionar", nil)
		}
		globais, projeto, err := contasRPCDirs(p.CWD)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		dir := globais[0]
		if p.Projeto {
			dir = projeto
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		res, err := contas.Adicionar(dir, p.Harness, p.Nome)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)
	case protocol.MethodContasRenomear:
		var p protocol.ContasRenomearParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Antigo == "" || p.Novo == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para contas.renomear", nil)
		}
		globais, projeto, err := contasRPCDirs(p.CWD)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		items, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		var dir string
		for _, item := range items {
			if item.Instancia == p.Antigo {
				dir = filepath.Dir(item.Arquivo)
				break
			}
		}
		if dir == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "conta não encontrada", nil)
		}
		res, err := contas.Renomear(dir, p.Antigo, p.Novo)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)
	case protocol.MethodContasRemover:
		var p protocol.ContasRemoverParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Nome == "" || !p.Confirmar {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "contas.remover exige nome e confirmar: true", nil)
		}
		globais, projeto, err := contasRPCDirs(p.CWD)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		items, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		var dir string
		for _, item := range items {
			if item.Instancia == p.Nome {
				dir = filepath.Dir(item.Arquivo)
				break
			}
		}
		if dir == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "conta não encontrada", nil)
		}
		c, err := contas.Remover(dir, p.Nome)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		if p.ApagarPasta {
			if err := os.RemoveAll(c.ContaDir); err != nil {
				return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
			}
		}
		return protocol.NewResponse(req.ID, c)
	case protocol.MethodSessionCreate:
		var params protocol.SessionCreateParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para session.create", nil)
		}
		res, err := r.manager.CreateSession(ctx, params)
		if err != nil {
			return errorToResponse(req.ID, err)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodSessionPrompt:
		var params protocol.SessionPromptParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para session.prompt", nil)
		}
		res, err := r.manager.PromptSession(ctx, params)
		if err != nil {
			return errorToResponse(req.ID, err)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodSessionPermissionRespond:
		var params protocol.PermissionRespondParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para session.permission_respond", nil)
		}
		res, err := r.manager.RespondPermission(ctx, params)
		if err != nil {
			return errorToResponse(req.ID, err)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodSessionAbort:
		var params protocol.SessionAbortParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para session.abort", nil)
		}
		res, err := r.manager.AbortSession(ctx, params.SessionID)
		if err != nil {
			return errorToResponse(req.ID, err)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodSessionList:
		list := r.manager.ListSessions()
		return protocol.NewResponse(req.ID, protocol.SessionListResult{Sessions: list})

	case protocol.MethodSessionResume:
		var params protocol.SessionResumeParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para session.resume", nil)
		}
		res, err := r.manager.ResumeSession(ctx, params)
		if err != nil {
			return errorToResponse(req.ID, err)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodCatalogList:
		catalog := harness.ListCatalog()
		return protocol.NewResponse(req.ID, protocol.CatalogListResult{Harnesses: catalog})

	case protocol.MethodHarnessRegister:
		var p protocol.HarnessRegisterParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para harness.register", nil)
		}
		err := harness.RegisterCustom(harness.CustomSpec{Name: p.Name, Base: p.Base, DisplayName: p.DisplayName, Command: p.Command, Args: p.Args, Env: p.Env, Model: p.Model, Prompt: p.Prompt, FinishRegex: p.FinishRegex, QuotaRegex: p.QuotaRegex, Reserva: p.Reserva})
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, map[string]string{"name": p.Name, "status": "registered"})

	case protocol.MethodRun:
		var p protocol.RunParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para run", nil)
		}
		cwd := p.CWD
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		res, err := orchestrator.Run(ctx, cwd, orchestrator.Options{Name: p.Nome, Motor: p.Motor, Model: p.Modelo, Effort: p.Esforco, PromptFile: p.Prompt, PromptText: p.Texto, Retomar: p.Retomar.Continuar, SessaoNativa: p.Retomar.ID, SemTrocaConta: p.SemTrocaConta, AgentsDir: p.Pasta, BranchBase: p.BranchBase, MaxLoad: p.CargaMax, MaxAgents: p.MaxAgentes, Attempts: p.Tentativas, QuotaMax: p.CotaMax, HarnessArgs: p.HarnessArgs})
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), res)
		}
		return protocol.NewResponse(req.ID, protocol.RunResult{Nome: res.Name, WorkDir: res.WorkDir, Log: res.LogFile, Meta: res.MetaFile, Tentativas: res.Attempts, Codigo: res.Code})

	case protocol.MethodLimits, protocol.MethodLimitesObter:
		var p protocol.LimitsParams
		_ = json.Unmarshal(req.Params, &p)
		if p.Atualizar || p.Forcar {
			res, err := limites.Atualizar(ctx, limites.AtualizarOpcoes{Forcar: true})
			if err != nil {
				return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
			}
			return protocol.NewResponse(req.ID, res)
		}
		return protocol.NewResponse(req.ID, limites.Obter())

	case protocol.MethodHarnessListar:
		return protocol.NewResponse(req.ID, protocol.CatalogListResult{Harnesses: harness.ListCatalog()})

	case protocol.MethodHarnessCapacidades:
		var p protocol.HarnessCapacidadesParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para harness.capacidades", nil)
			}
		}
		if p.Harness == "" {
			return protocol.NewResponse(req.ID, map[string]interface{}{"harnesses": capacidades.Todas()})
		}
		res, err := capacidades.Para(p.Harness)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodHarnessComandos, protocol.MethodHarnessComandosAnotar, protocol.MethodHarnessComandosConfirmar:
		var p protocol.HarnessComandosParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Harness == "" {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para "+req.Method+": informe harness", nil)
		}
		var (
			res interface{}
			err error
		)
		switch req.Method {
		case protocol.MethodHarnessComandos:
			res, err = comandos.Listar(p.Harness, p.CWD)
		case protocol.MethodHarnessComandosAnotar:
			res, err = comandos.Anotar(p.Harness, p.Comando, p.Anotacao, p.CWD)
		default:
			res, err = comandos.Confirmar(p.Harness, p.Comando, p.CWD)
		}
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodRodarIniciar:
		var p protocol.RodarIniciarParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para rodar.iniciar", nil)
		}
		res, err := r.orq.iniciar(p)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodRodarSeco:
		var p protocol.RunParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para rodar.seco", nil)
		}
		cwd := p.CWD
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		res, err := orchestrator.Seco(ctx, cwd, orchestrator.Options{Name: p.Nome, Motor: p.Motor, Model: p.Modelo, Effort: p.Esforco, PromptFile: p.Prompt, PromptText: p.Texto, Retomar: p.Retomar.Continuar, AgentsDir: p.Pasta, BranchBase: p.BranchBase, MaxLoad: p.CargaMax, WhenLoadBelow: p.CargaAbaixo, MaxAgents: p.MaxAgentes, Attempts: p.Tentativas, QuotaMax: p.CotaMax, HarnessArgs: p.HarnessArgs})
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, res)

	case protocol.MethodRodarListar:
		var p protocol.RodarListarParams
		_ = json.Unmarshal(req.Params, &p)
		return protocol.NewResponse(req.ID, r.orq.listar(p))

	case protocol.MethodRodarParar:
		var p protocol.RodarPararParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para rodar.parar", nil)
		}
		if err := r.orq.parar(p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, map[string]interface{}{"parando": true})

	case protocol.MethodRodarDecidir:
		var p protocol.RodarDecidirParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para rodar.decidir", nil)
		}
		if err := r.orq.decidir(p); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, map[string]interface{}{"id": p.ID, "resposta": p.Resposta})

	case protocol.MethodEventosAssinar:
		c, _ := ctx.Value(ctxKey{}).(*conexao)
		if c == nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, "eventos.assinar exige uma conexão com saída de eventos", nil)
		}
		var p protocol.EventosAssinarParams
		_ = json.Unmarshal(req.Params, &p)
		return protocol.NewResponse(req.ID, r.orq.assinar(c, p))

	case protocol.MethodDoctorCheck:
		var params protocol.DoctorCheckParams
		_ = json.Unmarshal(req.Params, &params)
		doc := doctor.CheckEnvironment(params.Harness)
		return protocol.NewResponse(req.ID, doc)

	case protocol.MethodMCPList:
		var params protocol.MCPListParams
		_ = json.Unmarshal(req.Params, &params)
		if params.CWD == "" {
			params.CWD = "."
		}
		list, err := mcp.GetHub().ListServers(params.CWD)
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, map[string]interface{}{"servers": list})

	case protocol.MethodMCPAdd:
		var params protocol.MCPAddParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInvalidParams, "Parâmetros inválidos para mcp.add", nil)
		}
		if params.CWD == "" {
			params.CWD = "."
		}
		cfg := mcp.ServerConfig{
			Command: params.Command,
			Args:    params.Args,
			Env:     params.Env,
			URL:     params.URL,
		}
		if err := mcp.GetHub().RegisterServer(params.CWD, params.Name, cfg); err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), nil)
		}
		return protocol.NewResponse(req.ID, map[string]interface{}{"status": "success", "name": params.Name})

	default:
		return protocol.NewErrorResponse(req.ID, protocol.CodeMethodNotFound, fmt.Sprintf("Método '%s' não encontrado", methodResumo(req.Method)), nil)
	}
}

func errorToResponse(id interface{}, err error) protocol.Response {
	if rpcErr, ok := err.(*protocol.RPCError); ok {
		return protocol.NewErrorResponse(id, rpcErr.Code, rpcErr.Message, rpcErr.Data)
	}
	return protocol.NewErrorResponse(id, protocol.CodeInternalError, err.Error(), nil)
}

func methodResumo(method string) string {
	r := []rune(method)
	if len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return method
}
