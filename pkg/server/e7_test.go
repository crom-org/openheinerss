package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
	"github.com/crom-org/openheinerss/pkg/storage"
)

func chamarRPC(r *server.Router, method string, p interface{}) protocol.Response {
	b, _ := json.Marshal(p)
	return r.HandleRequest(context.Background(), protocol.Request{JSONRPC: "2.0", ID: 1, Method: method, Params: b})
}

func TestHarnessCapacidadesPeloServidor(t *testing.T) {
	r := server.NewRouter(session.NewManager())
	res := chamarRPC(r, protocol.MethodHarnessCapacidades, protocol.HarnessCapacidadesParams{Harness: "codex"})
	if res.Error != nil {
		t.Fatal(res.Error.Message)
	}
	b, _ := json.Marshal(res.Result)
	if !strings.Contains(string(b), `"base":"codex"`) || !strings.Contains(string(b), "AGENTS.md") || !strings.Contains(string(b), `"retomar"`) {
		t.Fatalf("resultado: %s", b)
	}
	res = chamarRPC(r, protocol.MethodHarnessCapacidades, protocol.HarnessCapacidadesParams{})
	b, _ = json.Marshal(res.Result)
	if res.Error != nil || strings.Count(string(b), `"conferidoCom"`) != 5 {
		t.Fatalf("sem harness deveria listar as 5 bases: %v %s", res.Error, b)
	}
	if res := chamarRPC(r, protocol.MethodHarnessCapacidades, protocol.HarnessCapacidadesParams{Harness: "nao-existe"}); res.Error == nil {
		t.Fatal("harness inexistente deveria falhar")
	}
}

func TestSessionCreateRetomaPeloIdNativoESessaoPersistida(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	bin := t.TempDir()
	args := filepath.Join(t.TempDir(), "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + args + "\nprintf '%s\\n' '{\"type\":\"text\",\"sessionID\":\"ses_nova\",\"part\":{\"type\":\"text\",\"text\":\"ok\"}}'\nprintf '%s\\n' '{\"type\":\"step_finish\",\"sessionID\":\"ses_nova\",\"part\":{\"type\":\"step-finish\",\"tokens\":{\"total\":1}}}'\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cwd := t.TempDir()
	m := session.NewManager()
	defer m.Close()
	r := server.NewRouter(m)
	prompt := func(id string) {
		t.Helper()
		if res := chamarRPC(r, protocol.MethodSessionPrompt, protocol.SessionPromptParams{SessionID: id, Text: "oi"}); res.Error != nil {
			t.Fatal(res.Error.Message)
		}
	}
	esperarArgs := func(parte string) {
		t.Helper()
		for i := 0; i < 300; i++ {
			if b, _ := os.ReadFile(args); strings.Contains(string(b), parte) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		b, _ := os.ReadFile(args)
		t.Fatalf("argumentos sem %q: %q", parte, b)
	}
	criar := func(retomar string) string {
		t.Helper()
		res := chamarRPC(r, protocol.MethodSessionCreate, protocol.SessionCreateParams{Harness: "opencode", Mode: "cli", CWD: cwd, Retomar: retomar})
		if res.Error != nil {
			t.Fatal(res.Error.Message)
		}
		var criada protocol.SessionCreateResult
		b, _ := json.Marshal(res.Result)
		_ = json.Unmarshal(b, &criada)
		prompt(criada.SessionID)
		return criada.SessionID
	}
	// 1) id nativo direto
	primeira := criar("ses_nativa_9")
	esperarArgs("--session\nses_nativa_9\n")
	// 2) id de sessão do openheinerss: usa o id nativo que o opencode devolveu no fim do turno
	for i := 0; i < 300; i++ {
		if id, ok := storage.GetStorage().IDNativo(cwd, primeira); ok && id == "ses_nova" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	criar(primeira)
	esperarArgs("--session\nses_nova\n")
	// 3) base sem resume nativo recusa o pedido em vez de ignorá-lo
	if res := chamarRPC(r, protocol.MethodSessionCreate, protocol.SessionCreateParams{Harness: "aider", Mode: "cli", CWD: cwd, Retomar: "x"}); res.Error == nil || !strings.Contains(res.Error.Message, "não suportada") {
		t.Fatalf("aider deveria recusar: %+v", res.Error)
	}
}

func TestRunParamsRetomarAceitaBoolOuString(t *testing.T) {
	var p protocol.RodarIniciarParams
	if err := json.Unmarshal([]byte(`{"nome":"a","motor":"codex","retomar":"019abc","projeto":"x"}`), &p); err != nil || p.Retomar.ID != "019abc" || p.Retomar.Continuar || p.Projeto != "x" {
		t.Fatalf("%+v %v", p, err)
	}
	if err := json.Unmarshal([]byte(`{"nome":"a","motor":"codex","retomar":true}`), &p); err != nil || !p.Retomar.Continuar || p.Retomar.ID != "" {
		t.Fatalf("%+v %v", p, err)
	}
	if err := json.Unmarshal([]byte(`{"nome":"a","motor":"codex","retomar":3}`), &p); err == nil {
		t.Fatal("número deveria falhar")
	}
	b, _ := json.Marshal(protocol.RunParams{Retomar: protocol.Retomada{ID: "z"}})
	if !strings.Contains(string(b), `"retomar":"z"`) {
		t.Fatalf("%s", b)
	}
}

func TestHarnessVersoesEAtualizarSecoPeloServidor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // sem nenhum CLI: tudo "ausente"/"sem_cli", sem rede
	r := server.NewRouter(session.NewManager())
	res := chamarRPC(r, protocol.MethodHarnessVersoes, protocol.HarnessVersoesParams{Harness: "codex"})
	b, _ := json.Marshal(res.Result)
	if res.Error != nil || !strings.Contains(string(b), `"harness":"codex"`) || !strings.Contains(string(b), `"estado":"ausente"`) {
		t.Fatalf("versoes: %v %s", res.Error, b)
	}
	res = chamarRPC(r, protocol.MethodHarnessAtualizar, protocol.HarnessAtualizarParams{Harness: "codex", Seco: true})
	b, _ = json.Marshal(res.Result)
	if res.Error != nil || !strings.Contains(string(b), `"resultado":"sem_cli"`) {
		t.Fatalf("atualizar: %v %s", res.Error, b)
	}
	if res := chamarRPC(r, protocol.MethodHarnessAtualizar, protocol.HarnessAtualizarParams{Harness: "nao-existe"}); res.Error == nil {
		t.Fatal("harness desconhecido deveria falhar")
	}
	if res := chamarRPC(r, protocol.MethodHarnessVersoes, protocol.HarnessVersoesParams{Harness: "nao-existe"}); res.Error == nil {
		t.Fatal("harness desconhecido deveria falhar")
	}
}
