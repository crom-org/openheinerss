package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/process"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	harness.Register("opencode", protocol.HarnessCatalogItem{
		ID:                 "opencode",
		DisplayName:        "OpenCode Interpreter & Server",
		SupportedModes:     []string{"cli", "api"},
		SupportedProtocols: []string{"openai", "deepseek", "ollama", "groq"},
		MCP:                "por execução: OPENCODE_CONFIG_CONTENT",
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "deepseek",
				Name:        "DeepSeek Oficial",
				Endpoint:    "https://api.deepseek.com/v1",
				RequiresKey: true,
			},
			{
				ID:          "ollama-local",
				Name:        "Ollama Local",
				Endpoint:    "http://127.0.0.1:11434/v1",
				RequiresKey: false,
			},
			{
				ID:          "groq",
				Name:        "Groq Cloud (Fast Inference)",
				Endpoint:    "https://api.groq.com/openai/v1",
				RequiresKey: true,
			},
			{
				ID:          "openai",
				Name:        "OpenAI Platform",
				Endpoint:    "https://api.openai.com/v1",
				RequiresKey: true,
			},
		},
	}, func(mode harness.Mode) (harness.Harness, error) {
		if mode == "" || mode == harness.ModeMock {
			mode = harness.ModeCLI
		}
		return NewOpenCodeHarness(mode), nil
	})
}

// OpenCodeHarness implementa o conector para o OpenCode Interpreter
type OpenCodeHarness struct {
	mu       sync.Mutex
	mode     harness.Mode
	cfg      harness.SessionConfig
	env      []string
	events   chan harness.Event
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	ctx      context.Context
	cancel   context.CancelFunc
	resumeID string
	stopped  bool

	// Ajustes que valem para as próximas chamadas (começam nas options; /model, /effort, /agent, /new mexem neles).
	model    string
	variant  string
	agent    string
	cont     bool
	fork     bool
	share    bool
	thinking bool
}

// NewOpenCodeHarness instancia o adaptador OpenCode
func NewOpenCodeHarness(mode harness.Mode) *OpenCodeHarness {
	return &OpenCodeHarness{
		mode:   mode,
		events: make(chan harness.Event, 200),
	}
}

func (o *OpenCodeHarness) Name() string {
	return "opencode"
}

func (o *OpenCodeHarness) Mode() harness.Mode {
	return o.mode
}

func (o *OpenCodeHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if o.mode == harness.ModeCLI {
		if _, err := exec.LookPath("opencode"); err != nil {
			return harness.PrerequisiteResult{
				Satisfied:    false,
				MissingItems: []string{"opencode"},
				SuggestedFix: "Instale o OpenCode CLI ou adicione ao PATH do sistema.",
			}
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (o *OpenCodeHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.cfg = cfg
	o.ctx, o.cancel = context.WithCancel(ctx)
	o.resumeID = optionString(cfg.Options, "opencode_session_id", "session_id")
	o.stopped = false
	o.model = cfg.Model
	o.variant = harness.OpcaoTexto(cfg.Options, "variant", "effort")
	o.agent = harness.OpcaoTexto(cfg.Options, "agent")
	o.cont = harness.OpcaoBool(cfg.Options, "continue")
	o.fork = harness.OpcaoBool(cfg.Options, "fork")
	o.share = harness.OpcaoBool(cfg.Options, "share")
	o.thinking = harness.OpcaoBool(cfg.Options, "thinking")

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	if cfg.CWD != "" {
		env = harness.SetEnv(env, "PWD", cfg.CWD)
	}
	// Servidores MCP do openheinerss: OPENCODE_CONFIG_CONTENT, que o opencode mescla por cima
	// da config do usuário só neste processo. Se quem chama já definiu a variável, ela é respeitada.
	servs, err := harness.ServidoresMCP(cfg)
	if err != nil {
		return fmt.Errorf("MCP: %w", err)
	}
	_, definido := cfg.Env["OPENCODE_CONFIG_CONTENT"]
	if _, noAmbiente := os.LookupEnv("OPENCODE_CONFIG_CONTENT"); noAmbiente {
		definido = true
	}
	if len(servs) > 0 && !definido {
		data, err := mcp.ParaOpenCode(servs)
		if err != nil {
			return fmt.Errorf("MCP: %w", err)
		}
		env = harness.SetEnv(env, "OPENCODE_CONFIG_CONTENT", string(data))
	}
	// Leitura de pastas do sistema liberada por padrão; escrita fora da worktree só nas permitidas.
	leitura, err := config.PastasLeituraEfetivas(harness.OpcaoLista(cfg.Options, "pastas_leitura"))
	if err != nil {
		return fmt.Errorf("permissões de pasta: %w", err)
	}
	data, err := opencodeExternalDirectoryConfig(env, cfg.CWD, harness.OpcaoLista(cfg.Options, "pastas_permitidas"), leitura)
	if err != nil {
		return fmt.Errorf("permissões de pasta: %w", err)
	}
	env = harness.SetEnv(env, "OPENCODE_CONFIG_CONTENT", string(data))
	o.env = env

	return nil
}

func opencodeExternalDirectoryConfig(env []string, cwd string, escrita, leitura []string) ([]byte, error) {
	var root map[string]interface{}
	for _, item := range env {
		if strings.HasPrefix(item, "OPENCODE_CONFIG_CONTENT=") {
			v := strings.TrimPrefix(item, "OPENCODE_CONFIG_CONTENT=")
			if v != "" {
				var candidate map[string]interface{}
				if json.Unmarshal([]byte(v), &candidate) == nil {
					root = candidate
				}
			}
		}
	}
	if root == nil {
		root = map[string]interface{}{}
	}
	harness.AplicarPermissoesPastasOpenCode(root, cwd, escrita, leitura)
	return json.Marshal(root)
}

func (o *OpenCodeHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.stopped {
		return fmt.Errorf("processo do OpenCode não está ativo")
	}

	sessID := o.cfg.SessionID
	command, text, handled, err := o.slash(text)
	if err != nil {
		return err
	}
	if handled {
		o.emitLocal(sessID, text)
		return nil
	}
	files := harness.OpcaoLista(o.cfg.Options, "files", "file")
	extraFiles, cleanup, err := harness.AnexosEmArquivos(attachments)
	if err != nil {
		return err
	}
	call := callArgs{
		resumeID: o.resumeID, model: o.model, variant: o.variant, agent: o.agent,
		cont: o.cont, fork: o.fork, share: o.share, thinking: o.thinking,
		files: append(files, extraFiles...), command: command,
		extra: harness.HarnessArgs(o.cfg.Options), text: text,
	}
	go func() {
		defer cleanup()
		o.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Consultando OpenCode Interpreter...",
			},
		})

		args := buildArgs(call)

		cmd := exec.CommandContext(o.ctx, "opencode", args...)
		process.Configure(cmd)
		cmd.Dir = o.cfg.CWD
		cmd.Env = o.env

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			o.emitProcessError(sessID, err)
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			o.emitProcessError(sessID, err)
			return
		}

		if err := cmd.Start(); err != nil {
			o.emitProcessError(sessID, err)
			return
		}
		o.mu.Lock()
		o.cmd = cmd
		o.mu.Unlock()
		var stderrBuf strings.Builder
		stderrRaw := harness.NewLineWriter(func(line string) {
			o.emit(harness.RawEvent(sessID, "opencode", "stderr", line))
		})
		stderrDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.MultiWriter(&stderrBuf, stderrRaw), stderr)
			stderrRaw.Flush()
			close(stderrDone)
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			o.handleLine(scanner.Text(), sessID)
		}

		// Ler o stderr até o fim antes do Wait: o Wait fecha o pipe e o resto se perderia.
		<-stderrDone
		err = cmd.Wait()
		o.mu.Lock()
		if o.cmd == cmd {
			o.cmd = nil
		}
		stopped := o.stopped
		o.mu.Unlock()
		if stopped {
			return
		}
		if err != nil {
			message := strings.TrimSpace(stderrBuf.String())
			if message == "" {
				message = err.Error()
			}
			o.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: message}})
			o.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "process_error"}})
			return
		}
		if info := strings.TrimSpace(stderrBuf.String()); info != "" {
			o.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: "[stderr] " + info + "\n"}})
		}
		o.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    "completed",
			},
		})
	}()

	return nil
}

// callArgs reúne tudo que muda a linha de comando de uma chamada do `opencode run`.
type callArgs struct {
	resumeID, model, variant, agent, command, text string
	cont, fork, share, thinking                    bool
	files, extra                                   []string
}

// buildArgs monta `opencode run`: opções tipadas, depois harness_args (intactos, na ordem) e,
// por último, a mensagem. Com --command, a mensagem são os argumentos do comando.
func buildArgs(c callArgs) []string {
	args := []string{"run", "--format", "json"}
	if c.resumeID != "" {
		args = append(args, "--session", c.resumeID)
	} else if c.cont {
		args = append(args, "--continue")
	}
	if c.fork && (c.resumeID != "" || c.cont) {
		args = append(args, "--fork")
	}
	if c.model != "" {
		args = append(args, "-m", c.model)
	}
	if c.variant != "" {
		args = append(args, "--variant", c.variant)
	}
	if c.agent != "" {
		args = append(args, "--agent", c.agent)
	}
	if c.share {
		args = append(args, "--share")
	}
	if c.thinking {
		args = append(args, "--thinking")
	}
	for _, f := range c.files {
		args = append(args, "--file", f)
	}
	if c.command != "" {
		args = append(args, "--command", c.command)
	}
	args = append(args, c.extra...)
	if c.command != "" && c.text == "" {
		return args
	}
	return append(args, "--", c.text)
}

// slash trata um prompt que começa com "/". Comandos do opencode viram `--command nome` (a mensagem
// passa a ser o argumento); comandos que só existem na tela são traduzidos para ajustes das próximas
// chamadas (handled=true, a mensagem de aviso volta em text) ou recusados com NoEquivalent.
// Chamar com o.mu preso.
func (o *OpenCodeHarness) slash(text string) (command, message string, handled bool, err error) {
	name, rest, ok := harness.SlashCommand(text)
	if !ok {
		return "", text, false, nil
	}
	switch strings.ToLower(name) {
	case "model", "models":
		if rest == "" {
			return "", "", false, harness.NoEquivalent("opencode", name, "informe o modelo no formato provedor/modelo, ex.: /model openai/gpt-4o")
		}
		o.model = rest
		return "", "modelo das próximas chamadas: " + rest, true, nil
	case "effort", "variant":
		o.variant = rest
		return "", "esforço (--variant) das próximas chamadas: " + rest, true, nil
	case "agent":
		if rest == "" {
			return "", "", false, harness.NoEquivalent("opencode", name, "informe o agente, ex.: /agent build")
		}
		o.agent = rest
		return "", "agente das próximas chamadas: " + rest, true, nil
	case "new", "clear":
		o.resumeID, o.cont, o.fork = "", false, false
		return "", "sessão esquecida: a próxima chamada começa uma conversa nova", true, nil
	case "share":
		o.share = true
		return "", "as próximas chamadas serão compartilhadas (--share)", true, nil
	case "thinking":
		o.thinking = true
		return "", "as próximas chamadas mostram blocos de raciocínio (--thinking)", true, nil
	case "exit", "quit", "q", "themes", "editor", "details", "help", "sessions", "agents":
		return "", "", false, harness.NoEquivalent("opencode", name, "é um comando da tela (TUI); no `opencode run` só valem comandos definidos via --command e as opções da linha de comando")
	}
	return name, rest, false, nil
}

// emitLocal devolve o aviso de um comando traduzido pela ponte e encerra o turno.
func (o *OpenCodeHarness) emitLocal(sessionID, message string) {
	o.emitLocked(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: message + "\n"}})
	o.emitLocked(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: "completed"}})
}

func (o *OpenCodeHarness) emitProcessError(sessionID string, err error) {
	o.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: err.Error()}})
	o.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: "process_error"}})
}

func (o *OpenCodeHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.stopped || o.stdin == nil {
		return fmt.Errorf("processo do OpenCode não está ativo")
	}

	char := "n\n"
	if allow {
		char = "y\n"
	}
	_, err := o.stdin.Write([]byte(char))
	return err
}

func (o *OpenCodeHarness) Events() <-chan harness.Event {
	return o.events
}

// ResumeID retorna o sessionID atribuído pelo OpenCode para retomadas nativas.
func (o *OpenCodeHarness) ResumeID() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.resumeID
}

func (o *OpenCodeHarness) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.stopped {
		return nil
	}
	o.stopped = true

	if o.cancel != nil {
		o.cancel()
	}
	if o.stdin != nil {
		_ = o.stdin.Close()
	}
	if o.cmd != nil && o.cmd.Process != nil {
		if process.Interrupt(o.cmd) != nil {
			_ = o.cmd.Process.Signal(os.Interrupt)
		}
	}

	return nil
}

func (o *OpenCodeHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sessID := o.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		o.handleLine(line, sessID)
	}

	o.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
}

func (o *OpenCodeHarness) handleLine(line, sessionID string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var raw map[string]interface{}
	if json.Unmarshal([]byte(line), &raw) != nil {
		o.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: line + "\n"}})
		return
	}
	if sid, ok := raw["sessionID"].(string); ok && sid != "" {
		o.mu.Lock()
		o.resumeID = sid
		o.mu.Unlock()
	}
	events := parseOpenCodeEvent(raw, sessionID)
	if len(events) == 0 {
		o.emit(harness.RawEvent(sessionID, "opencode", "stdout", line))
		return
	}
	for _, event := range events {
		o.emit(event)
	}
}

func parseOpenCodeEvent(raw map[string]interface{}, sessionID string) []harness.Event {
	part, _ := raw["part"].(map[string]interface{})
	str := func(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }
	switch raw["type"] {
	case "text":
		if text := str(part, "text"); text != "" {
			return []harness.Event{{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: text}}}
		}
	case "tool_use":
		tool := str(part, "tool")
		call := str(part, "callID")
		state, _ := part["state"].(map[string]interface{})
		result := []harness.Event{{Type: harness.EventToolCall, Payload: protocol.ToolCallParams{SessionID: sessionID, CallID: call, Tool: tool, Input: state["input"]}}}
		if status := str(state, "status"); status == "completed" || status == "error" {
			result = append(result, harness.Event{Type: harness.EventToolResult, Payload: protocol.ToolResultParams{SessionID: sessionID, CallID: call, Status: status, Output: str(state, "output")}})
		}
		return result
	case "step_finish":
		tokens, _ := part["tokens"].(map[string]interface{})
		return []harness.Event{{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: sessionID, InputTokens: number(tokens, "input"), OutputTokens: number(tokens, "output"), TotalTokens: number(tokens, "total")}}}
	case "error":
		message := str(raw, "message")
		if message == "" {
			message = str(part, "message")
		}
		if message == "" {
			if nested, ok := raw["error"].(map[string]interface{}); ok {
				message = str(nested, "message")
				if data, ok := nested["data"].(map[string]interface{}); ok && str(data, "message") != "" {
					message = str(data, "message")
				}
			}
		}
		if message == "" {
			message = "erro do OpenCode sem mensagem"
		}
		return []harness.Event{{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: message}}}
	}
	return nil
}

func number(m map[string]interface{}, key string) int64 { v, _ := m[key].(float64); return int64(v) }

func optionString(options map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := options[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (o *OpenCodeHarness) emit(evt harness.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped {
		return
	}
	select {
	case o.events <- evt:
	default:
	}
}

// emitLocked é o emit para quem já está com o.mu preso (SendPrompt).
func (o *OpenCodeHarness) emitLocked(evt harness.Event) {
	select {
	case o.events <- evt:
	default:
	}
}
