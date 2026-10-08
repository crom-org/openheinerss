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

		if msg := missingModelMessage(a.cfg, a.env); msg != "" {
			a.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: msg, SuggestedFix: modelFix}})
			a.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "no_model"}})
			return
		}

		// --yes-always responde "sim" a toda pergunta e --no-pretty/--no-stream evitam
		// controle de terminal; sem isso o aider pode ficar esperando entrada.
		args := []string{"--no-git", "--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--message", text}
		if a.cfg.Model != "" {
			args = append(args, "--model", a.cfg.Model)
		}

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
		go func() { _, _ = io.Copy(stderrTail, stderr) }()

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
			a.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "provider_error"}})
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
