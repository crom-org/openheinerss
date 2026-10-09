package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Métodos de requisição do cliente para o servidor
const (
	MethodSessionCreate            = "session.create"
	MethodSessionPrompt            = "session.prompt"
	MethodSessionPermissionRespond = "session.permission_respond"
	MethodSessionAbort             = "session.abort"
	MethodSessionList              = "session.list"
	MethodSessionResume            = "session.resume"
	MethodDoctorCheck              = "doctor.check"
	MethodCatalogList              = "catalog.list"
	MethodMCPList                  = "mcp.list"
	MethodMCPAdd                   = "mcp.add"
	MethodHarnessRegister          = "harness.register"
	MethodRun                      = "run"
	MethodLimits                   = "limites"
	MethodInstanceIdentity         = "instancia.identidade"
	MethodContasListar             = "contas.listar"
	MethodContasAdicionar          = "contas.adicionar"
	MethodContasRenomear           = "contas.renomear"
	MethodContasRemover            = "contas.remover"
)

// MCPListParams parâmetros para mcp.list
type MCPListParams struct {
	CWD string `json:"cwd,omitempty"`
}

// MCPAddParams parâmetros para mcp.add
type MCPAddParams struct {
	CWD     string            `json:"cwd,omitempty"`
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Attachment representa anexos de imagem ou arquivos no prompt
type Attachment struct {
	MediaType string `json:"mediaType"` // ex: "image/png"
	Data      string `json:"data"`      // base64
}

// SessionOptions opções extras de configuração da sessão
type SessionOptions struct {
	Effort         string `json:"effort,omitempty"`         // "low", "medium", "high", "ultracode"
	PermissionMode string `json:"permissionMode,omitempty"` // "ask", "always_allow", "plan"
	SystemPrompt   string `json:"systemPrompt,omitempty"`
	// HarnessArgs vai intacto, na ordem, para o processo do harness (repasse de opções nativas).
	HarnessArgs []string               `json:"harnessArgs,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	// SemMCP desliga a entrega dos servidores de mcp.json ao harness; MCP escolhe quais (vazio = todos).
	SemMCP           bool     `json:"semMcp,omitempty"`
	MCP              []string `json:"mcp,omitempty"`
	PastasPermitidas []string `json:"pastasPermitidas,omitempty"`
	// ClassificarRisco liga (true) ou desliga (false) o classificador de risco nesta sessão;
	// ausente segue o padrão do servidor (desligado, salvo serve --classificar-risco).
	ClassificarRisco *bool `json:"classificarRisco,omitempty"`
}

// SessionCreateParams parâmetros para session.create
type SessionCreateParams struct {
	Papel    string            `json:"papel,omitempty"`
	Harness  string            `json:"harness"`            // "mock", "claude-code", "opencode", etc.
	Mode     string            `json:"mode,omitempty"`     // "sdk", "cli", etc.
	CWD      string            `json:"cwd"`                // Diretório de trabalho
	Provider string            `json:"provider,omitempty"` // "anthropic", "openrouter", "deepseek", etc.
	Model    string            `json:"model,omitempty"`    // Nome do modelo
	Env      map[string]string `json:"env,omitempty"`      // Variáveis de ambiente extras
	Options  SessionOptions    `json:"options,omitempty"`
	// Retomar continua uma conversa existente do harness: id nativo (claude/codex/opencode/agy) ou id de
	// sessão do openheinerss, de onde se lê o id nativo gravado. O aider não tem id e recusa o pedido.
	Retomar string `json:"retomar,omitempty"`
}

// SessionCreateResult retorno de session.create
type SessionCreateResult struct {
	SessionID  string   `json:"sessionId"`
	Harness    string   `json:"harness"`
	Mode       string   `json:"mode"`
	CWD        string   `json:"cwd"`
	Status     string   `json:"status"` // "ready", "running"
	Identidade Identity `json:"identidade"`
}

type Identity struct {
	Instancia    string `json:"instancia"`
	Base         string `json:"base"`
	ContaID      string `json:"contaId"`
	ContaIDFonte string `json:"contaIdFonte,omitempty"`
	ConfigFonte  string `json:"configFonte,omitempty"`
	ContaDir     string `json:"contaDir"`
}

type InstanceIdentityParams struct {
	Instancia string `json:"instancia"`
}

type ContasListarParams struct {
	CWD string `json:"cwd,omitempty"`
}
type ContasAdicionarParams struct {
	Harness      string `json:"harness"`
	Nome         string `json:"nome"`
	CWD          string `json:"cwd,omitempty"`
	IniciarLogin bool   `json:"iniciarLogin,omitempty"`
	Projeto      bool   `json:"projeto,omitempty"`
}
type ContasRenomearParams struct {
	Antigo  string `json:"antigo"`
	Novo    string `json:"novo"`
	CWD     string `json:"cwd,omitempty"`
	Projeto bool   `json:"projeto,omitempty"`
}
type ContasRemoverParams struct {
	Nome        string `json:"nome"`
	CWD         string `json:"cwd,omitempty"`
	Confirmar   bool   `json:"confirmar"`
	ApagarPasta bool   `json:"apagarPasta,omitempty"`
}

type SessionResumeParams struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd,omitempty"`
}

type SessionResumeResult = SessionCreateResult

// SessionPromptParams parâmetros para session.prompt
type SessionPromptParams struct {
	SessionID   string       `json:"sessionId"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"images,omitempty"`
}

// SessionPromptResult retorno de session.prompt
type SessionPromptResult struct {
	SessionID string `json:"sessionId"`
	Accepted  bool   `json:"accepted"`
}

// PermissionRespondParams parâmetros para session.permission_respond
type PermissionRespondParams struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"` // "allow" ou "deny"
	Message   string `json:"message,omitempty"`
}

// PermissionRespondResult retorno de session.permission_respond
type PermissionRespondResult struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Resolved  bool   `json:"resolved"`
}

// SessionAbortParams parâmetros para session.abort
type SessionAbortParams struct {
	SessionID string `json:"sessionId"`
}

// SessionAbortResult retorno de session.abort
type SessionAbortResult struct {
	SessionID string `json:"sessionId"`
	Aborted   bool   `json:"aborted"`
}

// SessionInfo resumo de uma sessão para listagem
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Harness   string `json:"harness"`
	Mode      string `json:"mode"`
	CWD       string `json:"cwd"`
	Running   bool   `json:"running"`
	CreatedAt string `json:"createdAt"`
}

// SessionListResult retorno de session.list
type SessionListResult struct {
	Sessions []SessionInfo `json:"sessions"`
}

// DoctorCheckParams parâmetros para doctor.check
type DoctorCheckParams struct {
	Harness string `json:"harness,omitempty"`
}

// DoctorItem resultado da checagem de um componente do ambiente
type DoctorItem struct {
	Name         string `json:"name"`
	Installed    bool   `json:"installed"`
	Version      string `json:"version,omitempty"`
	Path         string `json:"path,omitempty"`
	Required     bool   `json:"required"`
	SuggestedFix string `json:"suggestedFix,omitempty"`
}

// DoctorCheckResult retorno de doctor.check
type DoctorCheckResult struct {
	Status  string       `json:"status"` // "ok", "warning", "error"
	Items   []DoctorItem `json:"items"`
	Summary string       `json:"summary"`
}

// ProviderInfo informação sobre provedor compatível
type ProviderInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Endpoint    string   `json:"endpoint,omitempty"`
	Models      []string `json:"models,omitempty"`
	RequiresKey bool     `json:"requiresKey"`
}

// HarnessCatalogItem item do catálogo de harnesses
type HarnessCatalogItem struct {
	ID                 string         `json:"id"`
	DisplayName        string         `json:"displayName"`
	SupportedModes     []string       `json:"supportedModes"`
	SupportedProtocols []string       `json:"supportedProtocols"`
	DefaultProviders   []ProviderInfo `json:"defaultProviders,omitempty"`
	Origin             string         `json:"origin,omitempty"`
	// MCP diz como os servidores de mcp.json chegam ao harness (ou por que não chegam).
	MCP string `json:"mcp,omitempty"`
}

// CatalogListResult retorno de catalog.list
type CatalogListResult struct {
	Harnesses []HarnessCatalogItem `json:"harnesses"`
}

// HarnessRegisterParams descreve um harness custom registrável em tempo de execução.
type HarnessRegisterParams struct {
	Name        string            `json:"name" yaml:"name"`
	Base        string            `json:"base,omitempty" yaml:"base,omitempty"`
	DisplayName string            `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Model       string            `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt      string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	FinishRegex string            `json:"finishRegex,omitempty" yaml:"finishRegex,omitempty"`
	QuotaRegex  string            `json:"quotaRegex,omitempty" yaml:"quotaRegex,omitempty"`
	Reserva     []string          `json:"reserva,omitempty" yaml:"reserva,omitempty"`
}

// RunParams descreve uma missão no mesmo formato do comando openheinerss rodar.
type RunParams struct {
	Nome    string `json:"nome"`
	Motor   string `json:"motor"`
	Modelo  string `json:"modelo,omitempty"`
	Esforco string `json:"esforco,omitempty"`
	Prompt  string `json:"prompt,omitempty"` // arquivo de prompt
	Texto   string `json:"texto,omitempty"`  // prompt em texto (vale no lugar do arquivo)
	// Retomar aceita true (continuar o agente: preserva log e acrescenta o texto de continuação) ou
	// uma string (id nativo da conversa do harness, ou id de sessão do openheinerss, a retomar).
	Retomar     Retomada `json:"retomar,omitempty"`
	Pasta       string   `json:"pasta,omitempty"`
	BranchBase  string   `json:"branchBase,omitempty"`
	CargaMax    float64  `json:"cargaMax,omitempty"`
	CargaAbaixo float64  `json:"quandoCargaAbaixo,omitempty"`
	MaxAgentes  int      `json:"maxAgentes,omitempty"`
	Tentativas  int      `json:"tentativas,omitempty"`
	CotaMax     float64  `json:"cotaMax,omitempty"`
	// HarnessArgs vai intacto, na ordem, para o processo do harness.
	HarnessArgs      []string `json:"harnessArgs,omitempty"`
	PastasPermitidas []string `json:"pastasPermitidas,omitempty"`
	// FilhosObrigatorios: o pai termina com código 4 ("filho falhou") se um filho falhar.
	FilhosObrigatorios bool   `json:"filhosObrigatorios,omitempty"`
	CWD                string `json:"cwd,omitempty"`
	// SemTrocaConta: acima do limiar de cota, só pula a instância (sem procurar outra conta da mesma base).
	SemTrocaConta bool `json:"semTrocaConta,omitempty"`
}

// Retomada é o campo retomar de run/rodar: booleano (continuar o agente) ou id da conversa a retomar.
type Retomada struct {
	Continuar bool
	ID        string
}

func (r *Retomada) UnmarshalJSON(b []byte) error {
	*r = Retomada{}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case nil:
	case bool:
		r.Continuar = x
	case string:
		r.ID = strings.TrimSpace(x)
	default:
		return fmt.Errorf("retomar deve ser booleano ou string com o id da conversa")
	}
	return nil
}

func (r Retomada) MarshalJSON() ([]byte, error) {
	if r.ID != "" {
		return json.Marshal(r.ID)
	}
	return json.Marshal(r.Continuar)
}

type LimitsParams struct {
	Atualizar bool `json:"atualizar,omitempty"`
	Forcar    bool `json:"forcar,omitempty"`
}

type RunResult struct {
	Nome       string `json:"nome"`
	WorkDir    string `json:"workDir"`
	Log        string `json:"log"`
	Meta       string `json:"meta"`
	Tentativas int    `json:"tentativas"`
	Codigo     int    `json:"codigo"`
}
