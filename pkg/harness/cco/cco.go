package cco

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	harness.Register("cco", protocol.HarnessCatalogItem{ID: "cco", DisplayName: "Claude Code Open (CCO)", SupportedModes: []string{"cli"}, SupportedProtocols: []string{"anthropic"}}, func(mode harness.Mode) (harness.Harness, error) {
		if mode == "" || mode == harness.ModeMock {
			mode = harness.ModeCLI
		}
		return New(mode), nil
	})
}

type Harness struct {
	mu      sync.Mutex
	mode    harness.Mode
	cfg     harness.SessionConfig
	events  chan harness.Event
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
	cmd     *exec.Cmd
}

func New(mode harness.Mode) *Harness {
	return &Harness{mode: mode, events: make(chan harness.Event, 200)}
}
func (c *Harness) Name() string       { return "cco" }
func (c *Harness) Mode() harness.Mode { return c.mode }
func (c *Harness) ValidatePrerequisites(context.Context) harness.PrerequisiteResult {
	if _, err := exec.LookPath("cco"); err != nil {
		return harness.PrerequisiteResult{MissingItems: []string{"cco"}, SuggestedFix: "Instale o CCO e adicione cco ao PATH."}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}
func (c *Harness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false
	return nil
}
func (c *Harness) SendPrompt(ctx context.Context, text string, _ []protocol.Attachment) error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return fmt.Errorf("processo do CCO não está ativo")
	}
	cfg, base := c.cfg, c.ctx
	c.mu.Unlock()
	if ctx != nil {
		base = ctx
	}
	args := []string{}
	if cfg.Provider != "" {
		args = append(args, "--provider", cfg.Provider)
	}
	if effort := option(cfg.Options, "effort"); effort != "" {
		args = append(args, "--effort", effort)
	}
	args = append(args, "-p", text)
	envExtra := cfg.Env
	if cfg.Model != "" {
		if envExtra == nil {
			envExtra = map[string]string{}
		}
		envExtra["CCO_MODEL"] = cfg.Model
	}
	cmd := exec.CommandContext(base, "cco", args...)
	cmd.Dir = cfg.CWD
	cmd.Env = mergedEnv(envExtra)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout do cco: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar cco: %w", err)
	}
	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()
	go func() {
		s := bufio.NewScanner(out)
		for s.Scan() {
			c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: cfg.SessionID, Delta: s.Text() + "\n"}})
		}
		err := cmd.Wait()
		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if !stopped {
			reason := "completed"
			if err != nil {
				c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: cfg.SessionID, Message: err.Error()}})
				reason = "process_error"
			}
			c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: cfg.SessionID, Reason: reason}})
		}
	}()
	return nil
}
func (c *Harness) RespondPermission(context.Context, string, bool, string) error { return nil }
func (c *Harness) Events() <-chan harness.Event                                  { return c.events }
func (c *Harness) Stop() error {
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
		return c.cmd.Process.Signal(syscall.SIGINT)
	}
	return nil
}
func (c *Harness) emit(e harness.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		select {
		case c.events <- e:
		default:
		}
	}
}
func option(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}
func mergedEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}
