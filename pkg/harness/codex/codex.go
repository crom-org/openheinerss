package codex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	harness.Register("codex", protocol.HarnessCatalogItem{
		ID:                 "codex",
		DisplayName:        "OpenAI Codex / Assistant Engine",
		SupportedModes:     []string{"api", "cli"},
		SupportedProtocols: []string{"openai"},
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "openai",
				Name:        "OpenAI API",
				Endpoint:    "https://api.openai.com/v1",
				Models:      []string{"gpt-4o", "gpt-4o-mini", "o1", "o1-mini", "o3-mini"},
				RequiresKey: true,
			},
			{
				ID:          "azure-openai",
				Name:        "Azure OpenAI Service",
				Endpoint:    "https://your-resource.openai.azure.com",
				Models:      []string{"gpt-4o"},
				RequiresKey: true,
			},
		},
	}, func(mode harness.Mode) (harness.Harness, error) {
		if mode == "" || mode == harness.ModeMock {
			mode = harness.ModeAPI
		}
		return NewCodexHarness(mode), nil
	})
}

// CodexHarness conector para o ecossistema OpenAI Codex e Assistants
type CodexHarness struct {
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

// NewCodexHarness instancia o adaptador Codex
func NewCodexHarness(mode harness.Mode) *CodexHarness {
	return &CodexHarness{
		mode:   mode,
		events: make(chan harness.Event, 200),
	}
}

func (c *CodexHarness) Name() string {
	return "codex"
}

func (c *CodexHarness) Mode() harness.Mode {
	return c.mode
}

func (c *CodexHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if c.mode == harness.ModeCLI {
		if _, err := exec.LookPath("codex"); err != nil {
			return harness.PrerequisiteResult{
				Satisfied:    false,
				MissingItems: []string{"codex"},
				SuggestedFix: "Instale o OpenAI CLI ou Codex binário no PATH.",
			}
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (c *CodexHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg = cfg
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Se for modo CLI, dispara o subprocesso correspondente
	if c.mode == harness.ModeCLI {
		cmd := exec.CommandContext(c.ctx, "codex", "run")
		cmd.Dir = cfg.CWD
		cmd.Env = env

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdin para codex: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdout para codex: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("falha ao iniciar codex CLI: %w", err)
		}

		c.cmd = cmd
		c.stdin = stdin

		go c.readEvents(stdout)
	}

	return nil
}

func (c *CodexHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return fmt.Errorf("harness codex não está ativo")
	}

	sessID := c.cfg.SessionID

	// Em modo API, emite pensamento e streaming simulando resposta de assistente
	if c.mode == harness.ModeAPI || c.stdin == nil {
		go func() {
			c.emit(harness.Event{
				Type: harness.EventThinking,
				Payload: protocol.ThinkingParams{
					SessionID: sessID,
					Delta:     fmt.Sprintf("Processando com OpenAI %s...", c.cfg.Model),
				},
			})
			c.emit(harness.Event{
				Type: harness.EventText,
				Payload: protocol.TextParams{
					SessionID: sessID,
					Delta:     fmt.Sprintf("[Codex %s] Executando análise para '%s'.", c.cfg.Model, text),
				},
			})
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

	_, err := fmt.Fprintf(c.stdin, "%s\n", text)
	return err
}

func (c *CodexHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin != nil {
		char := "n\n"
		if allow {
			char = "y\n"
		}
		_, err := c.stdin.Write([]byte(char))
		return err
	}
	return nil
}

func (c *CodexHarness) Events() <-chan harness.Event {
	return c.events
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
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

func (c *CodexHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	sessID := c.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
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

	c.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
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
