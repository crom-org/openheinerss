package opencode

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
	harness.Register("opencode", protocol.HarnessCatalogItem{
		ID:                 "opencode",
		DisplayName:        "OpenCode Interpreter & Server",
		SupportedModes:     []string{"cli", "api"},
		SupportedProtocols: []string{"openai", "deepseek", "ollama", "groq"},
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "deepseek",
				Name:        "DeepSeek Oficial",
				Endpoint:    "https://api.deepseek.com/v1",
				Models:      []string{"deepseek-chat", "deepseek-coder"},
				RequiresKey: true,
			},
			{
				ID:          "ollama-local",
				Name:        "Ollama Local",
				Endpoint:    "http://127.0.0.1:11434/v1",
				Models:      []string{"qwen2.5-coder:latest", "deepseek-r1:14b"},
				RequiresKey: false,
			},
			{
				ID:          "groq",
				Name:        "Groq Cloud (Fast Inference)",
				Endpoint:    "https://api.groq.com/openai/v1",
				Models:      []string{"llama-3.3-70b-versatile"},
				RequiresKey: true,
			},
			{
				ID:          "openai",
				Name:        "OpenAI Platform",
				Endpoint:    "https://api.openai.com/v1",
				Models:      []string{"gpt-4o", "gpt-4o-mini", "o1-mini"},
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
	mu      sync.Mutex
	mode    harness.Mode
	cfg     harness.SessionConfig
	env     []string
	events  chan harness.Event
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
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
	o.stopped = false

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	o.env = env

	return nil
}

func (o *OpenCodeHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.stopped {
		return fmt.Errorf("processo do OpenCode não está ativo")
	}

	sessID := o.cfg.SessionID
	go func() {
		o.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Consultando OpenCode Interpreter...",
			},
		})

		args := []string{"run"}
		if o.cfg.Model != "" {
			args = append(args, "-m", o.cfg.Model)
		}
		args = append(args, text)

		cmd := exec.CommandContext(o.ctx, "opencode", args...)
		cmd.Dir = o.cfg.CWD
		cmd.Env = o.env

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			o.emit(harness.Event{
				Type: harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}

		if err := cmd.Start(); err != nil {
			o.emit(harness.Event{
				Type: harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			o.emit(harness.Event{
				Type: harness.EventText,
				Payload: protocol.TextParams{
					SessionID: sessID,
					Delta:     line + "\n",
				},
			})
		}

		_ = cmd.Wait()

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
		_ = o.cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

func (o *OpenCodeHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	sessID := o.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		o.emit(harness.Event{
			Type: harness.EventText,
			Payload: protocol.TextParams{
				SessionID: sessID,
				Delta:     line + "\n",
			},
		})
	}

	o.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
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
