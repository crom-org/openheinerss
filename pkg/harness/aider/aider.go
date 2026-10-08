package aider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/process"
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
	effort  string
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
	a.effort = harness.OpcaoTexto(cfg.Options, "reasoning_effort", "effort")

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

func (a *AiderHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("processo do aider não está ativo")
	}

	sessID := a.cfg.SessionID
	if name, rest, ok := harness.SlashCommand(text); ok {
		switch strings.ToLower(name) {
		case "model":
			if rest == "" {
				return harness.NoEquivalent("aider", name, "informe o modelo, ex.: /model gpt-4o")
			}
			a.cfg.Model = rest
			a.emitLocked(sessID, "modelo das próximas chamadas: "+rest)
			return nil
		case "reasoning-effort", "effort":
			a.effort = rest
			a.emitLocked(sessID, "esforço (--reasoning-effort) das próximas chamadas: "+rest)
			return nil
		}
		// Os demais /comandos o próprio aider interpreta dentro de --message: vão literais.
	}
	files, extraFiles, cleanup, err := a.anexos(attachments)
	if err != nil {
		return err
	}
	cfg, effort := a.cfg, a.effort
	go func() {
		defer cleanup()
		a.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Aider analisando repositório e histórico...",
			},
		})

		if msg := missingModelMessage(a.cfg, a.env); msg != "" {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: msg, SuggestedFix: modelFix}})
			a.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "process_error"}})
			return
		}

		// --yes-always responde "sim" a toda pergunta e --no-pretty/--no-stream evitam
		// controle de terminal; sem isso o aider pode ficar esperando entrada.
		// O git fica habilitado para que uma missão possa criar o commit pedido.
		args := buildArgs(cfg, effort, files, extraFiles, text)

		runCtx, stopRun := context.WithCancel(a.ctx)
		defer stopRun()
		cmd := exec.CommandContext(runCtx, "aider", args...)
		process.Configure(cmd)
		cmd.Cancel = func() error {
			if err := process.Kill(cmd); err != nil {
				return cmd.Process.Kill()
			}
			return nil
		}
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
		stderrTail := &tailBuffer{}
		stderrRaw := harness.NewLineWriter(func(line string) {
			a.emit(harness.RawEvent(sessID, "aider", "stderr", line))
		})
		stderrDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.MultiWriter(stderrTail, stderrRaw), stderr)
			stderrRaw.Flush()
			close(stderrDone)
		}()

		fatal := ""
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			// O aider repete chamadas com erro de provedor (402, chave inválida) para sempre.
			// Aborta no primeiro erro fatal em vez de esperar o timeout.
			if msg := fatalAiderLine(line); msg != "" && fatal == "" {
				fatal = msg
				stopRun()
				break
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

		select {
		case <-stderrDone:
		case <-time.After(2 * time.Second):
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
		if fatal != "" {
			_, fix := harness.ClassifyFailure(fatal)
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: fatal, SuggestedFix: fix}})
			a.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "process_error"}})
			return
		}
		if err != nil {
			message := err.Error()
			if tail := stderrTail.String(); tail != "" {
				message += ": " + tail
			}
			_, fix := harness.ClassifyFailure(message)
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: message, SuggestedFix: fix}})
		}

		reason := "completed"
		if err != nil {
			reason = "process_error"
		}
		a.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    reason,
			},
		})
	}()

	return nil
}

// anexos separa os arquivos pedidos nas options (files = editáveis, read_files = só leitura) e
// grava os anexos do prompt em arquivos temporários (o aider só recebe caminhos).
func (a *AiderHarness) anexos(attachments []protocol.Attachment) (edit, read []string, cleanup func(), err error) {
	extra, cleanup, err := harness.AnexosEmArquivos(attachments)
	if err != nil {
		return nil, nil, func() {}, err
	}
	edit = append(harness.OpcaoLista(a.cfg.Options, "files", "file"), extra...)
	return edit, harness.OpcaoLista(a.cfg.Options, "read_files", "read"), cleanup, nil
}

// buildArgs monta a invocação do aider: opções fixas, tipadas, harness_args (intactos, na ordem) e, por
// último, --message (inclusive "/comando ...", que o aider interpreta).
func buildArgs(cfg harness.SessionConfig, effort string, files, read []string, text string) []string {
	args := []string{"--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--no-browser"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if effort != "" {
		args = append(args, "--reasoning-effort", effort)
	}
	for _, f := range read {
		args = append(args, "--read", f)
	}
	for _, f := range files {
		args = append(args, "--file", f)
	}
	if harness.OpcaoBool(cfg.Options, "continue", "restore_chat_history") {
		args = append(args, "--restore-chat-history")
	}
	args = append(args, harness.HarnessArgs(cfg.Options)...)
	return append(args, "--message="+text)
}

const modelFix = "Informe um modelo (--model ou campo model da instância) e exporte a chave do provedor (ex.: OPENROUTER_API_KEY, ANTHROPIC_API_KEY), ou use um modelo local 'ollama/...'."

// missingModelMessage devolve uma mensagem clara quando o aider não tem como
// chamar nenhum modelo (sem --model, sem chave, sem configuração salva).
func missingModelMessage(cfg harness.SessionConfig, env []string) string {
	if strings.HasPrefix(cfg.Model, "ollama") {
		return ""
	}
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && value != "" && strings.HasSuffix(name, "_API_KEY") {
			return ""
		}
	}
	candidates := []string{filepath.Join(cfg.CWD, ".aider.conf.yml"), filepath.Join(cfg.CWD, ".env")}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".aider.conf.yml"), filepath.Join(home, ".aider", "oauth-keys.env"), filepath.Join(home, ".env"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return ""
		}
	}
	return "sem modelo configurado: o aider não tem modelo nem chave de API (defina --model e a chave do provedor)"
}

// fatalAiderLine reconhece saídas do aider/litellm que nunca vão se resolver sozinhas.
func fatalAiderLine(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "no llm model was specified"), strings.Contains(lower, "you need to specify a model"):
		return "sem modelo configurado: " + strings.TrimSpace(line)
	case strings.Contains(lower, "more credits"), strings.Contains(lower, "insufficient_quota"), strings.Contains(lower, "exceeded your current quota"), strings.Contains(lower, "code\":402"):
		return "sem cota/créditos no provedor do aider: " + strings.TrimSpace(line)
	case strings.Contains(lower, "authenticationerror"), strings.Contains(lower, "incorrect api key"), strings.Contains(lower, "invalid api key"), strings.Contains(lower, "invalid x-api-key"), strings.Contains(lower, "no api key"):
		return "sem login: chave de API inválida ou ausente no aider: " + strings.TrimSpace(line)
	case strings.Contains(lower, "upstream error"), strings.Contains(lower, "service temporarily overloaded"), strings.Contains(lower, "serviceunavailableerror"), strings.Contains(lower, "too many requests"), strings.Contains(lower, "status code: 429"), strings.Contains(lower, "status code: 5"):
		return "erro do provedor no aider: " + strings.TrimSpace(line)
	}
	return ""
}

// tailBuffer guarda só o fim do que recebe (o suficiente para explicar uma falha).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 1024 {
		t.buf = t.buf[len(t.buf)-1024:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

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
		if process.Interrupt(a.cmd) != nil {
			_ = a.cmd.Process.Signal(os.Interrupt)
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

// emitLocked devolve o aviso de um /comando traduzido pela ponte e encerra o turno
// (chamar com a.mu preso, como em SendPrompt).
func (a *AiderHarness) emitLocked(sessionID, message string) {
	for _, evt := range []harness.Event{
		{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: message + "\n"}},
		{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: "completed"}},
	} {
		select {
		case a.events <- evt:
		default:
		}
	}
}
