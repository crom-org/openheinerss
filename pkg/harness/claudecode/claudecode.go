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
	"sync"
	"syscall"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	harness.Register("claude-code", protocol.HarnessCatalogItem{
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

// ClaudeCodeHarness implementa o conector para o Claude Code nos modos SDK e CLI
type ClaudeCodeHarness struct {
	mu      sync.Mutex
	mode    harness.Mode
	cfg     harness.SessionConfig
	events  chan harness.Event
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
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
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false

	// Isola perfil em ~/.openheinerss/profiles/claude-<provider>
	globalDir, _ := config.GetGlobalDir()
	providerName := cfg.Provider
	if providerName == "" {
		providerName = "default"
	}
	profileDir := filepath.Join(globalDir, "profiles", fmt.Sprintf("claude-%s", providerName))
	_ = os.MkdirAll(profileDir, 0755)

	env := os.Environ()
	env = append(env,
		fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profileDir),
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_PROMPT_CACHING=1",
		"API_TIMEOUT_MS=600000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)

	if cfg.Model != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_MODEL=%s", cfg.Model))
	}
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	var cmd *exec.Cmd
	if c.mode == harness.ModeSDK {
		cmd = exec.CommandContext(c.ctx, "node", "-e", NodeWorkerScript)
	} else {
		// Modo CLI direto com flag de print/streaming quando apropriado
		cmd = exec.CommandContext(c.ctx, "claude", "--print")
	}

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
	if c.mode == harness.ModeSDK {
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

	if c.stopped || c.stdin == nil {
		return fmt.Errorf("processo do Claude Code não está ativo")
	}

	if c.mode == harness.ModeSDK {
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

	// Modo CLI: escreve linha direta no terminal do subprocesso
	_, err := fmt.Fprintf(c.stdin, "%s\n", text)
	return err
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
		_ = c.cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

func (c *ClaudeCodeHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
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
