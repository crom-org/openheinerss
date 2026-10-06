package agy

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
	harness.Register("agy", protocol.HarnessCatalogItem{
		ID:                 "agy",
		DisplayName:        "Google Antigravity (AGY Agent Suite)",
		SupportedModes:     []string{"cli", "sdk"},
		SupportedProtocols: []string{"antigravity", "gemini"},
		DefaultProviders: []protocol.ProviderInfo{
			{
				ID:          "google-deepmind",
				Name:        "Google DeepMind / Gemini",
				Endpoint:    "https://generativelanguage.googleapis.com",
				Models:      []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-3.8-flash"},
				RequiresKey: true,
			},
		},
	}, func(mode harness.Mode) (harness.Harness, error) {
		if mode == "" || mode == harness.ModeMock {
			mode = harness.ModeCLI
		}
		return NewAGYHarness(mode), nil
	})
}

// AGYHarness conector para o ecossistema Antigravity (AGY)
type AGYHarness struct {
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

// NewAGYHarness instancia o adaptador AGY
func NewAGYHarness(mode harness.Mode) *AGYHarness {
	return &AGYHarness{
		mode:   mode,
		events: make(chan harness.Event, 200),
	}
}

func (a *AGYHarness) Name() string {
	return "agy"
}

func (a *AGYHarness) Mode() harness.Mode {
	return a.mode
}

func (a *AGYHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	if a.mode == harness.ModeCLI {
		if _, err := exec.LookPath("agy"); err != nil {
			return harness.PrerequisiteResult{
				Satisfied:    false,
				MissingItems: []string{"agy"},
				SuggestedFix: "Instale o CLI 'agy' (Google Antigravity CLI) ou adicione ao PATH.",
			}
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (a *AGYHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cfg = cfg
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.stopped = false

	if a.mode == harness.ModeCLI {
		cmd := exec.CommandContext(a.ctx, "agy", "run")
		cmd.Dir = cfg.CWD

		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdin para agy: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdout para agy: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("falha ao iniciar agy: %w", err)
		}

		a.cmd = cmd
		a.stdin = stdin

		go a.readEvents(stdout)
	}

	return nil
}

func (a *AGYHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("harness agy não está ativo")
	}

	sessID := a.cfg.SessionID

	if a.mode == harness.ModeSDK || a.stdin == nil {
		go func() {
			a.emit(harness.Event{
				Type: harness.EventThinking,
				Payload: protocol.ThinkingParams{
					SessionID: sessID,
					Delta:     "Antigravity analisando com skills e regras do workspace...",
				},
			})
			a.emit(harness.Event{
				Type: harness.EventText,
				Payload: protocol.TextParams{
					SessionID: sessID,
					Delta:     fmt.Sprintf("[AGY] Executando comando com modelo %s: %s", a.cfg.Model, text),
				},
			})
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

	_, err := fmt.Fprintf(a.stdin, "%s\n", text)
	return err
}

func (a *AGYHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
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

func (a *AGYHarness) Events() <-chan harness.Event {
	return a.events
}

func (a *AGYHarness) Stop() error {
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
		_ = a.cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

func (a *AGYHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	sessID := a.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
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

func (a *AGYHarness) emit(evt harness.Event) {
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
