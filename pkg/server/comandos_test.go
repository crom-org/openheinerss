package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/config"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

func TestHarnessComandosPeloServidor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv(config.EnvConfigDir, tmp)
	r := server.NewRouter(session.NewManager())
	chamar := func(method string, p protocol.HarnessComandosParams) protocol.Response {
		b, _ := json.Marshal(p)
		return r.HandleRequest(context.Background(), protocol.Request{JSONRPC: "2.0", ID: 1, Method: method, Params: b})
	}
	if res := chamar(protocol.MethodHarnessComandosAnotar, protocol.HarnessComandosParams{Harness: "claude-code", Comando: "/compact", Anotacao: "compacta o claude code"}); res.Error != nil {
		t.Fatal(res.Error.Message)
	}
	if res := chamar(protocol.MethodHarnessComandosConfirmar, protocol.HarnessComandosParams{Harness: "claude-code", Comando: "/compact"}); res.Error != nil {
		t.Fatal(res.Error.Message)
	}
	res := chamar(protocol.MethodHarnessComandos, protocol.HarnessComandosParams{Harness: "claude-code"})
	if res.Error != nil {
		t.Fatal(res.Error.Message)
	}
	b, _ := json.Marshal(res.Result)
	if !strings.Contains(string(b), `"nome":"/compact"`) || !strings.Contains(string(b), `"anotacao":"compacta o claude code","confirmado":true`) {
		t.Fatalf("resultado: %s", b)
	}
	if _, err := os.Stat(filepath.Join(tmp, "comandos.yaml")); err != nil {
		t.Fatalf("comandos.yaml não gravado na pasta de config: %v", err)
	}
	if res := chamar(protocol.MethodHarnessComandos, protocol.HarnessComandosParams{}); res.Error == nil || res.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("sem harness deveria falhar: %+v", res.Error)
	}
	if res := chamar(protocol.MethodHarnessComandos, protocol.HarnessComandosParams{Harness: "nao-existe"}); res.Error == nil {
		t.Fatal("harness inexistente deveria falhar")
	}
}
