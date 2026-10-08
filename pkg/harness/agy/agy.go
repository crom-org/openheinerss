package agy

import (
	"bufio"
	"context"
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
	harness.Register("agy", protocol.HarnessCatalogItem{
		ID:                 "agy",
		DisplayName:        "Google Antigravity (AGY Agent Suite)",
		SupportedModes:     []string{"cli", "sdk"},
		SupportedProtocols: []string{"antigravity", "gemini"},
		MCP:                "sem equivalente por execução: o agy só tem \"agy mcp add\", que edita a config do usuário",
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
	a.cfg.Options = make(map[string]interface{}, len(cfg.Options))
	for k, v := range cfg.Options {
		a.cfg.Options[k] = v
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.stopped = false

	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	if cfg.CWD != "" {
		env = harness.SetEnv(env, "PWD", cfg.CWD)
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
	if name, rest, ok := harness.SlashCommand(text); ok {
		// Em modo print o agy expande /comandos e skills sozinho (--disable-slash-commands desliga): vão literais.
		// Só os que mudam a invocação (e não existem no print mode) são traduzidos.
		if msg, handled, err := a.translate(name, rest); err != nil {
			return err
		} else if handled {
			for _, evt := range []harness.Event{
				{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: msg + "\n"}},
				{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "completed"}},
			} {
				select {
				case a.events <- evt:
				default:
				}
			}
			return nil
		}
	}
	cfg := a.cfg
	go func() {
		a.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Antigravity analisando com skills e regras do workspace...",
			},
		})

		// O modo headless precisa autorizar ferramentas e emitir eventos para que
		// o agente consiga editar/commitar na worktree sem pedir interação.
		args := buildArgs(cfg, text)

		cmd := exec.CommandContext(a.ctx, "agy", args...)
		process.Configure(cmd)
		cmd.Dir = a.cfg.CWD
		cmd.Env = a.env

		pr, pw := io.Pipe()
		er, ew := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = ew

		if err := cmd.Start(); err != nil {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: "falha ao iniciar agy: " + err.Error()}})
			a.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "process_error"}})
			return
		}
		a.mu.Lock()
		a.cmd = cmd
		a.mu.Unlock()

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
			_ = pw.Close()
			_ = ew.Close()
		}()

		// O stderr do agy é um canal de erro; o stdout é a resposta do agente e só vale por eventos de erro.
		sawOutput, toolFailure := false, ""
		var failMu sync.Mutex
		setFailure := func(f string) {
			failMu.Lock()
			if toolFailure == "" {
				toolFailure = f
			}
			failMu.Unlock()
		}
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			es := bufio.NewScanner(er)
			es.Buffer(make([]byte, 64*1024), 16*1024*1024)
			for es.Scan() {
				line := es.Text()
				if line == "" {
					continue
				}
				failMu.Lock()
				sawOutput = true
				failMu.Unlock()
				if f := agyFailureLine(line); f != "" {
					setFailure(f)
				}
				a.emit(harness.RawEvent(sessID, "agy", "stderr", line))
				a.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: line + "\n"}})
			}
		}()

		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				failMu.Lock()
				sawOutput = true
				failMu.Unlock()
			}
			if events := harness.ParseJSONEvent(line, sessID); len(events) > 0 {
				for _, event := range events {
					if p, ok := event.Payload.(protocol.ErrorParams); ok && event.Type == harness.EventError {
						if f := agyFailureLine(p.Message); f != "" {
							setFailure(f)
						}
					}
					a.emit(event)
				}
				continue
			}
			if f := agyPlainFailure(line); f != "" {
				setFailure(f)
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
		<-stderrDone
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
		if toolFailure == "" && !sawOutput {
			toolFailure = "agy terminou sem produzir saída; nenhuma ferramenta foi executada"
		}
		if toolFailure != "" {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: toolFailure}})
		}

		a.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    map[bool]string{true: "process_error", false: "completed"}[waitErr != nil || toolFailure != ""],
			},
		})
	}()

	return nil
}

// buildArgs monta a invocação não-interativa documentada pelo agy: opções fixas, tipadas (cfg.Options),
// harness_args (intactos, na ordem) e, por último, o prompt. O prompt é
// um valor da opção -p; separá-lo com "--" faz o agy interpretar prompts que
// começam por hífen como sintaxe inválida.
func buildArgs(cfg harness.SessionConfig, text string) []string {
	args := []string{"--dangerously-skip-permissions", "--output-format", "stream-json"}
	if cfg.Model != "" {
		args = append([]string{"--model", cfg.Model}, args...)
	}
	o := cfg.Options
	if v := harness.OpcaoTexto(o, "effort"); v != "" {
		args = append(args, "--effort", v)
	}
	if v := harness.OpcaoTexto(o, "agent"); v != "" {
		args = append(args, "--agent", v)
	}
	if v := harness.OpcaoTexto(o, "mode"); v != "" {
		args = append(args, "--mode", v)
	}
	if v := harness.OpcaoTexto(o, "conversation"); v != "" {
		args = append(args, "--conversation", v)
	} else if harness.OpcaoBool(o, "continue") {
		args = append(args, "--continue")
	}
	if v := harness.OpcaoTexto(o, "project"); v != "" {
		args = append(args, "--project", v)
	}
	for _, dir := range harness.OpcaoLista(o, "add_dirs", "add_dir") {
		args = append(args, "--add-dir", dir)
	}
	if harness.OpcaoBool(o, "sandbox") {
		args = append(args, "--sandbox")
	}
	args = append(args, harness.HarnessArgs(o)...)
	return append(args, "-p="+text)
}

// translate converte em ajustes das próximas chamadas os /comandos que o print mode do agy não tem.
// Chamar com a.mu preso.
func (a *AGYHarness) translate(name, rest string) (msg string, handled bool, err error) {
	set := func(key, label string) (string, bool, error) {
		if rest == "" {
			return "", false, harness.NoEquivalent("agy", name, "informe o valor, ex.: /"+name+" <valor>")
		}
		a.cfg.Options[key] = rest
		return label + " das próximas chamadas: " + rest, true, nil
	}
	switch strings.ToLower(name) {
	case "model":
		if rest == "" {
			return "", false, harness.NoEquivalent("agy", name, "informe o modelo, ex.: /model gemini-3-pro")
		}
		a.cfg.Model = rest
		return "modelo das próximas chamadas: " + rest, true, nil
	case "effort":
		return set("effort", "esforço")
	case "agent":
		return set("agent", "agente")
	case "mode":
		return set("mode", "modo de execução")
	case "new", "clear":
		delete(a.cfg.Options, "conversation")
		delete(a.cfg.Options, "continue")
		return "conversa esquecida: a próxima chamada começa uma conversa nova", true, nil
	case "exit", "quit":
		return "", false, harness.NoEquivalent("agy", name, "o modo print termina sozinho a cada turno; use parar a sessão")
	}
	return "", false, nil
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

// agyFailureLine examina uma linha de ERRO do agy (evento de erro ou stderr): é onde o agy avisa que
// não conseguiu usar ferramentas. Nunca deve receber o texto livre do agente.
func agyFailureLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "no output produced") || strings.Contains(lower, "tool required") || (strings.Contains(lower, "permission") && strings.Contains(lower, "denied")) {
		return strings.TrimSpace(line)
	}
	return ""
}

// agyPlainFailure trata uma linha de stdout que não é JSON: só conta quando a linha COMEÇA com a frase do
// agy (opcionalmente após "Error:"); "Corrigi permission denied no README" é texto do agente.
func agyPlainFailure(line string) string {
	t := strings.ToLower(strings.TrimSpace(line))
	for _, p := range []string{"error:", "erro:", "[error]"} {
		t = strings.TrimSpace(strings.TrimPrefix(t, p))
	}
	for _, p := range []string{"no output produced", "tool required", "permission denied"} {
		if strings.HasPrefix(t, p) {
			return strings.TrimSpace(line)
		}
	}
	return ""
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
		if process.Interrupt(a.cmd) != nil {
			_ = a.cmd.Process.Signal(os.Interrupt)
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
