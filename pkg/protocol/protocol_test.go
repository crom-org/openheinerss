package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestJSONRPCSerialization(t *testing.T) {
	// 1. Test Request
	req := protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      1,
		Method:  protocol.MethodSessionCreate,
		Params:  json.RawMessage(`{"harness":"mock"}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("falha ao serializar request: %v", err)
	}

	var parsedReq protocol.Request
	if err := json.Unmarshal(data, &parsedReq); err != nil {
		t.Fatalf("falha ao desserializar request: %v", err)
	}
	if parsedReq.Method != protocol.MethodSessionCreate {
		t.Errorf("esperava método %s, obteve %s", protocol.MethodSessionCreate, parsedReq.Method)
	}

	// 2. Test Response
	res := protocol.NewResponse(1, map[string]string{"status": "ok"})
	resData, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("falha ao serializar response: %v", err)
	}
	var parsedRes protocol.Response
	if err := json.Unmarshal(resData, &parsedRes); err != nil {
		t.Fatalf("falha ao desserializar response: %v", err)
	}
	if parsedRes.Error != nil {
		t.Errorf("não esperava erro na resposta")
	}

	// 3. Test Error Response com dados ricos
	errData := protocol.ErrorData{
		Harness:      "claude-code",
		Mode:         "sdk",
		Missing:      []string{"node"},
		SuggestedFix: "Instale o Node.js",
	}
	errRes := protocol.NewErrorResponse(2, protocol.CodeHarnessDependencyMissing, "dependência faltando", errData)
	errBytes, err := json.Marshal(errRes)
	if err != nil {
		t.Fatalf("falha ao serializar erro: %v", err)
	}
	var parsedErrRes protocol.Response
	if err := json.Unmarshal(errBytes, &parsedErrRes); err != nil {
		t.Fatalf("falha ao desserializar erro: %v", err)
	}
	if parsedErrRes.Error == nil || parsedErrRes.Error.Code != protocol.CodeHarnessDependencyMissing {
		t.Errorf("código de erro incorreto: %+v", parsedErrRes.Error)
	}
}
