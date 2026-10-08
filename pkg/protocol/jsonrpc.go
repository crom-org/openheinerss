package protocol

import "encoding/json"

// Versão canônica JSON-RPC suportada
const JSONRPCVersion = "2.0"

// Standard e Custom Error Codes
const (
	CodeParseError               = -32700
	CodeInvalidRequest           = -32600
	CodeMethodNotFound           = -32601
	CodeInvalidParams            = -32602
	CodeInternalError            = -32603
	CodeSessionNotFound          = 4001
	CodeHarnessNotFound          = 4002
	CodeHarnessDependencyMissing = 4010
	CodeAuthTokenMissing         = 4011
	CodePermissionRejected       = 4020
	CodeProcessCrashed           = 4030
)

// Request representa uma chamada RPC cliente -> servidor
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response representa a resposta com sucesso para um Request com ID
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Geracao string      `json:"geracao"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// Notification representa um evento emitido assincronamente (sem campo id)
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	// Geracao identifica o processo do serve que emitiu o evento (vai em todos os eventos do serve).
	Geracao string      `json:"geracao,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// RPCError representa o objeto de erro padronizado JSON-RPC 2.0
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

// ErrorData traz informações adicionais ricas (ex: pré-requisito ausente e comando de conserto)
type ErrorData struct {
	Harness      string   `json:"harness,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	SuggestedFix string   `json:"suggestedFix,omitempty"`
}

// NewResponse cria uma resposta com sucesso
func NewResponse(id interface{}, result interface{}) Response {
	return Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse cria uma resposta de erro
func NewErrorResponse(id interface{}, code int, message string, data interface{}) Response {
	return Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewNotification cria um evento unidirecional
func NewNotification(method string, params interface{}) Notification {
	return Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	}
}
