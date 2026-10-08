package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	register := func(name string) {
		harness.Register(name, protocol.HarnessCatalogItem{
			ID:                 "claude-code",
			DisplayName:        "Claude Code (Anthropic & Provedores Abertos)",
			SupportedModes:     []string{"sdk", "cli"},
			SupportedProtocols: []string{"anthropic"},
			DefaultProviders: []protocol.ProviderInfo{
				{
					ID:          "claude-native",
					Name:        "Anthropic Oficial (Assinatura)",
					Endpoint:    "https://api.anthropic.com",
					Models:      []string{"claude-3-7-sonnet-latest", "claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"},
					RequiresKey: true,
				},
				{
					ID:          "openrouter",
					Name:        "OpenRouter AI",
					Endpoint:    "https://openrouter.ai/api",
					Models:      []string{"anthropic/claude-3.7-sonnet", "qwen/qwen-2.5-coder-32b-instruct"},
					RequiresKey: true,
				},
				{
					ID:          "opencode-zen",
					Name:        "OpenCode Zen (Modelos Gratuitos)",
					Endpoint:    "https://opencode.ai/zen",
					Models:      []string{"space-bunny-free"},
					RequiresKey: false,
				},
			},
		}, func(mode harness.Mode) (harness.Harness, error) {
			if mode == "" || mode == harness.ModeMock {
				mode = harness.ModeCLI
			}
			return NewClaudeCodeHarness(mode), nil
		})
	}
	register("claude-code")
}

// ClaudeCodeHarness implementa o conector para o Claude Code nos modos SDK e CLI
type ClaudeCodeHarness struct {
	mu       sync.Mutex
	mode     harness.Mode
	cfg      harness.SessionConfig
	env      []string
	events   chan harness.Event
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	ctx      context.Context
	cancel   context.CancelFunc
	stopped  bool
	resumeID string
}

// NewClaudeCodeHarness instancia o adaptador Claude Code
func NewClaudeCodeHarness(mode harness.Mode) *ClaudeCodeHarness {
	return &ClaudeCodeHarness{
		mode:   mode,
		events: make(chan harness.Event, 200),
	}
}

func (c *ClaudeCodeHarness) Name() string {
	return "claude-code"
}

func (c *ClaudeCodeHarness) Mode() harness.Mode {
	return c.mode
}

func (c *ClaudeCodeHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if c.mode == harness.ModeSDK {
		if _, err := exec.LookPath("node"); err != nil {
			return harness.PrerequisiteResult{
				Satisfied:    false,
				MissingItems: []string{"node"},
				SuggestedFix: "Node.js 18+ é necessário para o modo SDK. Instale via nvm ('nvm install 20') ou use o modo CLI.",
			}
		}
		return harness.PrerequisiteResult{Satisfied: true}
	}

	// Modo CLI
	if _, err := exec.LookPath("claude"); err != nil {
		return harness.PrerequisiteResult{
			Satisfied:    false,
			MissingItems: []string{"claude"},
			SuggestedFix: "Instale o Claude Code CLI via 'npm install -g @anthropic-ai/claude-code'.",
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (c *ClaudeCodeHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg = cfg
	c.resumeID = cfg.SessionID
	if c.mode == harness.ModeCLI && optionString(cfg.Options, "claude_session_id", "resume_session", "session_id") == "" {
		// A sessão criada pelo CLI só pode ser retomada depois que o primeiro
		// stream informar o session_id real do Claude.
		c.resumeID = ""
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false

	env := os.Environ()
	env = append(env,
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_PROMPT_CACHING=1",
		"API_TIMEOUT_MS=600000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)

	globalDir, _ := config.GetGlobalDir()
	if cfg.Provider != "" && cfg.Provider != "default" {
		profileDir := filepath.Join(globalDir, "profiles", fmt.Sprintf("claude-%s", cfg.Provider))
		_ = os.MkdirAll(profileDir, 0755)
		env = append(env, fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profileDir))
	}

	if cfg.Model != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_MODEL=%s", cfg.Model))
	}
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	c.env = env

	if c.mode == harness.ModeSDK {
		cmd := exec.CommandContext(c.ctx, "node", "-e", NodeWorkerScript)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Dir = cfg.CWD
		cmd.Env = env

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdin pipe: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdout pipe: %w", err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stderr pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("falha ao iniciar processo do Claude Code: %w", err)
		}

		c.cmd = cmd
		c.stdin = stdin

		// Goroutines de leitura e streaming
		go c.readEvents(stdout)
		go c.readStderr(stderr)

		// Se for modo SDK, envia handshake de inicialização
		initPayload := map[string]interface{}{
			"method": "init",
			"params": map[string]interface{}{
				"cwd":            cfg.CWD,
				"env":            cfg.Env,
				"model":          cfg.Model,
				"permissionMode": cfg.PermissionMode,
			},
		}
		data, _ := json.Marshal(initPayload)
		_, _ = fmt.Fprintf(c.stdin, "%s\n", data)
	}

	return nil
}

func (c *ClaudeCodeHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return fmt.Errorf("processo do Claude Code não está ativo")
	}

	if c.mode == harness.ModeSDK {
		if c.stdin == nil {
			return fmt.Errorf("stdin do Claude Code SDK indisponível")
		}
		payload := map[string]interface{}{
			"method": "prompt",
			"params": map[string]interface{}{
				"text":   text,
				"images": attachments,
			},
		}
		data, _ := json.Marshal(payload)
		_, err := fmt.Fprintf(c.stdin, "%s\n", data)
		return err
	}

	// Modo CLI: executa claude -p com streaming em tempo real
	sessID := c.cfg.SessionID
	go func() {
		c.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Consultando Claude Code CLI...",
			},
		})

		args := []string{"-p", text, "--output-format", "stream-json", "--verbose"}
		if c.cfg.Model != "" {
			args = append(args, "--model", c.cfg.Model)
		}
		if resume := c.resumeOption(); resume != "" {
			args = append([]string{"--resume", resume}, args...)
		}
		if permission := c.cliPermissionMode(); permission != "" {
			args = append(args, "--permission-mode", permission)
		}
		cmd := exec.CommandContext(c.ctx, "claude", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Dir = c.cfg.CWD
		cmd.Env = c.env

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			c.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
			return
		}

		if err := cmd.Start(); err != nil {
			c.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		c.mu.Lock()
		c.cmd = cmd
		c.mu.Unlock()
		go c.readStderr(stderr)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for scanner.Scan() {
			c.parseCLIEvent(scanner.Bytes(), sessID)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: "falha lendo stream do Claude: " + scanErr.Error()}})
		}

		err = cmd.Wait()
		c.mu.Lock()
		if c.cmd == cmd {
			c.cmd = nil
		}
		stopped := c.stopped
		c.mu.Unlock()
		if stopped {
			return
		}
		if err != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
		}

		c.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    "completed",
			},
		})
	}()

	return nil
}

func (c *ClaudeCodeHarness) resumeOption() string {
	for _, key := range []string{"claude_session_id", "resume_session", "session_id"} {
		if value, ok := c.cfg.Options[key].(string); ok && value != "" {
			return value
		}
	}
	return c.resumeID
}

func optionString(options map[string]interface{}, names ...string) string {
	for _, name := range names {
		if value, ok := options[name]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func (c *ClaudeCodeHarness) cliPermissionMode() string {
	value, _ := c.cfg.Options["permissoes"].(string)
	if value == "" {
		value = c.cfg.PermissionMode
	}
	switch strings.ToLower(value) {
	case "pular", "bypass", "bypasspermissions", "always_allow":
		return "bypassPermissions"
	case "perguntar", "ask", "manual":
		return "manual"
	}
	if rodar, ok := c.cfg.Options["rodar"].(bool); ok && rodar {
		return "bypassPermissions"
	}
	return ""
}

// ResumeID permite ao orquestrador retomar a conversa retornada pelo Claude.
func (c *ClaudeCodeHarness) ResumeID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumeID
}

func (c *ClaudeCodeHarness) parseCLIEvent(data []byte, fallbackSession string) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: fallbackSession, Delta: string(data) + "\n"}})
		return
	}
	sessionID := stringField(msg, "session_id")
	if sessionID == "" {
		sessionID = fallbackSession
	}
	if sessionID != "" {
		c.mu.Lock()
		c.resumeID = sessionID
		c.mu.Unlock()
	}
	switch stringField(msg, "type") {
	case "system":
		// init só estabelece o ID; post_turn_summary não é texto para a Central.
	case "assistant":
		message, _ := msg["message"].(map[string]interface{})
		c.parseContentBlocks(message["content"], sessionID)
	case "user":
		message, _ := msg["message"].(map[string]interface{})
		c.parseContentBlocks(message["content"], sessionID)
	case "result":
		usage, _ := msg["usage"].(map[string]interface{})
		if usage != nil {
			c.emit(harness.Event{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: sessionID, InputTokens: int64(number(usage, "input_tokens")), OutputTokens: int64(number(usage, "output_tokens")), TotalTokens: int64(number(usage, "input_tokens") + number(usage, "output_tokens")), CostUSD: number(msg, "total_cost_usd")}})
		}
		isError := boolField(msg, "is_error")
		reason := stringField(msg, "subtype")
		if reason == "" {
			reason = stringField(msg, "stop_reason")
		}
		if quotaMessage(msg) {
			isError = true
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: "limite de cota do Claude Code atingido"}})
		}
		if isError {
			message := stringField(msg, "result")
			if message == "" {
				message = "Claude Code encerrou com erro"
			}
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: message}})
		}
		c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: reason}})
	case "rate_limit_event":
		info, _ := msg["rate_limit_info"].(map[string]interface{})
		// Só status "rejected" é cota esgotada; overageStatus "rejected" quer dizer apenas que o uso extra pago está desligado.
		if stringField(msg, "status") == "rejected" || stringField(info, "status") == "rejected" {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: "limite de cota do Claude Code atingido"}})
		}
	}
}

func (c *ClaudeCodeHarness) parseContentBlocks(value interface{}, sessionID string) {
	blocks, _ := value.([]interface{})
	for _, raw := range blocks {
		block, _ := raw.(map[string]interface{})
		switch stringField(block, "type") {
		case "text":
			c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: stringField(block, "text")}})
		case "thinking":
			c.emit(harness.Event{Type: harness.EventThinking, Payload: protocol.ThinkingParams{SessionID: sessionID, Delta: stringField(block, "thinking")}})
		case "tool_use":
			c.emit(harness.Event{Type: harness.EventToolCall, Payload: protocol.ToolCallParams{SessionID: sessionID, CallID: stringField(block, "id"), Tool: stringField(block, "name"), Input: block["input"]}})
		case "tool_result":
			status := "success"
			if boolField(block, "is_error") {
				status = "error"
			}
			output := block["content"]
			out, _ := json.Marshal(output)
			if text, ok := output.(string); ok {
				out = []byte(text)
			}
			c.emit(harness.Event{Type: harness.EventToolResult, Payload: protocol.ToolResultParams{SessionID: sessionID, CallID: stringField(block, "tool_use_id"), Status: status, Output: string(out)}})
		}
	}
}

func stringField(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }
func boolField(m map[string]interface{}, key string) bool     { v, _ := m[key].(bool); return v }
func number(m map[string]interface{}, key string) float64     { v, _ := m[key].(float64); return v }
func quotaMessage(m map[string]interface{}) bool {
	data, _ := json.Marshal(m)
	s := strings.ToLower(string(data))
	for _, marker := range []string{"usage limit", "rate limit", "rate_limit", "quota", "limite de uso", "limite de cota", "out of credits"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func (c *ClaudeCodeHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped || c.stdin == nil {
		return fmt.Errorf("processo do Claude Code não está ativo")
	}

	if c.mode == harness.ModeSDK {
		payload := map[string]interface{}{
			"method": "permission_respond",
			"params": map[string]interface{}{
				"requestId": reqID,
				"allow":     allow,
				"message":   message,
			},
		}
		data, _ := json.Marshal(payload)
		_, err := fmt.Fprintf(c.stdin, "%s\n", data)
		return err
	}

	// Modo CLI: responde com caractere interativo ('y' ou 'n')
	char := "n\n"
	if allow {
		char = "y\n"
	}
	_, err := c.stdin.Write([]byte(char))
	return err
}

func (c *ClaudeCodeHarness) Events() <-chan harness.Event {
	return c.events
}

func (c *ClaudeCodeHarness) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return nil
	}
	c.stopped = true

	if c.cancel != nil {
		c.cancel()
	}

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// Envia sinal SIGINT limpo antes de encerrar
		if syscall.Kill(-c.cmd.Process.Pid, syscall.SIGINT) != nil {
			_ = c.cmd.Process.Signal(syscall.SIGINT)
		}
	}

	return nil
}

func (c *ClaudeCodeHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sessID := c.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		if c.mode == harness.ModeSDK {
			// Parse de mensagens NDJSON do worker
			var msg struct {
				Method string                 `json:"method"`
				Params map[string]interface{} `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &msg); err == nil && msg.Method != "" {
				c.handleSDKMessage(msg.Method, msg.Params)
				continue
			}
		}

		// Fallback para streaming textual (modo CLI ou mensagens de texto puro)
		if events := harness.ParseJSONEvent(line, sessID); len(events) > 0 {
			for _, event := range events {
				c.emit(event)
			}
			continue
		}
		c.emit(harness.Event{
			Type: harness.EventText,
			Payload: protocol.TextParams{
				SessionID: sessID,
				Delta:     line + "\n",
			},
		})
	}

	// Ao fechar a saída do processo
	c.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
}

func (c *ClaudeCodeHarness) handleSDKMessage(method string, params map[string]interface{}) {
	sessID := c.cfg.SessionID

	switch method {
	case "agent.thinking":
		delta, _ := params["delta"].(string)
		c.emit(harness.Event{
			Type:    harness.EventThinking,
			Payload: protocol.ThinkingParams{SessionID: sessID, Delta: delta},
		})
	case "agent.text":
		delta, _ := params["delta"].(string)
		c.emit(harness.Event{
			Type:    harness.EventText,
			Payload: protocol.TextParams{SessionID: sessID, Delta: delta},
		})
	case "agent.permission_request":
		reqID, _ := params["requestId"].(string)
		tool, _ := params["tool"].(string)
		command, _ := params["command"].(string)
		risk, _ := params["risk"].(string)
		c.emit(harness.Event{
			Type: harness.EventPermission,
			Payload: protocol.PermissionRequestParams{
				SessionID: sessID,
				RequestID: reqID,
				Tool:      tool,
				Command:   command,
				Risk:      risk,
			},
		})
	case "agent.complete":
		reason, _ := params["reason"].(string)
		c.emit(harness.Event{
			Type:    harness.EventComplete,
			Payload: protocol.CompleteParams{SessionID: sessID, Reason: reason},
		})
	case "agent.error":
		msg, _ := params["message"].(string)
		c.emit(harness.Event{
			Type:    harness.EventError,
			Payload: protocol.ErrorParams{SessionID: sessID, Message: msg},
		})
	}
}

func (c *ClaudeCodeHarness) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		// Stderr do processo pode ser logado ou monitorado
	}
}

func (c *ClaudeCodeHarness) emit(evt harness.Event) {
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
