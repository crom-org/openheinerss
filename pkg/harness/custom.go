package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"gopkg.in/yaml.v3"
)

// CustomSpec é o formato de .openheinerss/harnesses/*.yaml (JSON também é aceito).
type CustomSpec struct {
	Name        string            `json:"name" yaml:"name"`
	Base        string            `json:"base,omitempty" yaml:"base,omitempty"`
	DisplayName string            `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Model       string            `json:"model,omitempty" yaml:"model,omitempty"`
	Modelo      string            `json:"modelo,omitempty" yaml:"modelo,omitempty"`
	Effort      string            `json:"effort,omitempty" yaml:"effort,omitempty"`
	Prompt      string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	FinishRegex string            `json:"finishRegex,omitempty" yaml:"finishRegex,omitempty"`
	QuotaRegex  string            `json:"quotaRegex,omitempty" yaml:"quotaRegex,omitempty"`
	Reserva     []string          `json:"reserva,omitempty" yaml:"reserva,omitempty"`
}

var customMu sync.RWMutex
var customSpecs = map[string]CustomSpec{}

// RegisterCustom registra um harness NDJSON sem recompilar o binário.
func RegisterCustom(spec CustomSpec) error {
	resolved, err := resolveSpec(spec, map[string]bool{})
	if err != nil {
		return err
	}
	if resolved.Name == "" {
		return fmt.Errorf("harness custom: campo name é obrigatório")
	}
	if resolved.Model == "" {
		resolved.Model = resolved.Modelo
	}
	if resolved.Command == "" && resolved.Base == "" {
		return fmt.Errorf("harness custom '%s': informe base ou command", resolved.Name)
	}
	if resolved.Prompt == "" {
		resolved.Prompt = "stdin"
	}
	if resolved.Prompt != "stdin" && resolved.Prompt != "argument" {
		return fmt.Errorf("harness custom '%s': prompt deve ser stdin ou argument", resolved.Name)
	}
	if resolved.FinishRegex != "" {
		if _, err := regexp.Compile(resolved.FinishRegex); err != nil {
			return fmt.Errorf("harness custom '%s': finishRegex inválido: %w", resolved.Name, err)
		}
	}
	if resolved.QuotaRegex != "" {
		if _, err := regexp.Compile(resolved.QuotaRegex); err != nil {
			return fmt.Errorf("harness custom '%s': quotaRegex inválido: %w", resolved.Name, err)
		}
	}
	customMu.Lock()
	customSpecs[resolved.Name] = resolved
	customMu.Unlock()
	meta := protocol.HarnessCatalogItem{ID: resolved.Name, DisplayName: resolved.DisplayName, SupportedModes: []string{"cli"}, SupportedProtocols: []string{"ndjson"}, Origin: "custom"}
	if meta.DisplayName == "" {
		meta.DisplayName = resolved.Name
	}
	Register(resolved.Name, meta, func(mode Mode) (Harness, error) {
		if mode == "" || mode == ModeMock {
			mode = ModeCLI
		}
		if resolved.Base != "" && resolved.Command == "" {
			return newOverlay(resolved, mode)
		}
		return newCustom(resolved, mode), nil
	})
	return nil
}

// CustomSpecFor retorna a configuração efetiva de uma instância declarada pelo usuário.
func CustomSpecFor(name string) (CustomSpec, bool) {
	customMu.RLock()
	defer customMu.RUnlock()
	s, ok := customSpecs[name]
	return s, ok
}

func resolveSpec(s CustomSpec, seen map[string]bool) (CustomSpec, error) {
	if s.Name == "" {
		return s, fmt.Errorf("harness custom: campo name é obrigatório")
	}
	if s.Base == "" {
		return s, nil
	}
	if seen[s.Name] {
		return s, fmt.Errorf("herança circular envolvendo '%s'", s.Name)
	}
	seen[s.Name] = true
	if !Exists(s.Base) {
		customMu.RLock()
		_, ok := customSpecs[s.Base]
		customMu.RUnlock()
		if !ok {
			return s, fmt.Errorf("harness custom '%s': base '%s' não existe", s.Name, s.Base)
		}
	}
	customMu.RLock()
	base, customBase := customSpecs[s.Base]
	customMu.RUnlock()
	if customBase {
		if resolved, err := resolveSpec(base, seen); err != nil {
			return s, err
		} else {
			base = resolved
		}
	} else {
		base = CustomSpec{Name: s.Base}
	}
	if !Exists(s.Base) {
		return s, fmt.Errorf("harness custom '%s': base '%s' não existe", s.Name, s.Base)
	}
	base.Name = s.Name
	base.Base = s.Base
	if s.DisplayName != "" {
		base.DisplayName = s.DisplayName
	}
	if s.Command != "" {
		base.Command = s.Command
	}
	if s.Args != nil {
		base.Args = s.Args
	}
	if s.Env != nil {
		if base.Env == nil {
			base.Env = map[string]string{}
		}
		for k, v := range s.Env {
			base.Env[k] = expandHome(v)
		}
	}
	if s.Model != "" {
		base.Model = s.Model
	}
	if s.Effort != "" {
		base.Effort = s.Effort
	}
	if s.Prompt != "" {
		base.Prompt = s.Prompt
	}
	if s.FinishRegex != "" {
		base.FinishRegex = s.FinishRegex
	}
	if s.QuotaRegex != "" {
		base.QuotaRegex = s.QuotaRegex
	}
	if s.Reserva != nil {
		base.Reserva = append([]string(nil), s.Reserva...)
	}
	return base, nil
}

func builtinSpec(name string) (CustomSpec, bool) {
	return CustomSpec{}, false
}

// LoadCustom carrega o projeto e ~/.config/openheinerss (projeto vence usuário).
func LoadCustom(cwd string) error {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.config/openheinerss/harnesses")
	}
	paths = append(paths, cwd+"/.openheinerss/harnesses")
	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("ler harnesses em %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".json")) {
				continue
			}
			if err := LoadCustomFile(dir + "/" + e.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func LoadCustomFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var s CustomSpec
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return fmt.Errorf("harness %s inválido: %w", path, err)
	}
	if s.Name == "" {
		s.Name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(filepathBase(path), ".yaml"), ".yml"), ".json")
	}
	return RegisterCustom(s)
}
func filepathBase(p string) string {
	i := strings.LastIndexAny(p, "/\\")
	if i >= 0 {
		return p[i+1:]
	}
	return p
}

type customHarness struct {
	mu        sync.Mutex
	spec      CustomSpec
	mode      Mode
	cfg       SessionConfig
	ctx       context.Context
	cancel    context.CancelFunc
	events    chan Event
	cmd       *exec.Cmd
	stopped   bool
	completed bool
}

func newCustom(s CustomSpec, mode Mode) *customHarness {
	return &customHarness{spec: s, mode: mode, events: make(chan Event, 200)}
}

// overlayHarness aplica a configuração de uma instância ao harness base real.
// Assim `base: claude-code` continua usando o SDK/CLI oficial do adaptador.
type overlayHarness struct {
	base harnessAlias
	spec CustomSpec
}

type harnessAlias interface {
	Name() string
	Mode() Mode
	ValidatePrerequisites(context.Context) PrerequisiteResult
	Start(context.Context, SessionConfig) error
	SendPrompt(context.Context, string, []protocol.Attachment) error
	RespondPermission(context.Context, string, bool, string) error
	Events() <-chan Event
	Stop() error
}

func newOverlay(s CustomSpec, mode Mode) (Harness, error) {
	b, err := Create(s.Base, mode)
	if err != nil {
		return nil, err
	}
	return &overlayHarness{base: b, spec: s}, nil
}
func (o *overlayHarness) Name() string { return o.spec.Name }
func (o *overlayHarness) Mode() Mode   { return o.base.Mode() }
func (o *overlayHarness) ValidatePrerequisites(ctx context.Context) PrerequisiteResult {
	return o.base.ValidatePrerequisites(ctx)
}
func (o *overlayHarness) Start(ctx context.Context, cfg SessionConfig) error {
	if cfg.Model == "" {
		cfg.Model = o.spec.Model
	}
	if o.spec.Env != nil {
		cfg.Env = mergeEnv(cfg.Env, o.spec.Env)
	}
	return o.base.Start(ctx, cfg)
}
func (o *overlayHarness) SendPrompt(ctx context.Context, text string, a []protocol.Attachment) error {
	return o.base.SendPrompt(ctx, text, a)
}
func (o *overlayHarness) RespondPermission(ctx context.Context, id string, allow bool, msg string) error {
	return o.base.RespondPermission(ctx, id, allow, msg)
}
func (o *overlayHarness) Events() <-chan Event { return o.base.Events() }
func (o *overlayHarness) Stop() error          { return o.base.Stop() }
func mergeEnv(first, second map[string]string) map[string]string {
	out := make(map[string]string, len(first)+len(second))
	for k, v := range first {
		out[k] = v
	}
	for k, v := range second {
		out[k] = v
	}
	return out
}
func (c *customHarness) Name() string         { return c.spec.Name }
func (c *customHarness) Mode() Mode           { return c.mode }
func (c *customHarness) Events() <-chan Event { return c.events }
func (c *customHarness) ValidatePrerequisites(context.Context) PrerequisiteResult {
	if _, err := exec.LookPath(c.spec.Command); err != nil {
		return PrerequisiteResult{MissingItems: []string{c.spec.Command}, SuggestedFix: "Instale o comando do harness custom e verifique o PATH."}
	}
	return PrerequisiteResult{Satisfied: true}
}
func (c *customHarness) Start(ctx context.Context, cfg SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false
	c.completed = false
	return nil
}
func (c *customHarness) SendPrompt(ctx context.Context, text string, _ []protocol.Attachment) error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return fmt.Errorf("harness custom '%s' parado", c.Name())
	}
	base := c.ctx
	if ctx != nil {
		base = ctx
	}
	spec, cfg := c.spec, c.cfg
	c.mu.Unlock()
	args := make([]string, len(spec.Args))
	hasPrompt := false
	for i, a := range spec.Args {
		if strings.Contains(a, "{{prompt}}") {
			hasPrompt = true
			args[i] = strings.ReplaceAll(a, "{{prompt}}", text)
		} else {
			args[i] = a
		}
	}
	cmd := exec.CommandContext(base, spec.Command, args...)
	cmd.Dir = cfg.CWD
	cmd.Env = mergedCustomEnv(spec.Env, cfg.Env)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("iniciar harness custom: %w", err)
	}
	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()
	if spec.Prompt == "stdin" && !hasPrompt {
		go func() { _, _ = io.WriteString(in, text+"\n"); _ = in.Close() }()
	} else {
		_ = in.Close()
	}
	go c.read(out)
	return nil
}
func (c *customHarness) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		c.parseLine(sc.Text())
	}
	c.complete("process_exit")
}
func (c *customHarness) parseLine(line string) {
	if c.spec.QuotaRegex != "" {
		if regexp.MustCompile(c.spec.QuotaRegex).MatchString(line) {
			c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: "limite de cota detectado"}})
			return
		}
	}
	if c.spec.FinishRegex != "" && regexp.MustCompile(c.spec.FinishRegex).MatchString(line) {
		c.complete("finish_regex")
		return
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(line), &m) != nil {
		c.emit(Event{Type: EventText, Payload: protocol.TextParams{SessionID: c.cfg.SessionID, Delta: line + "\n"}})
		return
	}
	typ, _ := m["type"].(string)
	if typ == "" {
		typ, _ = m["event"].(string)
	}
	payload := m["payload"]
	if payload == nil {
		payload = m
	}
	sid := c.cfg.SessionID
	switch typ {
	case "text", "agent.text":
		d := str(m, "text")
		if d == "" {
			d = str(m, "delta")
		}
		c.emit(Event{Type: EventText, Payload: protocol.TextParams{SessionID: sid, Delta: d}})
	case "tool", "tool_call", "agent.tool_call":
		c.emit(Event{Type: EventToolCall, Payload: protocol.ToolCallParams{SessionID: sid, CallID: str(m, "callId"), Tool: str(m, "tool"), Input: m["input"]}})
	case "error", "agent.error":
		c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: sid, Message: str(m, "message")}})
	case "usage", "agent.usage":
		c.emit(Event{Type: EventUsage, Payload: protocol.UsageParams{SessionID: sid, InputTokens: int64(num(m, "inputTokens")), OutputTokens: int64(num(m, "outputTokens")), TotalTokens: int64(num(m, "totalTokens"))}})
	case "end", "done", "complete", "agent.complete":
		reason := str(m, "reason")
		if reason == "" {
			reason = "completed"
		}
		c.complete(reason)
	default:
		if payload != nil {
			b, _ := json.Marshal(payload)
			c.emit(Event{Type: EventText, Payload: protocol.TextParams{SessionID: sid, Delta: string(b)}})
		}
	}
}
func str(m map[string]interface{}, k string) string                                    { v, _ := m[k].(string); return v }
func num(m map[string]interface{}, k string) float64                                   { v, _ := m[k].(float64); return v }
func (c *customHarness) RespondPermission(context.Context, string, bool, string) error { return nil }
func (c *customHarness) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return nil
	}
	c.stopped = true
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}
func (c *customHarness) emit(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		select {
		case c.events <- e:
		default:
		}
	}
}
func (c *customHarness) complete(reason string) {
	c.mu.Lock()
	if c.completed || c.stopped {
		c.mu.Unlock()
		return
	}
	c.completed = true
	c.mu.Unlock()
	c.emit(Event{Type: EventComplete, Payload: protocol.CompleteParams{SessionID: c.cfg.SessionID, Reason: reason}})
}
func mergedCustomEnv(base, extra map[string]string) []string {
	env := os.Environ()
	for k, v := range base {
		env = append(env, k+"="+v)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// expandHome troca o "~/" inicial pela pasta do usuário (o shell não faz isso por nós).
func expandHome(v string) string {
	if v == "~" || strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + v[1:]
		}
	}
	return v
}
