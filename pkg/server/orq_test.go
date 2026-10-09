package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crom-org/openheinerss/pkg/orchestrator"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

// repoOrq cria um repositório git com a pasta de agentes e um prompt para cada nome.
func repoOrq(t *testing.T, nomes ...string) string {
	t.Helper()
	root := t.TempDir()
	gitT(t, root, "init", "-b", "main")
	gitT(t, root, "config", "user.email", "teste@example.invalid")
	gitT(t, root, "config", "user.name", "Teste")
	_ = os.WriteFile(filepath.Join(root, "README"), []byte("base\n"), 0644)
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-m", "base")
	prompts := filepath.Join(root, ".claude", "agentes", "prompts")
	_ = os.MkdirAll(prompts, 0755)
	for _, n := range nomes {
		_ = os.WriteFile(filepath.Join(prompts, n+".md"), []byte("faça a tarefa"), 0644)
	}
	return root
}

type clienteWS struct {
	t       *testing.T
	conn    *websocket.Conn
	resp    chan protocol.Response
	eventos chan protocol.Notification
	nextID  int
}

func novoServidorWS(t *testing.T) string {
	return novoServidorWSComLimite(t, 0)
}

func novoServidorWSComLimite(t *testing.T, limite int) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := server.NewWSServerWithMaxAgents(session.NewManager(), limite)
	go func() { _ = s.ListenAndServe(addr) }()
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	return addr
}

func conectar(t *testing.T, addr string) *clienteWS {
	t.Helper()
	var conn *websocket.Conn
	var err error
	for i := 0; i < 50; i++ {
		conn, _, err = websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	c := &clienteWS{t: t, conn: conn, resp: make(chan protocol.Response, 16), eventos: make(chan protocol.Notification, 256)}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var probe struct {
				ID     interface{}     `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(msg, &probe) != nil {
				continue
			}
			if probe.Method != "" && probe.ID == nil {
				var n protocol.Notification
				_ = json.Unmarshal(msg, &n)
				n.Params = json.RawMessage(probe.Params)
				c.eventos <- n
				continue
			}
			var r protocol.Response
			_ = json.Unmarshal(msg, &r)
			c.resp <- r
		}
	}()
	return c
}

func (c *clienteWS) chamar(method string, params interface{}) protocol.Response {
	c.t.Helper()
	c.nextID++
	b, _ := json.Marshal(protocol.Request{JSONRPC: "2.0", ID: c.nextID, Method: method, Params: mustJSON(params)})
	if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		c.t.Fatal(err)
	}
	select {
	case r := <-c.resp:
		return r
	case <-time.After(60 * time.Second):
		c.t.Fatalf("sem resposta para %s", method)
	}
	return protocol.Response{}
}

func (c *clienteWS) resultado(method string, params interface{}, out interface{}) {
	c.t.Helper()
	r := c.chamar(method, params)
	if r.Error != nil {
		c.t.Fatalf("%s: erro %+v", method, r.Error)
	}
	b, _ := json.Marshal(r.Result)
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
}

// proximo espera o próximo evento do método dado, ignorando os demais, e devolve o JSON dos params.
func (c *clienteWS) proximo(metodo string, ignorar ...string) json.RawMessage {
	c.t.Helper()
	timeout := time.After(60 * time.Second)
	for {
		select {
		case n := <-c.eventos:
			if n.Method == metodo {
				return n.Params.(json.RawMessage)
			}
			ok := false
			for _, i := range ignorar {
				ok = ok || n.Method == i
			}
			if !ok {
				c.t.Fatalf("esperava %s, chegou %s (%s)", metodo, n.Method, n.Params)
			}
		case <-timeout:
			c.t.Fatalf("timeout esperando %s", metodo)
		}
	}
}

func (c *clienteWS) semEvento(d time.Duration) {
	c.t.Helper()
	select {
	case n := <-c.eventos:
		c.t.Fatalf("evento inesperado %s %s", n.Method, n.Params.(json.RawMessage))
	case <-time.After(d):
	}
}

func TestOrqRodarIniciarInicioProgressoDecisaoFim(t *testing.T) {
	root := repoOrq(t, "mock-a")
	c := conectar(t, novoServidorWS(t))
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)

	var ini protocol.RodarIniciarResult
	c.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-a", Motor: "mock", CWD: root, MaxAgentes: 99}, Projeto: "demo"}, &ini)
	if ini.Geracao == "" || ini.ID == "" {
		t.Fatalf("resposta sem geração/id: %+v", ini)
	}
	if ini.ID == "" || ini.Projeto != "demo" {
		t.Fatalf("resultado inesperado: %+v", ini)
	}

	var inicio protocol.OrqInicioParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqInicio), &inicio)
	if inicio.Geracao != ini.Geracao || inicio.Agente != "mock-a" || inicio.Motor != "mock" || inicio.Tentativa != 1 || inicio.ID != ini.ID || inicio.Projeto != "demo" || inicio.Worktree == "" {
		t.Fatalf("inicio: %+v", inicio)
	}
	var prog protocol.OrqProgressoParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqProgresso), &prog)
	if prog.Agente != "mock-a" || prog.Resumo == "" {
		t.Fatalf("progresso: %+v", prog)
	}
	var dec protocol.OrqDecisaoParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqProgresso), &dec)
	if dec.ID == "" || dec.Agente != "mock-a" || len(dec.Opcoes) != 2 || dec.Pergunta == "" {
		t.Fatalf("decisão: %+v", dec)
	}
	if dec.Geracao != ini.Geracao || dec.Run != ini.ID {
		t.Fatalf("decisão sem geração/run: %+v", dec)
	}

	// rodar.listar mostra o agente rodando e a decisão pendente.
	var lista protocol.RodarListarResult
	c.resultado(protocol.MethodRodarListar, protocol.RodarListarParams{CWD: root}, &lista)
	if len(lista.Agentes) != 1 || lista.Agentes[0].Estado != "rodando" || len(lista.Decisoes) != 1 {
		t.Fatalf("listar durante: %+v", lista)
	}

	if r := c.chamar(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Run: dec.Run, ID: dec.ID, Resposta: "talvez"}); r.Error == nil {
		t.Fatal("resposta inválida deveria falhar")
	}
	if r := c.chamar(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Geracao: "geracao-inexistente", Run: dec.Run, ID: dec.ID, Resposta: "permitir"}); r.Error == nil || !strings.Contains(r.Error.Message, "geração") {
		t.Fatalf("geração errada deveria ser recusada: %+v", r.Error)
	}
	if r := c.chamar(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Geracao: dec.Geracao, Run: "rodar-errado", ID: dec.ID, Resposta: "permitir"}); r.Error == nil || !strings.Contains(r.Error.Message, "não pertence") {
		t.Fatalf("run errado deveria ser recusado: %+v", r.Error)
	}
	c.resultado(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Run: dec.Run, ID: dec.ID, Resposta: "permitir"}, nil)

	var fim protocol.OrqFimParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso), &fim)
	if fim.Codigo != 0 || fim.Agente != "mock-a" || fim.Tentativas != 1 || fim.ID != ini.ID {
		t.Fatalf("fim: %+v", fim)
	}
	if r := c.chamar(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Run: dec.Run, ID: dec.ID, Resposta: "permitir"}); r.Error == nil {
		t.Fatal("decisão repetida deveria falhar")
	}

	// Depois do fim, nada de progresso atrasado.
	c.semEvento(300 * time.Millisecond)
	c.resultado(protocol.MethodRodarListar, protocol.RodarListarParams{CWD: root}, &lista)
	if len(lista.Agentes) != 1 || lista.Agentes[0].Estado != "concluido" || len(lista.Decisoes) != 0 {
		t.Fatalf("listar depois: %+v", lista)
	}
}

func TestGeracaoDiferenteEntreServes(t *testing.T) {
	a := conectar(t, novoServidorWS(t))
	b := conectar(t, novoServidorWS(t))
	var ra, rb protocol.Response
	ra = a.chamar(protocol.MethodHarnessListar, nil)
	rb = b.chamar(protocol.MethodHarnessListar, nil)
	if ra.Geracao == "" || rb.Geracao == "" || ra.Geracao == rb.Geracao {
		t.Fatalf("servidores deveriam ter gerações distintas: %q %q", ra.Geracao, rb.Geracao)
	}
}

func TestOrqDecisaoNegadaGeraErroEFimComFalha(t *testing.T) {
	root := repoOrq(t, "mock-n")
	c := conectar(t, novoServidorWS(t))
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)
	c.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-n", Motor: "mock", CWD: root, MaxAgentes: 99, Tentativas: 1}}, nil)
	var dec protocol.OrqDecisaoParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqInicio, protocol.EventOrqProgresso), &dec)
	c.resultado(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{Run: dec.Run, ID: dec.ID, Resposta: "negar"}, nil)
	var erro protocol.OrqErroParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqErro, protocol.EventOrqProgresso), &erro)
	if erro.Agente != "mock-n" || erro.Mensagem == "" || erro.Cota {
		t.Fatalf("erro: %+v", erro)
	}
	var fim protocol.OrqFimParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso), &fim)
	if fim.Codigo != 1 {
		t.Fatalf("fim: %+v", fim)
	}
}

func TestOrqFiltrosDeAssinatura(t *testing.T) {
	root := repoOrq(t, "mock-f")
	addr := novoServidorWS(t)
	todos, outro, certo := conectar(t, addr), conectar(t, addr), conectar(t, addr)
	todos.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)
	outro.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root, Agente: "outro"}, nil)
	certo.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root, Projeto: "p1", Agente: "mock-f"}, nil)
	semAssinar := conectar(t, addr)

	var ini protocol.RodarIniciarResult
	todos.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-f", Motor: "mock", CWD: root, MaxAgentes: 99}, Projeto: "p1"}, &ini)
	var dec protocol.OrqDecisaoParams
	_ = json.Unmarshal(todos.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqInicio, protocol.EventOrqProgresso), &dec)
	_ = certo.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqInicio, protocol.EventOrqProgresso)
	// Sem run nem geração (cliente antigo): continua aceito.
	todos.resultado(protocol.MethodRodarDecidir, protocol.RodarDecidirParams{ID: dec.ID, Resposta: "permitir"}, nil)
	_ = todos.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso)
	_ = certo.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso)
	outro.semEvento(200 * time.Millisecond)
	semAssinar.semEvento(50 * time.Millisecond)
}

func TestOrqPararExecucao(t *testing.T) {
	root := repoOrq(t, "mock-p")
	c := conectar(t, novoServidorWS(t))
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)
	var ini protocol.RodarIniciarResult
	c.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-p", Motor: "mock", CWD: root, MaxAgentes: 99}}, &ini)
	if r := c.chamar(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-p", Motor: "mock", CWD: root}}); r.Error == nil {
		t.Fatal("mesmo agente duas vezes deveria falhar")
	}
	_ = c.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqInicio, protocol.EventOrqProgresso)
	c.resultado(protocol.MethodRodarParar, protocol.RodarPararParams{ID: ini.ID}, nil)
	var fim protocol.OrqFimParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso, protocol.EventOrqErro), &fim)
	if fim.Codigo != 130 {
		t.Fatalf("fim: %+v", fim)
	}
	if r := c.chamar(protocol.MethodRodarParar, protocol.RodarPararParams{ID: "nao-existe"}); r.Error == nil {
		t.Fatal("parar id inexistente deveria falhar")
	}
}

func TestOrqObservaAgenteLancadoForaDoServidor(t *testing.T) {
	old := server.IntervaloObservador
	server.IntervaloObservador = 40 * time.Millisecond
	defer func() { server.IntervaloObservador = old }()
	root := repoOrq(t, "externo")
	c := conectar(t, novoServidorWS(t))
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)

	// Como o CLI: orchestrator.Run direto, sem passar pelo servidor.
	done := make(chan error, 1)
	go func() {
		_, err := orchestrator.Run(context.Background(), root, orchestrator.Options{Name: "externo", Motor: "mock", MaxAgents: 99})
		done <- err
	}()
	var inicio protocol.OrqInicioParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqInicio), &inicio)
	if inicio.Agente != "externo" || inicio.Motor != "mock" || inicio.ID != "" || inicio.Projeto != filepath.Base(root) {
		t.Fatalf("inicio: %+v", inicio)
	}
	var fim protocol.OrqFimParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso), &fim)
	if fim.Codigo != 0 || fim.Agente != "externo" {
		t.Fatalf("fim: %+v", fim)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.semEvento(300 * time.Millisecond) // sem duplicar
}

func TestOrqMetodosSimples(t *testing.T) {
	c := conectar(t, novoServidorWS(t))
	var cat protocol.CatalogListResult
	c.resultado(protocol.MethodHarnessListar, nil, &cat)
	achou := false
	for _, h := range cat.Harnesses {
		achou = achou || h.ID == "mock"
	}
	if !achou {
		t.Fatalf("harness.listar sem mock: %+v", cat)
	}
	var lim map[string]interface{}
	c.resultado(protocol.MethodLimitesObter, nil, &lim)
	if _, ok := lim["instancias"]; !ok {
		t.Fatalf("limites.obter: %+v", lim)
	}
	if r := c.chamar(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{}); r.Error == nil {
		t.Fatal("rodar.iniciar sem nome deveria falhar")
	}
}

func TestOrqViaStdio(t *testing.T) {
	root := repoOrq(t, "mock-s")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := server.NewStdioServer(session.NewManager(), inR, outW)
	go func() { _ = s.Run(ctx); _ = outW.Close() }()
	linhas := make(chan map[string]interface{}, 64)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			var m map[string]interface{}
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				linhas <- m
			}
		}
	}()
	enviar := func(s string) { _, _ = inW.Write([]byte(s + "\n")) }
	esperar := func(ok func(map[string]interface{}) bool) map[string]interface{} {
		for {
			select {
			case m := <-linhas:
				if ok(m) {
					return m
				}
			case <-time.After(60 * time.Second):
				t.Fatal("timeout no stdio")
			}
		}
	}
	enviar(`{"jsonrpc":"2.0","id":1,"method":"eventos.assinar","params":{"cwd":` + string(mustJSON(root)) + `}}`)
	esperar(func(m map[string]interface{}) bool { return m["id"] == float64(1) })
	enviar(`{"jsonrpc":"2.0","id":2,"method":"rodar.iniciar","params":{"nome":"mock-s","motor":"mock","cwd":` + string(mustJSON(root)) + `,"maxAgentes":99}}`)
	d := esperar(func(m map[string]interface{}) bool { return m["method"] == protocol.EventOrqPrecisaDecisao })
	id := d["params"].(map[string]interface{})["id"].(string)
	run := d["params"].(map[string]interface{})["run"].(string)
	enviar(`{"jsonrpc":"2.0","id":3,"method":"rodar.decidir","params":{"run":"` + run + `","id":"` + id + `","resposta":"permitir"}}`)
	f := esperar(func(m map[string]interface{}) bool { return m["method"] == protocol.EventOrqFim })
	if f["params"].(map[string]interface{})["codigo"] != float64(0) {
		t.Fatalf("fim: %v", f)
	}
}

func TestOrqMensagemParaAgenteVivo(t *testing.T) {
	root := repoOrq(t, "mock-p")
	c := conectar(t, novoServidorWS(t))
	c.resultado(protocol.MethodEventosAssinar, protocol.EventosAssinarParams{CWD: root}, nil)
	var ini protocol.RodarIniciarResult
	c.resultado(protocol.MethodRodarIniciar, protocol.RodarIniciarParams{RunParams: protocol.RunParams{Nome: "mock-p", Motor: "mock", CWD: root, MaxAgentes: 99}}, &ini)
	_ = c.proximo(protocol.EventOrqPrecisaDecisao, protocol.EventOrqInicio, protocol.EventOrqProgresso)

	var rec protocol.RodarMensagemResult
	c.resultado(protocol.MethodRodarMensagem, protocol.RodarMensagemParams{CWD: root, Agente: "mock-p", Texto: "faça X"}, &rec)
	// O harness mock não aceita texto no meio do turno: o recibo fica pendente até a retomada.
	if rec.ID == "" || rec.Agente != "mock-p" || rec.Status != "pendente" || rec.Recibo != "pendente" || rec.Em == "" {
		t.Fatalf("recibo: %+v", rec)
	}
	var ev protocol.OrqMensagemParams
	_ = json.Unmarshal(c.proximo(protocol.EventOrqMensagem, protocol.EventOrqProgresso, protocol.EventOrqPrecisaDecisao), &ev)
	if ev.Mensagem != rec.ID || ev.Estado != "recebida" || ev.Agente != "mock-p" || ev.ID != ini.ID {
		t.Fatalf("orq.mensagem: %+v", ev)
	}
	if r := c.chamar(protocol.MethodRodarMensagem, protocol.RodarMensagemParams{CWD: root, Agente: "nao-existe", Texto: "x"}); r.Error == nil {
		t.Fatal("agente inexistente deveria falhar")
	}
	if r := c.chamar(protocol.MethodRodarMensagem, protocol.RodarMensagemParams{CWD: root, Agente: "mock-p"}); r.Error == nil {
		t.Fatal("texto vazio deveria falhar")
	}

	c.resultado(protocol.MethodRodarParar, protocol.RodarPararParams{ID: ini.ID}, nil)
	_ = c.proximo(protocol.EventOrqFim, protocol.EventOrqProgresso, protocol.EventOrqErro, protocol.EventOrqMensagem)
	if r := c.chamar(protocol.MethodRodarMensagem, protocol.RodarMensagemParams{CWD: root, Agente: "mock-p", Texto: "tarde"}); r.Error == nil || !strings.Contains(r.Error.Message, "já terminou") {
		t.Fatalf("agente terminado deveria dar erro claro: %+v", r.Error)
	}
}
