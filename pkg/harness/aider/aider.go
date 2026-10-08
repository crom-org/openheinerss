package aider

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
	harness.Register("aider", protocol.HarnessCatalogItem{
		ID:                 "aider",
		DisplayName:        "Aider (AI Pair Programming in Terminal)",
		SupportedModes:     []string{"cli"},
		SupportedProtocols: []string{"openai", "anthropic", "ollama", "openrouter"},
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "openrouter",
				Name:        "OpenRouter",
				Endpoint:    "https://openrouter.ai/api",
				Models:      []string{"anthropic/claude-3.7-sonnet", "deepseek/deepseek-chat"},
				RequiresKey: true,
			},
			{
				ID:          "ollama-local",
				Name:        "Ollama Local",
				Endpoint:    "http://127.0.0.1:11434",
				Models:      []string{"qwen2.5-coder", "deepseek-r1"},
				RequiresKey: false,
			},
		},
	}, func(mode harness.Mode) (harness.Harness, error) {
		return NewAiderHarness(mode), nil
	})
}

// AiderHarness conector para o CLI do Aider
type AiderHarness struct {
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

// NewAiderHarness instancia o adaptador Aider
func NewAiderHarness(mode harness.Mode) *AiderHarness {
	return &AiderHarness{
		mode:   harness.ModeCLI,
		events: make(chan harness.Event, 200),
	}
}

func (a *AiderHarness) Name() string {
	return "aider"
}

func (a *AiderHarness) Mode() harness.Mode {
	return harness.ModeCLI
}

func (a *AiderHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if _, err := exec.LookPath("aider"); err != nil {
		return harness.PrerequisiteResult{
			Satisfied:    false,
			MissingItems: []string{"aider"},
			SuggestedFix: "Instale o Aider via 'pip install aider-chat' ou pipx.",
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (a *AiderHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cfg = cfg
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.stopped = false

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	a.env = env

	return nil
}

func (a *AiderHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("processo do aider não está ativo")
	}

	sessID := a.cfg.SessionID
	go func() {
		a.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Aider analisando repositório e histórico...",
			},
		})

		args := []string{"--no-git", "--yes", "--message", text}
		if a.cfg.Model != "" {
			args = append(args, "--model", a.cfg.Model)
		}

		cmd := exec.CommandContext(a.ctx, "aider", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Dir = a.cfg.CWD
		cmd.Env = a.env

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			a.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
			return
		}

		if err := cmd.Start(); err != nil {
			a.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		a.mu.Lock()
		a.cmd = cmd
		a.mu.Unlock()
		go drainStderr(stderr)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if events := harness.ParseJSONEvent(line, sessID); len(events) > 0 {
				for _, event := range events {
					a.emit(event)
				}
				continue
			}
			a.emit(harness.Event{
				Type: harness.EventText,
				Payload: protocol.TextParams{
					SessionID: sessID,
					Delta:     line + "\n",
				},
			})
		}

		err = cmd.Wait()
		a.mu.Lock()
		if a.cmd == cmd {
			a.cmd = nil
		}
		stopped := a.stopped
		a.mu.Unlock()
		if stopped {
			return
		}
		if err != nil {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
		}

		a.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    "completed",
			},
		})
	}()

	return nil
}

func drainStderr(r io.Reader) { _, _ = io.Copy(io.Discard, r) }

func (a *AiderHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin != nil {
		char := "n\n"
		if allow {
			char = "y\n"
		}
		_, err := a.stdin.Write([]byte(char))
		return err
	}
	return nil
}

func (a *AiderHarness) Events() <-chan harness.Event {
	return a.events
}

func (a *AiderHarness) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return nil
	}
	a.stopped = true

	if a.cancel != nil {
		a.cancel()
	}
	if a.stdin != nil {
		_ = a.stdin.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		if syscall.Kill(-a.cmd.Process.Pid, syscall.SIGINT) != nil {
			_ = a.cmd.Process.Signal(syscall.SIGINT)
		}
	}

	return nil
}

func (a *AiderHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sessID := a.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		if events := harness.ParseJSONEvent(line, sessID); len(events) > 0 {
			for _, event := range events {
				a.emit(event)
			}
			continue
		}
		a.emit(harness.Event{
			Type: harness.EventText,
			Payload: protocol.TextParams{
				SessionID: sessID,
				Delta:     line + "\n",
			},
		})
	}

	a.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
}

func (a *AiderHarness) emit(evt harness.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return
	}
	select {
	case a.events <- evt:
	default:
	}
}
