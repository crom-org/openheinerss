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
	"time"

	"github.com/crom-org/openheinerss/pkg/harness/process"
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
	Mode        string            `json:"mode,omitempty" yaml:"mode,omitempty"`
	Modo        string            `json:"modo,omitempty" yaml:"modo,omitempty"`
	Prompt      string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	FinishRegex string            `json:"finishRegex,omitempty" yaml:"finishRegex,omitempty"`
	QuotaRegex  string            `json:"quotaRegex,omitempty" yaml:"quotaRegex,omitempty"`
	ErrorRegex  string            `json:"errorRegex,omitempty" yaml:"errorRegex,omitempty"`
	ErroRegex   string            `json:"error_regex,omitempty" yaml:"error_regex,omitempty"`
	ErroRegexPT string            `json:"erro_regex,omitempty" yaml:"erro_regex,omitempty"`
	Reserva     []string          `json:"reserva,omitempty" yaml:"reserva,omitempty"`
	EventLog    string            `json:"eventosLog,omitempty" yaml:"eventosLog,omitempty"`
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
	if resolved.Mode == "" {
		resolved.Mode = resolved.Modo
	}
	if resolved.ErrorRegex == "" {
		resolved.ErrorRegex = resolved.ErroRegex
	}
	if resolved.ErrorRegex == "" {
		resolved.ErrorRegex = resolved.ErroRegexPT
	}
	if resolved.Command == "" && resolved.Base == "" {
		return fmt.Errorf("harness custom '%s': informe base ou command", resolved.Name)
	}
	// O "~/" não é expandido pelo shell: vale para o comando e para as variáveis de ambiente.
	resolved.Command = expandHome(resolved.Command)
	if resolved.Env != nil {
		env := make(map[string]string, len(resolved.Env))
		for k, v := range resolved.Env {
			env[k] = expandHome(v)
		}
		resolved.Env = env
	}
	if resolved.Prompt == "" {
		resolved.Prompt = "stdin"
	}
	if resolved.Prompt != "stdin" && resolved.Prompt != "argument" {
		return fmt.Errorf("harness custom '%s': prompt deve ser stdin ou argument", resolved.Name)
	}
	if resolved.Mode != "" && resolved.Mode != string(ModeCLI) && resolved.Mode != string(ModeSDK) {
		return fmt.Errorf("harness custom '%s': modo deve ser cli ou sdk", resolved.Name)
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
	if resolved.ErrorRegex != "" {
		if _, err := regexp.Compile(resolved.ErrorRegex); err != nil {
			return fmt.Errorf("harness custom '%s': errorRegex inválido: %w", resolved.Name, err)
		}
	}
	customMu.Lock()
	customSpecs[resolved.Name] = resolved
	customMu.Unlock()
	meta := protocol.HarnessCatalogItem{ID: resolved.Name, DisplayName: resolved.DisplayName, SupportedModes: []string{"cli"}, SupportedProtocols: []string{"ndjson"}, Origin: "custom"}
	if meta.DisplayName == "" {
		meta.DisplayName = resolved.Name
	}
	if resolved.Base != "" && resolved.Command == "" {
		defaultRegistry.mu.RLock()
		meta.MCP = defaultRegistry.metadata[resolved.Base].MCP
		defaultRegistry.mu.RUnlock()
	} else {
		meta.MCP = "sem suporte: harness custom por comando não recebe mcp.json (passe a config dele em args/env)"
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
	if s.Mode != "" {
		base.Mode = s.Mode
	}
	if s.Modo != "" {
		base.Mode = s.Modo
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
	if s.ErrorRegex != "" {
		base.ErrorRegex = s.ErrorRegex
	}
	if s.ErroRegex != "" {
		base.ErrorRegex = s.ErroRegex
	}
	if s.ErroRegexPT != "" {
		base.ErrorRegex = s.ErroRegexPT
	}
	if s.Reserva != nil {
		base.Reserva = append([]string(nil), s.Reserva...)
	}
	if s.EventLog != "" {
		base.EventLog = s.EventLog
	}
	return base, nil
}

// LoadCustom carrega o projeto e ~/.config/openheinerss (projeto vence usuário).
func LoadCustom(cwd string) error {
	return LoadCustomDir(cwd + "/.openheinerss/harnesses")
}

// LoadCustomDir carrega ~/.config/openheinerss/harnesses e depois dir (dir vence usuário).
func LoadCustomDir(dir string) error {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.config/openheinerss/harnesses")
	}
	paths = append(paths, dir)
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
	finishRe  *regexp.Regexp
	quotaRe   *regexp.Regexp
	errorRe   *regexp.Regexp
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
	// usedTool marca que o turno chamou ferramentas (resultado útil); o stderr de um turno sem isso é erro do motor.
	usedTool bool
	// Fim do turno: o stderr só é conferido quando o processo saiu (ou, se ele segue vivo, após uma espera curta).
	exited       chan struct{}
	stderr       *tailBuffer
	pending      string
	stderrJudged bool
	finMu        sync.Mutex
}

func newCustom(s CustomSpec, mode Mode) *customHarness {
	c := &customHarness{spec: s, mode: mode, events: make(chan Event, 200)}
	// RegisterCustom já validou as expressões.
	if s.FinishRegex != "" {
		c.finishRe, _ = regexp.Compile(s.FinishRegex)
	}
	if s.QuotaRegex != "" {
		c.quotaRe, _ = regexp.Compile(s.QuotaRegex)
	}
	if s.ErrorRegex != "" {
		c.errorRe, _ = regexp.Compile(s.ErrorRegex)
	}
	return c
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
	// A instância pode trazer variáveis que mudam o resultado (caminho do SDK, por exemplo).
	if v, ok := o.base.(interface {
		ValidatePrerequisitesEnv(context.Context, map[string]string) PrerequisiteResult
	}); ok {
		return v.ValidatePrerequisitesEnv(ctx, o.spec.Env)
	}
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

// ResumeID repassa o ID de retomada nativa do motor base, se ele tiver um.
func (o *overlayHarness) ResumeID() string {
	if r, ok := o.base.(interface{ ResumeID() string }); ok {
		return r.ResumeID()
	}
	return ""
}
func (o *overlayHarness) Stop() error { return o.base.Stop() }
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
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("harness custom '%s': prompt vazio recebido", c.Name())
	}
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
	// harness_args vão depois dos args do spec (que podem conter o {{prompt}}); sem {{prompt}} o prompt
	// segue por stdin, então ficam antes dele. Ver docs/ponte/custom-mock.md.
	args = append(args, HarnessArgs(cfg.Options)...)
	cmd := exec.CommandContext(base, spec.Command, args...)
	process.Configure(cmd)
	cmd.Dir = cfg.CWD
	cmd.Env = mergedCustomEnv(spec.Env, cfg.Env, cfg.CWD)
	stderr := &tailBuffer{max: 4096}
	stderrRaw := NewLineWriter(func(line string) {
		c.emit(RawEvent(cfg.SessionID, c.spec.Name, "stderr", line))
	})
	cmd.Stderr = io.MultiWriter(stderr, stderrRaw)
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
	c.usedTool, c.pending, c.stderrJudged = false, "", false
	c.exited, c.stderr = make(chan struct{}), stderr
	c.mu.Unlock()
	if spec.Prompt == "stdin" && !hasPrompt {
		go func() { _, _ = io.WriteString(in, text+"\n"); _ = in.Close() }()
	} else {
		_ = in.Close()
	}
	go func() { c.read(cmd, out, stderr); stderrRaw.Flush() }()
	return nil
}

// read consome o stdout, colhe o processo (sem deixar zumbi) e conta como falha uma saída com erro sem o evento de fim.
func (c *customHarness) read(cmd *exec.Cmd, r io.Reader, stderr *tailBuffer) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		c.parseLine(sc.Text())
	}
	_, _ = io.Copy(io.Discard, r)
	err := cmd.Wait()
	c.mu.Lock()
	parado, pending, exited := c.stopped, c.pending, c.exited
	c.mu.Unlock()
	defer close(exited)
	c.finMu.Lock()
	defer c.finMu.Unlock()
	if err == nil && !parado {
		c.erroDeStderr()
	}
	if err != nil && !parado {
		tail := strings.TrimSpace(stderr.String())
		if c.quotaRe != nil && c.quotaRe.MatchString(tail) {
			c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: "limite de cota detectado: " + tail}})
		} else if c.errorRe != nil && c.errorRe.MatchString(tail) {
			c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: "erro do provedor detectado: " + tail}})
		} else {
			c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: fmt.Sprintf("harness custom '%s' terminou com erro: %v %s", c.spec.Name, err, tail)}})
		}
		if pending != "" {
			c.complete("process_error")
			return
		}
		c.complete("process_error")
		return
	}
	if pending != "" {
		c.complete(pending)
		return
	}
	c.complete("process_exit")
}

// erroDeStderr: saída limpa (ou fim já declarado) sem ferramenta usada = turno sem resultado útil; o que o
// motor escreveu no stderr (cota, sobrecarga) é o motivo e vira evento de erro. Com ferramentas é só aviso.
// Chamar com finMu seguro.
func (c *customHarness) erroDeStderr() {
	c.mu.Lock()
	if c.stderrJudged || c.usedTool || c.stopped || c.stderr == nil {
		c.mu.Unlock()
		return
	}
	c.stderrJudged = true
	tail := strings.TrimSpace(c.stderr.String())
	c.mu.Unlock()
	if tail != "" {
		c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: tail}})
	}
}

// fimDeclarado trata o "end"/finishRegex: espera o processo sair (até 300 ms) para o stderr chegar antes do fim.
func (c *customHarness) fimDeclarado(reason string) {
	c.mu.Lock()
	if c.pending == "" {
		c.pending = reason
	}
	exited := c.exited
	c.mu.Unlock()
	go func() {
		if exited != nil {
			select {
			case <-exited:
			case <-time.After(300 * time.Millisecond):
			}
		}
		c.finMu.Lock()
		defer c.finMu.Unlock()
		c.erroDeStderr()
		c.complete(reason)
	}()
}
func (c *customHarness) parseLine(line string) {
	if c.quotaRe != nil && c.quotaRe.MatchString(line) {
		c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: "limite de cota detectado: " + line}})
		return
	}
	if c.errorRe != nil && c.errorRe.MatchString(line) {
		c.emit(Event{Type: EventError, Payload: protocol.ErrorParams{SessionID: c.cfg.SessionID, Message: "erro do provedor detectado: " + line}})
		return
	}
	if c.finishRe != nil && c.finishRe.MatchString(line) {
		c.fimDeclarado("finish_regex")
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
		c.mu.Lock()
		c.usedTool = true
		c.mu.Unlock()
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
		c.fimDeclarado(reason)
	default:
		c.emit(RawEvent(sid, c.spec.Name, "stdout", line))
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
	if c.cmd != nil && c.cmd.Process != nil {
		_ = process.Interrupt(c.cmd)
	}
	return nil
}

// emit entrega o evento; bloqueia se o consumidor está lento (não perde o fim) e desiste quando o harness para.
func (c *customHarness) emit(e Event) {
	c.mu.Lock()
	stopped, ctx := c.stopped, c.ctx
	c.mu.Unlock()
	if stopped || ctx == nil {
		return
	}
	select {
	case c.events <- e:
	case <-ctx.Done():
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
func mergedCustomEnv(base, extra map[string]string, cwd ...string) []string {
	env := os.Environ()
	for k, v := range base {
		env = append(env, k+"="+v)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	if len(cwd) > 0 && cwd[0] != "" {
		env = SetEnv(env, "PWD", cwd[0])
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

// tailBuffer guarda só o fim do que foi escrito (stderr do harness, para mensagens de erro).
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
