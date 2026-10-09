package codex

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/process"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	register := func(name string) {
		harness.Register(name, protocol.HarnessCatalogItem{ID: name, DisplayName: "OpenAI Codex / Assistant Engine", SupportedModes: []string{"cli"}, SupportedProtocols: []string{"openai"}, MCP: "por execução: -c mcp_servers.<nome>.* (env e headers pelo ambiente)"}, func(mode harness.Mode) (harness.Harness, error) {
			if mode == harness.ModeAPI {
				return nil, fmt.Errorf("modo Codex api removido: use cli com codex exec")
			}
			if mode == "" || mode == harness.ModeMock {
				mode = harness.ModeCLI
			}
			return NewCodexHarness(mode), nil
		})
	}
	register("codex")
}

// Modelo e esforço que o `codex exec` recebe quando a instância não define os seus.
const (
	ModeloPadrao  = "gpt-reserve"
	EsforcoPadrao = "medium"
)

// CodexHarness adapta o streaming JSONL do `codex exec` ao protocolo do projeto.
type CodexHarness struct {
	mu       sync.Mutex
	mode     harness.Mode
	cfg      harness.SessionConfig
	events   chan harness.Event
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	ctx      context.Context
	stopped  bool
	threadID string
}

func NewCodexHarness(mode harness.Mode) *CodexHarness {
	return &CodexHarness{mode: mode, events: make(chan harness.Event, 200)}
}
func (c *CodexHarness) Name() string       { return "codex" }
func (c *CodexHarness) Mode() harness.Mode { return c.mode }
func (c *CodexHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if c.mode == harness.ModeCLI {
		if _, err := exec.LookPath("codex"); err != nil {
			return harness.PrerequisiteResult{Satisfied: false, MissingItems: []string{"codex"}, SuggestedFix: "Instale o Codex CLI no PATH."}
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (c *CodexHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	if c.cfg.Model == "" {
		// Mantém o modelo efetivo explícito no estado do adaptador; o CLI usa o
		// mesmo valor em buildExecArgs quando a instância não o sobrescreve.
		c.cfg.Model = ModeloPadrao
	}
	c.stopped = false
	c.threadID = optionString(cfg.Options, "codex_session_id", "resume_session", "session_id")
	// Servidores MCP do openheinerss: -c mcp_servers.<nome>.* por execução; os valores secretos
	// (env/headers) vão no ambiente do processo, nunca na linha de comando.
	servs, err := harness.ServidoresMCP(cfg)
	if err != nil {
		return fmt.Errorf("MCP: %w", err)
	}
	if len(servs) > 0 {
		args, env, err := mcp.ParaCodex(servs)
		if err != nil {
			return fmt.Errorf("MCP: %w", err)
		}
		var kv []string
		for i := 1; i < len(args); i += 2 {
			kv = append(kv, args[i])
		}
		c.cfg.Options = harness.WithOption(c.cfg.Options, "config", append(kv, harness.OptionStrings(c.cfg.Options, "config")...))
		merged := make(map[string]string, len(env)+len(cfg.Env))
		for k, v := range env {
			merged[k] = v
		}
		for k, v := range cfg.Env {
			merged[k] = v
		}
		c.cfg.Env = merged
	}
	var cancel context.CancelFunc
	c.ctx, cancel = context.WithCancel(ctx)
	c.cancel = cancel
	return nil
}

func (c *CodexHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return fmt.Errorf("harness codex não está ativo")
	}
	if c.mode == harness.ModeAPI {
		return fmt.Errorf("modo Codex api removido: use cli com codex exec")
	}
	if c.mode != harness.ModeCLI {
		return fmt.Errorf("modo Codex não suportado: %s; use cli", c.mode)
	}
	// codex exec não interpreta "/": a ponte traduz o que tem equivalente e recusa o resto.
	if name, rest, ok := harness.SlashCommand(text); ok {
		sessID := c.cfg.SessionID
		switch name {
		case "model", "effort", "reasoning":
			if rest == "" {
				return harness.NoEquivalent("codex", name, "informe o valor, por exemplo /"+name+" <valor>")
			}
			if name == "model" {
				c.cfg.Model = rest
			} else {
				c.cfg.Options = harness.WithOption(c.cfg.Options, "effort", rest)
			}
			c.announceLocal(sessID, "Próximas chamadas com "+name+" "+rest)
			return nil
		case "new", "clear":
			c.threadID = ""
			c.announceLocal(sessID, "Conversa nova: a próxima chamada não retoma a thread anterior")
			return nil
		case "compact":
			return harness.NoEquivalent("codex", name, "codex exec não tem compactação; use /new para recomeçar ou harnessArgs com -c (ex.: model_auto_compact_token_limit=N)")
		default:
			return harness.NoEquivalent("codex", name, "use harnessArgs (flags do codex exec) ou o comando nativo no codex interativo")
		}
	}
	cmdCtx := c.ctx
	if ctx != nil {
		cmdCtx = ctx
	}
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	imageFiles, err := writeImages(attachments)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(cmdCtx, "codex", buildExecArgs(c.cfg, c.threadID, text, imageFiles...)...)
	// O CLI pode criar processos auxiliares. O grupo próprio garante que timeout
	// e Stop não deixem filhos segurando os pipes de streaming abertos.
	configureProcessGroup(cmd)
	cmd.Dir, cmd.Env = c.cfg.CWD, mergedEnv(c.cfg.Env, c.cfg.CWD)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		removeAll(imageFiles)
		return fmt.Errorf("stdout do codex: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		removeAll(imageFiles)
		return fmt.Errorf("stderr do codex: %w", err)
	}
	if err := cmd.Start(); err != nil {
		removeAll(imageFiles)
		return fmt.Errorf("falha ao iniciar codex exec: %w", err)
	}
	c.cmd = cmd
	sessionID := c.cfg.SessionID
	// Os leitores precisam terminar antes do Wait, senão o fim pode chegar antes do texto.
	var leitores sync.WaitGroup
	leitores.Add(2)
	go func() { defer leitores.Done(); c.readJSONL(stdout, sessionID) }()
	go func() { defer leitores.Done(); c.readStderr(stderr, sessionID) }()
	go func() {
		leitores.Wait()
		err := cmd.Wait()
		removeAll(imageFiles)
		c.mu.Lock()
		stopped := c.stopped
		if c.cmd == cmd {
			c.cmd = nil
		}
		c.mu.Unlock()
		if stopped {
			return
		}
		if err != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: err.Error()}})
			c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: "process_error"}})
			return
		}
		c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: "completed"}})
	}()
	return nil
}

// announceLocal avisa que a ponte tratou o comando sozinha. Roda em goroutine porque
// emit pega o mesmo mutex que SendPrompt segura.
func (c *CodexHarness) announceLocal(sessID, msg string) {
	go func() {
		c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: msg + "\n"}})
		c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "completed"}})
	}()
}

// writeImages grava os anexos (base64) em arquivos temporários para o -i do codex exec.
func writeImages(attachments []protocol.Attachment) ([]string, error) {
	var files []string
	for _, a := range attachments {
		data, err := base64.StdEncoding.DecodeString(a.Data)
		if err != nil {
			removeAll(files)
			return nil, fmt.Errorf("anexo inválido (base64): %w", err)
		}
		ext := ".png"
		switch a.MediaType {
		case "image/jpeg", "image/jpg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		}
		f, err := os.CreateTemp("", "openheinerss-img-*"+ext)
		if err != nil {
			removeAll(files)
			return nil, err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		files = append(files, f.Name())
		if werr != nil || cerr != nil {
			removeAll(files)
			return nil, fmt.Errorf("falha gravando anexo: %v %v", werr, cerr)
		}
	}
	return files, nil
}

func removeAll(files []string) {
	for _, f := range files {
		_ = os.Remove(f)
	}
}

func configureProcessGroup(cmd *exec.Cmd) {
	process.Configure(cmd)
}

func (c *CodexHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	return nil
}
func (c *CodexHarness) Events() <-chan harness.Event { return c.events }

// ResumeID expõe o thread_id descoberto pelo codex exec para retomadas nativas.
func (c *CodexHarness) ResumeID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.threadID
}

func (c *CodexHarness) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return nil
	}
	c.stopped = true
	if c.cancel != nil {
		c.cancel()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		if err := process.Interrupt(c.cmd); err == nil {
			return nil
		}
		return c.cmd.Process.Signal(os.Interrupt)
	}
	return nil
}

func (c *CodexHarness) readJSONL(r io.Reader, sessionID string) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if json.Unmarshal([]byte(line), &raw) == nil && raw["type"] == "thread.started" {
			if threadID, ok := raw["thread_id"].(string); ok && threadID != "" {
				c.mu.Lock()
				c.threadID = threadID
				c.mu.Unlock()
			}
		}
		events := parseJSONL(line, sessionID)
		for _, event := range events {
			c.emit(event)
		}
		if len(events) == 0 {
			c.emit(harness.RawEvent(sessionID, "codex", "stdout", line))
		}
	}
	if err := s.Err(); err != nil {
		c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: err.Error()}})
	}
}
func (c *CodexHarness) readStderr(r io.Reader, sessionID string) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for s.Scan() {
		c.emit(harness.RawEvent(sessionID, "codex", "stderr", s.Text()))
		// Aviso informativo do codex exec, não é erro.
		if text := strings.TrimSpace(s.Text()); text != "" && !strings.HasPrefix(text, "Reading additional input from stdin") {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: text}})
		}
	}
}
func (c *CodexHarness) emit(evt harness.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	select {
	case c.events <- evt:
	default:
	}
}

func mergedEnv(extra map[string]string, cwd ...string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	if len(cwd) > 0 && cwd[0] != "" {
		env = harness.SetEnv(env, "PWD", cwd[0])
	}
	return env
}
func buildExecArgs(cfg harness.SessionConfig, threadID, prompt string, images ...string) []string {
	effort := optionString(cfg.Options, "effort")
	if effort == "" {
		effort = EsforcoPadrao
	}
	model := cfg.Model
	if model == "" {
		model = ModeloPadrao
	}
	args := []string{"exec"}
	if threadID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json", "-m", model, "-c", "model_reasoning_effort="+effort)
	sandbox := optionString(cfg.Options, "sandbox")
	switch {
	case sandbox == "":
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	case threadID != "":
		// exec resume não tem -s; o -c equivale.
		args = append(args, "-c", "sandbox_mode=\""+sandbox+"\"")
	default:
		args = append(args, "--sandbox", sandbox)
	}
	// exec resume não aceita --profile: a retomada herda a configuração da thread.
	if profile := optionString(cfg.Options, "profile"); profile != "" && threadID == "" {
		args = append(args, "--profile", profile)
	}
	for _, kv := range harness.OptionStrings(cfg.Options, "config") {
		args = append(args, "-c", kv)
	}
	for _, dir := range harness.OptionStrings(cfg.Options, "add_dirs") {
		args = append(args, "--add-dir", dir)
	}
	for _, img := range images {
		// Forma com "=": --image aceita lista e engoliria o próximo argumento.
		args = append(args, "--image="+img)
	}
	// Argumentos nativos extras: intactos e na ordem, antes do id da thread e do prompt.
	args = append(args, harness.HarnessArgs(cfg.Options)...)
	if threadID != "" {
		args = append(args, threadID)
	}
	return append(args, "--", prompt)
}
func optionString(options map[string]interface{}, names ...string) string {
	for _, name := range names {
		if value, ok := options[name]; ok {
			if s, ok := value.(string); ok {
				return s
			}
			return fmt.Sprint(value)
		}
	}
	return ""
}

// parseJSONL aceita eventos do Codex e formas delta de versões anteriores.
func parseJSONL(line, sessionID string) []harness.Event {
	var raw map[string]interface{}
	if json.Unmarshal([]byte(line), &raw) != nil {
		return []harness.Event{{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: "JSONL inválido do codex: " + line}}}
	}
	typ, _ := raw["type"].(string)
	item, _ := raw["item"].(map[string]interface{})
	if item == nil {
		item = raw
	}
	itemType, _ := item["type"].(string)
	out := []harness.Event{}
	text := stringValue(raw, "delta")
	if text == "" {
		text = itemText(item)
	}
	if strings.Contains(typ, "text") || strings.Contains(typ, "message") || itemType == "agent_message" {
		if text != "" {
			out = append(out, harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: text}})
		}
	}
	if typ == "item.started" && (itemType == "command_execution" || itemType == "command") {
		out = append(out, harness.Event{Type: harness.EventToolCall, Payload: protocol.ToolCallParams{SessionID: sessionID, CallID: stringValue(item, "id"), Tool: "shell", Input: stringValue(item, "command")}})
	}
	if typ == "item.completed" && (itemType == "command_execution" || itemType == "command") {
		status := "success"
		if code, ok := numberValue(item, "exit_code"); ok && code != 0 {
			status = "error"
		}
		out = append(out, harness.Event{Type: harness.EventToolResult, Payload: protocol.ToolResultParams{SessionID: sessionID, CallID: stringValue(item, "id"), Status: status, Output: stringValue(item, "aggregated_output")}})
	}
	if typ == "error" || typ == "turn.failed" {
		msg := nestedErrorMessage(raw)
		out = append(out, harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: msg}})
	}
	if usage, ok := raw["usage"].(map[string]interface{}); ok {
		in, _ := numberValue(usage, "input_tokens")
		output, _ := numberValue(usage, "output_tokens")
		total, _ := numberValue(usage, "total_tokens")
		out = append(out, harness.Event{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: sessionID, InputTokens: in, OutputTokens: output, TotalTokens: total}})
	}
	return out
}
func nestedErrorMessage(m map[string]interface{}) string {
	if msg := stringValue(m, "message"); msg != "" {
		return msg
	}
	if msg := stringValue(m, "error"); msg != "" {
		return msg
	}
	if nested, ok := m["error"].(map[string]interface{}); ok {
		if msg := stringValue(nested, "message"); msg != "" {
			return msg
		}
		b, _ := json.Marshal(nested)
		return string(b)
	}
	return "erro do motor Codex sem mensagem"
}
func stringValue(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }
func itemText(item map[string]interface{}) string {
	if text := stringValue(item, "text"); text != "" {
		return text
	}
	content, ok := item["content"].([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range content {
		if part, ok := entry.(map[string]interface{}); ok {
			if text := stringValue(part, "text"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}
func numberValue(m map[string]interface{}, key string) (int64, bool) {
	if v, ok := m[key].(float64); ok {
		return int64(v), true
	}
	if s, ok := m[key].(string); ok {
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	}
	return 0, false
}
