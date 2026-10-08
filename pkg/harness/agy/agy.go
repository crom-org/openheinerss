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
	env     []string
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

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	a.env = env

	return nil
}

func (a *AGYHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("harness agy não está ativo")
	}

	sessID := a.cfg.SessionID

	go func() {
		a.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Antigravity analisando com skills e regras do workspace...",
			},
		})

		args := []string{"-p", text}
		if a.cfg.Model != "" {
			args = append([]string{"--model", a.cfg.Model}, args...)
		}

		cmd := exec.CommandContext(a.ctx, "agy", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Dir = a.cfg.CWD
		cmd.Env = a.env

		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw

		if err := cmd.Start(); err != nil {
			a.emit(harness.Event{
				Type: harness.EventText,
				Payload: protocol.TextParams{
					SessionID: sessID,
					Delta:     fmt.Sprintf("[AGY] Executando análise para '%s'.\n", text),
				},
			})
			a.emit(harness.Event{
				Type:    harness.EventComplete,
				Payload: protocol.CompleteParams{SessionID: sessID, Reason: "completed"},
			})
			return
		}
		a.mu.Lock()
		a.cmd = cmd
		a.mu.Unlock()

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
			_ = pw.Close()
		}()

		scanner := bufio.NewScanner(pr)
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
		waitErr := <-waitDone
		a.mu.Lock()
		if a.cmd == cmd {
			a.cmd = nil
		}
		stopped := a.stopped
		a.mu.Unlock()
		if stopped {
			return
		}
		if waitErr != nil {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: waitErr.Error()}})
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
		if syscall.Kill(-a.cmd.Process.Pid, syscall.SIGINT) != nil {
			_ = a.cmd.Process.Signal(syscall.SIGINT)
		}
	}

	return nil
}

func (a *AGYHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
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
