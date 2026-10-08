package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/crom-org/openheinerss/pkg/doctor"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/orchestrator"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/session"
)

// Router processa requisições JSON-RPC e delega para o SessionManager ou subsistemas
type Router struct {
	manager *session.Manager
	orq     *Orq
}

// NewRouter cria um novo despachante de métodos
func NewRouter(m *session.Manager) *Router {
	return &Router{manager: m, orq: newOrq()}
}

// HandleRequest executa a lógica do método solicitado e devolve a resposta JSON-RPC
func (r *Router) HandleRequest(ctx context.Context, req protocol.Request) protocol.Response {
	switch req.Method {
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
		res, err := orchestrator.Run(ctx, cwd, orchestrator.Options{Name: p.Nome, Motor: p.Motor, Model: p.Modelo, Effort: p.Esforco, PromptFile: p.Prompt, PromptText: p.Texto, Retomar: p.Retomar, AgentsDir: p.Pasta, BranchBase: p.BranchBase, MaxLoad: p.CargaMax, MaxAgents: p.MaxAgentes, Attempts: p.Tentativas, QuotaMax: p.CotaMax})
		if err != nil {
			return protocol.NewErrorResponse(req.ID, protocol.CodeInternalError, err.Error(), res)
		}
		return protocol.NewResponse(req.ID, protocol.RunResult{Nome: res.Name, WorkDir: res.WorkDir, Log: res.LogFile, Meta: res.MetaFile, Tentativas: res.Attempts, Codigo: res.Code})

	case protocol.MethodLimits, protocol.MethodLimitesObter:
		return protocol.NewResponse(req.ID, limites.Obter())

	case protocol.MethodHarnessListar:
		return protocol.NewResponse(req.ID, protocol.CatalogListResult{Harnesses: harness.ListCatalog()})

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
