package claudecode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/process"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func init() {
	register := func(name string) {
		harness.Register(name, protocol.HarnessCatalogItem{
			ID:                 "claude-code",
			DisplayName:        "Claude Code (Anthropic & Provedores Abertos)",
			SupportedModes:     []string{"sdk", "cli"},
			SupportedProtocols: []string{"anthropic"},
			MCP:                "por execução: --mcp-config <arquivo temporário>",
			DefaultProviders: []protocol.ProviderInfo{
				{
					ID:       "claude-native",
					Name:     "Anthropic Oficial (Assinatura)",
					Endpoint: "https://api.anthropic.com",
					// Modelos são descobertos/configurados pelo CLI; não congelar uma lista.
					RequiresKey: true,
				},
				{
					ID:          "openrouter",
					Name:        "OpenRouter AI",
					Endpoint:    "https://openrouter.ai/api",
					RequiresKey: true,
				},
				{
					ID:          "opencode-zen",
					Name:        "OpenCode Zen (Modelos Gratuitos)",
					Endpoint:    "https://opencode.ai/zen",
					RequiresKey: false,
				},
			},
		}, func(mode harness.Mode) (harness.Harness, error) {
			if mode == "" || mode == harness.ModeMock {
				mode = harness.ModeCLI
			}
			return NewClaudeCodeHarness(mode), nil
		})
	}
	register("claude-code")
}

// ClaudeCodeHarness implementa o conector para o Claude Code nos modos SDK e CLI
type ClaudeCodeHarness struct {
	mu     sync.Mutex
	mode   harness.Mode
	cfg    harness.SessionConfig
	env    []string
	events chan harness.Event
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	// vivoStdin é o stdin do turno em curso quando as mensagens vivas estão ligadas; vivasPend guarda os
	// uuids enviados que o claude ainda não confirmou (replay) como consumidos.
	vivoStdin io.WriteCloser
	vivasPend map[string]bool
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   bool
	resumeID  string
	// stderrTail guarda o fim do stderr do processo atual para explicar falhas.
	stderrTail string
}

// NewClaudeCodeHarness instancia o adaptador Claude Code
func NewClaudeCodeHarness(mode harness.Mode) *ClaudeCodeHarness {
	return &ClaudeCodeHarness{
		mode:   mode,
		events: make(chan harness.Event, 200),
	}
}

func (c *ClaudeCodeHarness) Name() string {
	return "claude-code"
}

func (c *ClaudeCodeHarness) Mode() harness.Mode {
	return c.mode
}

func (c *ClaudeCodeHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	return c.ValidatePrerequisitesEnv(ctx, nil)
}

// ValidatePrerequisitesEnv valida com o env de uma instância (por exemplo, OPENHEINERSS_CLAUDE_SDK_PATH
// declarado no arquivo da instância), que o processo ainda não recebeu antes do Start.
func (c *ClaudeCodeHarness) ValidatePrerequisitesEnv(ctx context.Context, instanceEnv map[string]string) harness.PrerequisiteResult {
	if c.mode == harness.ModeSDK {
		if _, err := exec.LookPath("node"); err != nil {
			return harness.PrerequisiteResult{
				Satisfied:    false,
				MissingItems: []string{"node"},
				SuggestedFix: "Node.js 18+ é necessário para o modo SDK. Instale via nvm ('nvm install 20') ou use o modo CLI.",
			}
		}
		check := exec.CommandContext(ctx, "node", "-e", "const path = require('path'); for (const p of [ '@anthropic-ai/claude-agent-sdk', process.env.OPENHEINERSS_CLAUDE_SDK_PATH || '', path.join(process.env.HOME || '', '.openheinerss/shims/node_modules/@anthropic-ai/claude-agent-sdk') ]) { if (!p) continue; try { require.resolve(p); process.exit(0) } catch (_) {} } process.exit(1)")
		check.Env = os.Environ()
		for k, v := range instanceEnv {
			check.Env = append(check.Env, k+"="+v)
		}
		if err := check.Run(); err != nil {
			return harness.PrerequisiteResult{Satisfied: false, MissingItems: []string{"@anthropic-ai/claude-agent-sdk"}, SuggestedFix: "Instale a versão do SDK compatível com o Claude Code ou use o modo CLI; o worker não deve esperar até timeout."}
		}
		return harness.PrerequisiteResult{Satisfied: true}
	}

	// Modo CLI
	if _, err := exec.LookPath("claude"); err != nil {
		return harness.PrerequisiteResult{
			Satisfied:    false,
			MissingItems: []string{"claude"},
			SuggestedFix: "Instale o Claude Code CLI via 'npm install -g @anthropic-ai/claude-code'.",
		}
	}
	return harness.PrerequisiteResult{Satisfied: true}
}

func (c *ClaudeCodeHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg = cfg
	// O ID de retomada é o session_id real do Claude, informado pelo primeiro stream
	// (ou recebido via opções ao retomar uma sessão).
	c.resumeID = optionString(cfg.Options, "claude_session_id", "resume_session", "session_id")
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.stopped = false

	env := os.Environ()
	// A instância principal é sempre a conta de ~/.claude. Não deixe o shell
	// que iniciou o servidor selecionar silenciosamente outra conta.
	if _, custom := cfg.Env["CLAUDE_CONFIG_DIR"]; !custom {
		home, _ := os.UserHomeDir()
		env = harness.SetEnv(env, "CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	}
	env = append(env,
		"DISABLE_AUTOUPDATER=1",
		"API_TIMEOUT_MS=600000",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)
	// O cache de prompt é da Anthropic; provedores de terceiros costumam rejeitá-lo.
	if !directAnthropic(cfg) {
		env = append(env, "DISABLE_PROMPT_CACHING=1")
	}

	globalDir, _ := config.GetGlobalDir()
	if cfg.Provider != "" && cfg.Provider != "default" {
		profileDir := filepath.Join(globalDir, "profiles", fmt.Sprintf("claude-%s", cfg.Provider))
		_ = config.PastaPrivada(profileDir) // CLAUDE_CONFIG_DIR guarda o login
		env = append(env, fmt.Sprintf("CLAUDE_CONFIG_DIR=%s", profileDir))
	}

	if cfg.Model != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_MODEL=%s", cfg.Model))
	}
	for k, v := range cfg.Env {
		if k == "CLAUDE_CONFIG_DIR" && harness.CanonicalName(cfg.Harness) == "claude-code" {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	if cfg.CWD != "" {
		env = harness.SetEnv(env, "PWD", cfg.CWD)
	}
	c.env = env

	// Servidores MCP do openheinerss: arquivo temporário por execução, via --mcp-config.
	servs, err := harness.ServidoresMCP(cfg)
	if err != nil {
		return fmt.Errorf("MCP: %w", err)
	}
	if len(servs) > 0 {
		data, err := mcp.ParaClaude(servs)
		if err != nil {
			return fmt.Errorf("MCP: %w", err)
		}
		path, err := harness.ArquivoTemporario(c.ctx, "mcp.json", data)
		if err != nil {
			return fmt.Errorf("MCP: %w", err)
		}
		c.cfg.Options = harness.WithOption(cfg.Options, "mcp_config", append(harness.OptionStrings(cfg.Options, "mcp_config"), path))
	}

	if c.mode == harness.ModeSDK {
		cmd := exec.CommandContext(c.ctx, "node", "-e", NodeWorkerScript)
		process.Configure(cmd)
		cmd.Dir = cfg.CWD
		cmd.Env = env

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdin pipe: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stdout pipe: %w", err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("falha ao criar stderr pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("falha ao iniciar processo do Claude Code: %w", err)
		}

		c.cmd = cmd
		c.stdin = stdin

		// Goroutines de leitura e streaming
		var leitores sync.WaitGroup
		leitores.Add(2)
		go func() { defer leitores.Done(); c.readEvents(stdout) }()
		go func() { defer leitores.Done(); c.readStderr(stderr) }()
		// Colhe o worker depois que os dois pipes acabam, para não deixar processo zumbi.
		go func() { leitores.Wait(); _ = cmd.Wait() }()

		// Se for modo SDK, envia handshake de inicialização
		initPayload := map[string]interface{}{
			"method": "init",
			"params": map[string]interface{}{
				"cwd":            cfg.CWD,
				"env":            cfg.Env,
				"model":          cfg.Model,
				"permissionMode": cfg.PermissionMode,
				"resume":         optionString(cfg.Options, "claude_session_id", "resume_session"),
			},
		}
		// O worker repassa ao SDK o que o SDK sabe receber (extraArgs cobre as flags nativas).
		initParams := initPayload["params"].(map[string]interface{})
		for k, v := range sdkExtras(c.cfg) {
			initParams[k] = v
		}
		data, _ := json.Marshal(initPayload)
		_, _ = fmt.Fprintf(c.stdin, "%s\n", data)
	}

	return nil
}

// directAnthropic informa se a sessão fala direto com a API da Anthropic
// (sem ANTHROPIC_BASE_URL apontando para outro host e sem provedor de terceiros).
func directAnthropic(cfg harness.SessionConfig) bool {
	switch strings.ToLower(cfg.Provider) {
	case "", "default", "anthropic", "claude-native":
	default:
		return false
	}
	base := cfg.Env["ANTHROPIC_BASE_URL"]
	if base == "" {
		base = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if base == "" {
		return true
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "anthropic.com" || strings.HasSuffix(host, ".anthropic.com")
}

func (c *ClaudeCodeHarness) SendPrompt(ctx context.Context, text string, attachments []protocol.Attachment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return fmt.Errorf("processo do Claude Code não está ativo")
	}

	if c.mode == harness.ModeSDK {
		if c.stdin == nil {
			return fmt.Errorf("stdin do Claude Code SDK indisponível")
		}
		payload := map[string]interface{}{
			"method": "prompt",
			"params": map[string]interface{}{
				"text":   text,
				"images": attachments,
			},
		}
		data, _ := json.Marshal(payload)
		_, err := fmt.Fprintf(c.stdin, "%s\n", data)
		return err
	}

	// Modo CLI: executa claude -p com streaming em tempo real
	sessID := c.cfg.SessionID
	// O claude -p entende comandos de barra (skills, comandos personalizados e embutidos):
	// eles seguem literalmente. Só /model e /effort mudam o que a ponte passa nas próximas chamadas.
	if name, rest, ok := harness.SlashCommand(text); ok && rest != "" {
		switch name {
		case "model":
			c.cfg.Model = rest
			c.announceLocal(sessID, "Modelo das próximas chamadas: "+rest)
			return nil
		case "effort":
			c.cfg.Options = harness.WithOption(c.cfg.Options, "effort", rest)
			c.announceLocal(sessID, "Esforço das próximas chamadas: "+rest)
			return nil
		}
	}
	args := buildCLIArgs(c.cfg, c.resumeOption(), c.cliPermissionMode(), text)
	live := vivasLigadas(c.cfg)
	cwd, env, ctxProc := c.cfg.CWD, c.env, c.ctx
	go func() {
		c.emit(harness.Event{
			Type: harness.EventThinking,
			Payload: protocol.ThinkingParams{
				SessionID: sessID,
				Delta:     "Consultando Claude Code CLI...",
			},
		})

		cmd := exec.CommandContext(ctxProc, "claude", args...)
		process.Configure(cmd)
		cmd.Dir = cwd
		cmd.Env = env

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			c.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
			return
		}

		var liveIn io.WriteCloser
		if live {
			if liveIn, err = cmd.StdinPipe(); err != nil {
				c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
				return
			}
		}
		if err := cmd.Start(); err != nil {
			c.emit(harness.Event{
				Type:    harness.EventError,
				Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()},
			})
			return
		}
		c.mu.Lock()
		c.cmd = cmd
		if live {
			c.vivoStdin, c.vivasPend = liveIn, map[string]bool{}
			if err := escreverUsuario(liveIn, novoUUID(), text); err != nil {
				c.vivoStdin = nil
			}
		}
		c.mu.Unlock()
		stderrDone := make(chan struct{})
		go func() { defer close(stderrDone); c.readStderr(stderr) }()

		turn := &cliTurn{}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for scanner.Scan() {
			c.parseCLIEvent(scanner.Bytes(), sessID, turn)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: "falha lendo stream do Claude: " + scanErr.Error()}})
		}

		<-stderrDone
		err = cmd.Wait()
		c.mu.Lock()
		if c.cmd == cmd {
			c.cmd = nil
		}
		stopped := c.stopped
		tail := strings.TrimSpace(c.stderrTail)
		c.stderrTail = ""
		if live && c.vivoStdin == liveIn {
			c.vivoStdin, c.vivasPend = nil, nil
		}
		c.mu.Unlock()
		if stopped {
			return
		}
		if turn.suprimido {
			// O processo acabou antes do resultado da mensagem viva: fecha o turno que ficou em suspenso.
			if err != nil {
				c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: err.Error()}})
			}
			c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "completed"}})
			return
		}
		// O evento "result" do stream já encerrou o turno; um segundo complete aqui
		// faria o turno seguinte parecer terminado sem texto.
		if turn.completed {
			if err != nil {
				message := err.Error()
				if tail != "" {
					message += ": " + tail
				}
				c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: message}})
				c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "process_error"}})
			}
			return
		}
		if turn.quotaRejected {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: "limite de cota do Claude Code atingido"}})
		}
		if err != nil {
			message := err.Error()
			if tail != "" {
				message += ": " + tail
			}
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessID, Message: message}})
		}

		c.emit(harness.Event{
			Type: harness.EventComplete,
			Payload: protocol.CompleteParams{
				SessionID: sessID,
				Reason:    "completed",
			},
		})
	}()

	return nil
}

// announceLocal avisa que a ponte tratou o comando sozinha, sem chamar o processo.
// Roda em goroutine porque emit pega o mesmo mutex que SendPrompt segura.
func (c *ClaudeCodeHarness) announceLocal(sessID, msg string) {
	go func() {
		c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessID, Delta: msg + "\n"}})
		c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessID, Reason: "completed"}})
	}()
}

func (c *ClaudeCodeHarness) resumeOption() string {
	for _, key := range []string{"claude_session_id", "resume_session", "session_id"} {
		if value, ok := c.cfg.Options[key].(string); ok && value != "" {
			return value
		}
	}
	return c.resumeID
}

func optionString(options map[string]interface{}, names ...string) string {
	for _, name := range names {
		if value, ok := options[name]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func (c *ClaudeCodeHarness) cliPermissionMode() string {
	value, _ := c.cfg.Options["permissoes"].(string)
	if value == "" {
		value = c.cfg.PermissionMode
	}
	switch strings.ToLower(value) {
	case "pular", "bypass", "bypasspermissions", "always_allow":
		return "bypassPermissions"
	case "perguntar", "ask", "manual":
		return "manual"
	}
	if rodar, ok := c.cfg.Options["rodar"].(bool); ok && rodar {
		return "bypassPermissions"
	}
	return ""
}

// ResumeID permite ao orquestrador retomar a conversa retornada pelo Claude.
func (c *ClaudeCodeHarness) ResumeID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumeID
}

// cliTurn guarda o estado de um único `claude -p`.
// cliTurn guarda o estado de um turno do claude -p. quotaRejected marca um rate_limit_event
// "rejected": só vira falta de cota se o turno terminar sem resultado bem-sucedido.
type cliTurn struct{ completed, quotaRejected, suprimido bool }

func buildCLIArgs(cfg harness.SessionConfig, resume, permission, text string) []string {
	args := []string{"--print", "--output-format", "stream-json", "--verbose"}
	live := vivasLigadas(cfg)
	if live {
		// O prompt e as mensagens seguintes entram pelo stdin (uma linha JSON por mensagem).
		args = append(args, "--input-format", "stream-json", "--replay-user-messages")
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if resume != "" {
		args = append([]string{"--resume", resume}, args...)
	} else if on, _ := cfg.Options["continue"].(bool); on {
		args = append([]string{"--continue"}, args...)
	}
	if permission != "" {
		args = append(args, "--permission-mode", permission)
	}
	args = append(args, typedArgs(cfg)...)
	// Argumentos nativos extras: intactos e na ordem, antes do prompt.
	args = append(args, harness.HarnessArgs(cfg.Options)...)
	if live {
		return args
	}
	return append(args, "--", text)
}

// sdkExtras monta as opções do modo SDK: o que tem campo próprio no SDK vai nele e o resto
// (harness_args, mcp_config, effort) vai em extraArgs, que o SDK converte em flags do claude.
func sdkExtras(cfg harness.SessionConfig) map[string]interface{} {
	out := map[string]interface{}{}
	extra := map[string]interface{}{}
	if v := optionString(cfg.Options, "effort"); v != "" {
		extra["effort"] = v
	}
	if m := harness.OptionStrings(cfg.Options, "mcp_config"); len(m) > 0 {
		extra["mcp-config"] = strings.Join(m, " ")
	}
	// harness_args: "--flag valor" vira {flag: valor}; "--flag" sozinho vira {flag: null}.
	// Argumentos posicionais não existem no SDK e são ignorados.
	ha := harness.HarnessArgs(cfg.Options)
	for i := 0; i < len(ha); i++ {
		if !strings.HasPrefix(ha[i], "--") {
			continue
		}
		flag := strings.TrimPrefix(ha[i], "--")
		if k, v, ok := strings.Cut(flag, "="); ok {
			extra[k] = v
		} else if i+1 < len(ha) && !strings.HasPrefix(ha[i+1], "-") {
			extra[flag] = ha[i+1]
			i++
		} else {
			extra[flag] = nil
		}
	}
	if len(extra) > 0 {
		out["extraArgs"] = extra
	}
	if cfg.SystemPrompt != "" {
		out["appendSystemPrompt"] = cfg.SystemPrompt
	}
	if d := harness.OptionStrings(cfg.Options, "add_dirs"); len(d) > 0 {
		out["additionalDirectories"] = d
	}
	if t := harness.OptionStrings(cfg.Options, "allowed_tools"); len(t) > 0 {
		out["allowedTools"] = t
	}
	if t := harness.OptionStrings(cfg.Options, "disallowed_tools"); len(t) > 0 {
		out["disallowedTools"] = t
	}
	if on, _ := cfg.Options["continue"].(bool); on {
		out["continue"] = true
	}
	return out
}

// typedArgs traduz as opções conhecidas para flags do claude (todas conferidas no --help).
func typedArgs(cfg harness.SessionConfig) []string {
	var args []string
	if v := optionString(cfg.Options, "effort"); v != "" {
		args = append(args, "--effort", v)
	}
	if cfg.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", cfg.SystemPrompt)
	}
	for _, dir := range harness.OptionStrings(cfg.Options, "add_dirs") {
		args = append(args, "--add-dir", dir)
	}
	for _, m := range harness.OptionStrings(cfg.Options, "mcp_config") {
		args = append(args, "--mcp-config", m)
	}
	if tools := harness.OptionStrings(cfg.Options, "allowed_tools"); len(tools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(tools, ","))
	}
	if tools := harness.OptionStrings(cfg.Options, "disallowed_tools"); len(tools) > 0 {
		args = append(args, "--disallowed-tools", strings.Join(tools, ","))
	}
	return args
}

func (c *ClaudeCodeHarness) parseCLIEvent(data []byte, fallbackSession string, turn *cliTurn) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: fallbackSession, Delta: string(data) + "\n"}})
		return
	}
	line := string(data)
	sessionID := stringField(msg, "session_id")
	if sessionID == "" {
		sessionID = fallbackSession
	}
	if sessionID != "" {
		c.mu.Lock()
		c.resumeID = sessionID
		c.mu.Unlock()
	}
	switch stringField(msg, "type") {
	case "system":
		// init só estabelece o ID; post_turn_summary não é texto para a Central.
		// A linha original segue como raw para quem quiser o detalhe.
		c.emit(harness.RawEvent(sessionID, "claude-code", "stdout", line))
	case "assistant", "user":
		if u := stringField(msg, "uuid"); u != "" && boolField(msg, "isReplay") {
			c.mu.Lock()
			delete(c.vivasPend, u) // o claude consumiu a mensagem viva
			c.mu.Unlock()
		}
		message, _ := msg["message"].(map[string]interface{})
		c.parseContentBlocks(message["content"], sessionID, line)
	case "result":
		usage, _ := msg["usage"].(map[string]interface{})
		if usage != nil {
			c.emit(harness.Event{Type: harness.EventUsage, Payload: protocol.UsageParams{SessionID: sessionID, InputTokens: int64(number(usage, "input_tokens")), OutputTokens: int64(number(usage, "output_tokens")), TotalTokens: int64(number(usage, "input_tokens") + number(usage, "output_tokens")), CostUSD: number(msg, "total_cost_usd")}})
		}
		isError := boolField(msg, "is_error")
		reason := stringField(msg, "subtype")
		if reason == "" {
			reason = stringField(msg, "stop_reason")
		}
		// Cota só quando o turno falhou: um resultado bem-sucedido pode citar "rate limit" ou "cota"
		// no próprio texto, e um rate_limit_event no meio do turno não impediu o resultado.
		if isError && (turn.quotaRejected || quotaMessage(msg)) {
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: "limite de cota do Claude Code atingido"}})
		}
		if isError {
			message := stringField(msg, "result")
			if message == "" {
				message = "Claude Code encerrou com erro"
			}
			c.emit(harness.Event{Type: harness.EventError, Payload: protocol.ErrorParams{SessionID: sessionID, Message: message}})
		}
		turn.completed = true
		if isError {
			reason = "process_error"
		}
		if c.fecharTurnoVivo() {
			// Há mensagem viva ainda na fila do claude: vem outro resultado; este não encerra o turno.
			turn.suprimido = true
			return
		}
		turn.suprimido = false
		c.emit(harness.Event{Type: harness.EventComplete, Payload: protocol.CompleteParams{SessionID: sessionID, Reason: reason}})
	case "rate_limit_event":
		info, _ := msg["rate_limit_info"].(map[string]interface{})
		// Só status "rejected" é cota esgotada; overageStatus "rejected" quer dizer apenas que o uso extra pago está desligado.
		// A decisão fica para o fim do turno (resultado com erro ou processo sem resultado).
		if stringField(msg, "status") == "rejected" || stringField(info, "status") == "rejected" {
			turn.quotaRejected = true
		}
		c.emit(harness.RawEvent(sessionID, "claude-code", "stdout", line))
	default:
		c.emit(harness.RawEvent(sessionID, "claude-code", "stdout", line))
	}
}

func (c *ClaudeCodeHarness) parseContentBlocks(value interface{}, sessionID, line string) {
	blocks, _ := value.([]interface{})
	for _, raw := range blocks {
		block, _ := raw.(map[string]interface{})
		switch stringField(block, "type") {
		case "text":
			c.emit(harness.Event{Type: harness.EventText, Payload: protocol.TextParams{SessionID: sessionID, Delta: stringField(block, "text")}})
		case "thinking":
			c.emit(harness.Event{Type: harness.EventThinking, Payload: protocol.ThinkingParams{SessionID: sessionID, Delta: stringField(block, "thinking")}})
		case "tool_use":
			c.emit(harness.Event{Type: harness.EventToolCall, Payload: protocol.ToolCallParams{SessionID: sessionID, CallID: stringField(block, "id"), Tool: stringField(block, "name"), Input: block["input"]}})
		case "tool_result":
			status := "success"
			if boolField(block, "is_error") {
				status = "error"
			}
			output := block["content"]
			out, _ := json.Marshal(output)
			if text, ok := output.(string); ok {
				out = []byte(text)
			}
			c.emit(harness.Event{Type: harness.EventToolResult, Payload: protocol.ToolResultParams{SessionID: sessionID, CallID: stringField(block, "tool_use_id"), Status: status, Output: string(out)}})
		default:
			c.emit(harness.RawEvent(sessionID, "claude-code", "stdout", line))
		}
	}
}

func stringField(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }
func boolField(m map[string]interface{}, key string) bool     { v, _ := m[key].(bool); return v }
func number(m map[string]interface{}, key string) float64     { v, _ := m[key].(float64); return v }

// quotaMessage olha só os campos de erro do resultado, nunca a mensagem inteira.
func quotaMessage(m map[string]interface{}) bool {
	parts := []string{stringField(m, "result"), stringField(m, "subtype")}
	switch e := m["error"].(type) {
	case string:
		parts = append(parts, e)
	case map[string]interface{}:
		data, _ := json.Marshal(e)
		parts = append(parts, string(data))
	}
	if errs, ok := m["errors"].([]interface{}); ok {
		data, _ := json.Marshal(errs)
		parts = append(parts, string(data))
	}
	s := strings.ToLower(strings.Join(parts, " "))
	for _, marker := range []string{"session limit", "usage limit", "hit your limit", "rate limit", "rate_limit", "quota", "limite de uso", "limite de cota", "out of credits"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func (c *ClaudeCodeHarness) RespondPermission(ctx context.Context, reqID string, allow bool, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped || c.stdin == nil {
		return fmt.Errorf("processo do Claude Code não está ativo")
	}

	if c.mode == harness.ModeSDK {
		payload := map[string]interface{}{
			"method": "permission_respond",
			"params": map[string]interface{}{
				"requestId": reqID,
				"allow":     allow,
				"message":   message,
			},
		}
		data, _ := json.Marshal(payload)
		_, err := fmt.Fprintf(c.stdin, "%s\n", data)
		return err
	}

	// Modo CLI: responde com caractere interativo ('y' ou 'n')
	char := "n\n"
	if allow {
		char = "y\n"
	}
	_, err := c.stdin.Write([]byte(char))
	return err
}

func (c *ClaudeCodeHarness) Events() <-chan harness.Event {
	return c.events
}

func (c *ClaudeCodeHarness) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return nil
	}
	c.stopped = true

	if c.cancel != nil {
		c.cancel()
	}

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// Envia sinal SIGINT limpo antes de encerrar
		if process.Interrupt(c.cmd) != nil {
			_ = c.cmd.Process.Signal(os.Interrupt)
		}
	}

	return nil
}

func (c *ClaudeCodeHarness) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sessID := c.cfg.SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		if c.mode == harness.ModeSDK {
			// Parse de mensagens NDJSON do worker
			var msg struct {
				Method string                 `json:"method"`
				Params map[string]interface{} `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &msg); err == nil && msg.Method != "" {
				c.handleSDKMessage(msg.Method, msg.Params)
				continue
			}
		}

		// Fallback para streaming textual (modo CLI ou mensagens de texto puro)
		if events := harness.ParseJSONEvent(line, sessID); len(events) > 0 {
			for _, event := range events {
				c.emit(event)
			}
			continue
		}
		c.emit(harness.Event{
			Type: harness.EventText,
			Payload: protocol.TextParams{
				SessionID: sessID,
				Delta:     line + "\n",
			},
		})
	}

	// Ao fechar a saída do processo
	c.emit(harness.Event{
		Type: harness.EventComplete,
		Payload: protocol.CompleteParams{
			SessionID: sessID,
			Reason:    "process_exit",
		},
	})
}

func (c *ClaudeCodeHarness) handleSDKMessage(method string, params map[string]interface{}) {
	sessID := c.cfg.SessionID

	switch method {
	case "agent.thinking":
		delta, _ := params["delta"].(string)
		c.emit(harness.Event{
			Type:    harness.EventThinking,
			Payload: protocol.ThinkingParams{SessionID: sessID, Delta: delta},
		})
	case "agent.text":
		delta, _ := params["delta"].(string)
		c.emit(harness.Event{
			Type:    harness.EventText,
			Payload: protocol.TextParams{SessionID: sessID, Delta: delta},
		})
	case "agent.permission_request":
		reqID, _ := params["requestId"].(string)
		tool, _ := params["tool"].(string)
		command, _ := params["command"].(string)
		risk, _ := params["risk"].(string)
		c.emit(harness.Event{
			Type: harness.EventPermission,
			Payload: protocol.PermissionRequestParams{
				SessionID: sessID,
				RequestID: reqID,
				Tool:      tool,
				Command:   command,
				Risk:      risk,
			},
		})
	case "agent.tool_call":
		callID, _ := params["callId"].(string)
		tool, _ := params["tool"].(string)
		c.emit(harness.Event{
			Type:    harness.EventToolCall,
			Payload: protocol.ToolCallParams{SessionID: sessID, CallID: callID, Tool: tool, Input: params["input"]},
		})
	case "agent.usage":
		in, out := int64(number(params, "inputTokens")), int64(number(params, "outputTokens"))
		c.emit(harness.Event{
			Type:    harness.EventUsage,
			Payload: protocol.UsageParams{SessionID: sessID, InputTokens: in, OutputTokens: out, TotalTokens: in + out, CostUSD: number(params, "costUsd")},
		})
	case "agent.complete":
		reason, _ := params["reason"].(string)
		if id, _ := params["sessionId"].(string); id != "" {
			c.mu.Lock()
			c.resumeID = id
			c.mu.Unlock()
		}
		c.emit(harness.Event{
			Type:    harness.EventComplete,
			Payload: protocol.CompleteParams{SessionID: sessID, Reason: reason},
		})
	case "agent.raw":
		line, _ := params["line"].(string)
		c.emit(harness.RawEvent(sessID, "claude-code", "stdout", line))
	case "agent.error":
		msg, _ := params["message"].(string)
		c.emit(harness.Event{
			Type:    harness.EventError,
			Payload: protocol.ErrorParams{SessionID: sessID, Message: msg},
		})
	}
}

func (c *ClaudeCodeHarness) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		c.emit(harness.RawEvent(c.cfg.SessionID, "claude-code", "stderr", scanner.Text()))
		c.mu.Lock()
		c.stderrTail += scanner.Text() + "\n"
		if len(c.stderrTail) > 2048 {
			c.stderrTail = c.stderrTail[len(c.stderrTail)-2048:]
		}
		c.mu.Unlock()
	}
}

func (c *ClaudeCodeHarness) emit(evt harness.Event) {
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

// vivasLigadas diz se o rodar pediu a entrada de mensagens durante o turno.
func vivasLigadas(cfg harness.SessionConfig) bool {
	on, _ := cfg.Options[harness.OptionMensagensVivas].(bool)
	return on
}

// novoUUID gera um UUID v4 para identificar uma mensagem de entrada.
func novoUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// escreverUsuario grava uma mensagem de usuário em stream-json no stdin do claude.
func escreverUsuario(w io.Writer, uuid, text string) error {
	data, _ := json.Marshal(map[string]interface{}{
		"type": "user", "uuid": uuid,
		"message": map[string]interface{}{"role": "user", "content": text},
	})
	_, err := fmt.Fprintf(w, "%s\n", data)
	return err
}

// fecharTurnoVivo roda ao chegar o resultado de um turno: com mensagem viva ainda sem consumo devolve
// true (o turno continua); sem ela fecha o stdin para o claude sair sozinho e devolve false.
func (c *ClaudeCodeHarness) fecharTurnoVivo() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vivoStdin == nil {
		return false
	}
	if len(c.vivasPend) > 0 {
		return true
	}
	_ = c.vivoStdin.Close()
	c.vivoStdin = nil
	return false
}

// EnviarVivo entrega a mensagem ao claude em execução (stream-json), sem esperar o fim do turno.
func (c *ClaudeCodeHarness) EnviarVivo(id, texto string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.vivoStdin == nil {
		return harness.ErrSemTurnoVivo
	}
	uuid := novoUUID()
	c.vivasPend[uuid] = true
	if err := escreverUsuario(c.vivoStdin, uuid, texto); err != nil {
		delete(c.vivasPend, uuid)
		c.vivoStdin = nil
		return harness.ErrSemTurnoVivo
	}
	return nil
}
