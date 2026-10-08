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

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/process"
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
	resumeID := o.resumeID
	go func() {
		o.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Consultando OpenCode Interpreter...",
			},
		})

		args := []string{"run", "--format", "json"}
		if resumeID != "" {
			args = append(args, "--session", resumeID)
		}
		if o.cfg.Model != "" {
			args = append(args, "-m", o.cfg.Model)
		}
		args = append(args, text)

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
		stderrDone := make(chan struct{})
		go func() { _, _ = io.Copy(&stderrBuf, stderr); close(stderrDone) }()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			o.handleLine(scanner.Text(), sessID)
		}

		err = cmd.Wait()
		<-stderrDone
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
	for _, event := range parseOpenCodeEvent(raw, sessionID) {
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
