package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	register := func(name string) {
		harness.Register(name, protocol.HarnessCatalogItem{ID: name, DisplayName: "OpenAI Codex / Assistant Engine", SupportedModes: []string{"cli", "api"}, SupportedProtocols: []string{"openai"}}, func(mode harness.Mode) (harness.Harness, error) {
			if mode == "" || mode == harness.ModeMock {
				mode = harness.ModeCLI
			}
			return NewCodexHarness(mode), nil
		})
	}
	register("codex")
}

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
	c.stopped = false
	c.threadID = optionString(cfg.Options, "codex_session_id", "resume_session")
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
		go func() {
			c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: c.cfg.SessionID, Delta: fmt.Sprintf("[Codex %s] %s", c.cfg.Model, text)}})
			c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: c.cfg.SessionID, Reason: "completed"}})
		}()
		return nil
	}
	if c.mode != harness.ModeCLI {
		return fmt.Errorf("modo Codex não suportado: %s; use cli", c.mode)
	}
	cmdCtx := c.ctx
	if ctx != nil {
		cmdCtx = ctx
	}
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	cmd := exec.CommandContext(cmdCtx, "codex", buildExecArgs(c.cfg, c.threadID, text)...)
	cmd.Dir, cmd.Env = c.cfg.CWD, mergedEnv(c.cfg.Env)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout do codex: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr do codex: %w", err)
	}
	if err := cmd.Start(); err != nil {
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

func (c *CodexHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	return nil
}
func (c *CodexHarness) Events() <-chan harness.Event { return c.events }
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
		return c.cmd.Process.Signal(syscall.SIGINT)
	}
	return nil
}

func (c *CodexHarness) readJSONL(r io.Reader, sessionID string) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
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
		for _, event := range parseJSONL(line, sessionID) {
			c.emit(event)
		}
	}
	if err := s.Err(); err != nil {
		c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: err.Error()}})
	}
}
func (c *CodexHarness) readStderr(r io.Reader, sessionID string) {
	for s := bufio.NewScanner(r); s.Scan(); {
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

func mergedEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}
func buildExecArgs(cfg harness.SessionConfig, threadID, prompt string) []string {
	effort := optionString(cfg.Options, "effort")
	if effort == "" {
		effort = "medium"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-reserve"
	}
	if threadID != "" {
		return []string{"exec", "resume", "--json", "-m", model, "-c", "model_reasoning_effort=" + effort, "--dangerously-bypass-approvals-and-sandbox", threadID, prompt}
	}
	return []string{"exec", "--json", "-m", model, "-c", "model_reasoning_effort=" + effort, "--dangerously-bypass-approvals-and-sandbox", prompt}
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
		msg := stringValue(raw, "message")
		if msg == "" {
			msg = stringValue(raw, "error")
		}
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
