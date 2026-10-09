package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crom-org/openheinerss/pkg/harness/process"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/crom-org/openheinerss/pkg/capacidades"
	"github.com/crom-org/openheinerss/pkg/comandos"
	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/doctor"
	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/motor"
	"github.com/crom-org/openheinerss/pkg/orchestrator"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

var (
	// Estas variáveis recebem valores reais no release pelo GoReleaser. Um
	// build local deliberadamente continua identificável como desenvolvimento.
	Version    = "dev"
	Commit     = "desconhecido"
	Date       = "desconhecida"
	projetoDir string
)

func main() {
	// Subcomando oculto: supervisiona um motor (grupo de processos, Pdeathsig). Sem cobra nem instâncias.
	if len(os.Args) > 1 && os.Args[1] == process.SupervisorArg {
		os.Exit(process.RunSupervisor(os.Args[2:]))
	}
	if exe, err := os.Executable(); err == nil {
		process.UseSupervisor(exe)
	}
	// --config precisa valer antes do cobra: as instâncias entram no catálogo antes dos comandos.
	if dir := flagConfig(os.Args[1:]); dir != "" {
		config.SetConfigDir(dir)
		// Processos filhos (agentes que chamam o openheinerss) herdam a mesma pasta.
		if abs, err := filepath.Abs(dir); err == nil {
			_ = os.Setenv(config.EnvConfigDir, abs)
		}
	}
	if dir := flagProjeto(os.Args[1:]); dir != "" {
		projetoDir = dir
	}
	if _, err := config.ConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
	if cwd, err := projetoAtual(); err == nil {
		if err := carregarInstancias(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "Aviso: %v\n", err)
		}
	}
	rootCmd := newRootCmd()

	// Ctrl-C/SIGTERM cancelam o contexto: o rodar fecha log e meta.json (FIM 130) e para o harness
	// em vez de morrer deixando o CLI do motor órfão.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := rootCmd.ExecuteContext(ctx)
	interrompido := ctx.Err() != nil
	stop()
	var saida codigoSaida
	switch {
	case errors.As(err, &saida):
		os.Exit(int(saida))
	case interrompido:
		os.Exit(130)
	case err != nil:
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
}

// flagConfig acha --config/--configuracao (com valor separado ou após "=") antes do "--".
func flagConfig(args []string) string {
	dir := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		for _, nome := range []string{"--config", "--configuracao"} {
			if a == nome && i+1 < len(args) {
				dir = args[i+1]
				i++
			} else if strings.HasPrefix(a, nome+"=") {
				dir = strings.TrimPrefix(a, nome+"=")
			}
		}
	}
	return dir
}

func flagProjeto(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if args[i] == "--projeto" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--projeto=") {
			return strings.TrimPrefix(args[i], "--projeto=")
		}
	}
	return ""
}

func projetoAtual() (string, error) {
	if projetoDir != "" {
		return filepath.Abs(projetoDir)
	}
	return os.Getwd()
}

// pastaHarnesses é onde ficam as instâncias: <config>/harnesses com --config/OPENHEINERSS_CONFIG,
// senão <base>/.openheinerss/harnesses.
func pastaHarnesses(base string) (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	if dir != "" {
		return filepath.Join(dir, "harnesses"), nil
	}
	return filepath.Join(base, config.WorkspaceDirName, "harnesses"), nil
}

// carregarInstancias carrega as instâncias custom. Ordem: --config > OPENHEINERSS_CONFIG >
// raiz do REPOSITÓRIO (independe da subpasta/worktree) > pasta atual fora de repositório.
func carregarInstancias(cwd string) error {
	if dir, err := config.ConfigDir(); err != nil {
		return err
	} else if dir != "" {
		return harness.LoadCustomDir(filepath.Join(dir, "harnesses"))
	}
	if root, err := orchestrator.RepoRoot(cwd); err == nil {
		if err := harness.LoadCustom(root); err != nil {
			return err
		}
		return nil
	}
	return harness.LoadCustom(cwd)
}

// codigoSaida faz o processo terminar com o código do FIM do rodar (0 ok, 1 erro, 2 sem cota, 3 negado, 4 filho falhou, 5 parado pelo detector, 130 parado).
type codigoSaida int

func (c codigoSaida) Error() string { return fmt.Sprintf("código de saída %d", int(c)) }

func newRootCmd() *cobra.Command {
	versao, commit, data := buildVersion()
	rootCmd := &cobra.Command{
		Use: "openheinerss",
		// --version / -v mostram o mesmo texto do comando `version`.
		Version: fmt.Sprintf("%s (commit %s, data %s)", versao, commit, data),
		Short:   "Openheinerss - O maestro universal de orquestração de AI Coding Agents",
		Long: `🎼 Openheinerss (crom-org)
Regendo a orquestra universal de agentes e harnesses de IA.
Unifica Claude Code, OpenCode, Codex e outros sob um único protocolo JSON-RPC de alta performance.`,
		// O main imprime o erro uma vez só (e decide o código de saída).
		SilenceErrors: true,
	}
	// Lidas também pelo main antes do cobra (ver flagConfig); declaradas aqui para aparecer na ajuda.
	rootCmd.PersistentFlags().String("config", "", "Pasta de configuração (harnesses/ e motores.yaml, ou um projeto com .openheinerss/); vence "+config.EnvConfigDir+" e a busca pela pasta atual")
	rootCmd.PersistentFlags().String("configuracao", "", "Alias de --config")
	rootCmd.PersistentFlags().StringVar(&projetoDir, "projeto", "", "Pasta do projeto cuja configuração deve ser lida (em vez da pasta atual)")

	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newRodarCmd())
	rootCmd.AddCommand(newMotorsCmd())
	rootCmd.AddCommand(newMcpCmd())
	rootCmd.AddCommand(newHarnessCmd())
	rootCmd.AddCommand(newLimitesCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDocsCmd())
	rootCmd.AddCommand(newAgentesCmd())
	rootCmd.AddCommand(newComandosCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newIdentityCmd())
	rootCmd.AddCommand(newCapacidadesCmd())
	rootCmd.AddCommand(novaContasCmd())
	return rootCmd
}

func newCapacidadesCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:     "capacidades [harness]",
		Aliases: []string{"capabilities"},
		Short:   "Mostra o que cada harness lê e aceita (instruções, skills, MCP, retomada, permissões)",
		Long: `Matriz de capacidades por harness ou instância. Cada célula traz a fonte da confirmação
(ajuda do CLI ou texto do binário instalado) ou "nao_confirmado". Sem argumento, mostra as bases embutidas.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var lista []capacidades.Capacidades
			if len(args) == 1 {
				c, err := capacidades.Para(args[0])
				if err != nil {
					return err
				}
				lista = []capacidades.Capacidades{c}
			} else {
				lista = capacidades.Todas()
			}
			if jsonOutput {
				var v interface{} = lista
				if len(args) == 1 {
					v = lista[0]
				}
				b, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(b))
				return nil
			}
			for _, c := range lista {
				for _, l := range capacidades.Linhas(c) {
					fmt.Println(l)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Imprime o resultado em JSON")
	return cmd
}

func newIdentityCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{Use: "identidade <instancia>", Aliases: []string{"identity"}, Short: "Mostra a identidade efetiva de uma instância", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		res, err := identidade.Para(args[0], nil)
		if err != nil {
			return err
		}
		if jsonOutput {
			b, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("instancia=%s base=%s contaId=%s contaDir=%s", res.Instancia, res.Base, res.ContaID, res.ContaDir)
		if res.ConfigFonte != "" {
			fmt.Printf(" configFonte=%s", res.ConfigFonte)
		}
		fmt.Println()
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Imprime o resultado em JSON")
	return cmd
}

// pastaAgentes resolve a pasta de agentes pela raiz do repositório (vale também dentro de worktrees).
func pastaAgentes(dir string) (string, error) {
	cwd, err := projetoAtual()
	if err != nil {
		return "", err
	}
	return orchestrator.ValidateAgentsDir(cwd, dir)
}

func newAgentesCmd() *cobra.Command {
	var agentsDir string
	var jsonOutput bool
	listar := func() error {
		dir, err := pastaAgentes(agentsDir)
		if err != nil {
			return err
		}
		items, err := orchestrator.ListAgents(dir, time.Now())
		if err != nil {
			return err
		}
		if jsonOutput {
			b, err := json.MarshalIndent(items, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		for _, a := range items {
			fmt.Printf("%-20s %-18s motor=%-16s tentativa=%d início=%s duração=%s", a.Nome, a.Estado, a.Motor, a.Tentativa, a.Inicio, a.Duracao)
			if a.Pai != "" {
				fmt.Printf(" pai=%s", a.Pai)
			}
			if len(a.Filhos) > 0 {
				fmt.Printf(" filhos=%s", strings.Join(a.Filhos, ","))
			}
			if len(a.Orfaos) > 0 {
				fmt.Printf(" órfãos=%s", strings.Join(a.Orfaos, ","))
			}
			if a.Orfao {
				fmt.Printf(" ÓRFÃO(pai terminou)")
			}
			if a.PaiMorto {
				fmt.Printf(" PAI MORTO")
			}
			fmt.Printf(" última=%s\n", a.UltimaLinha)
		}
		return nil
	}
	root := &cobra.Command{Use: "agentes", Aliases: []string{"agents"}, Short: "Lista e controla agentes em execução", RunE: func(cmd *cobra.Command, args []string) error { return listar() }}
	root.PersistentFlags().StringVar(&agentsDir, "pasta-agentes", ".claude/agentes", "Pasta dos agentes (relativa à raiz do repositório)")
	root.PersistentFlags().StringVar(&agentsDir, "agents-dir", ".claude/agentes", "Alias em inglês de --pasta-agentes")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Emite JSON")
	root.AddCommand(&cobra.Command{Use: "listar", Aliases: []string{"list"}, Short: "Lista os agentes e seus estados", RunE: func(cmd *cobra.Command, args []string) error { return listar() }})
	root.AddCommand(&cobra.Command{Use: "ver <nome>", Aliases: []string{"show"}, Short: "Mostra o fim do log de um agente", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := pastaAgentes(agentsDir)
		if err != nil {
			return err
		}
		log, err := orchestrator.ShowAgentLog(dir, args[0], 80)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("agente %q não encontrado", args[0])
			}
			return err
		}
		fmt.Println(log)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "parar <nome>", Aliases: []string{"stop"}, Short: "Para somente o agente informado", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := pastaAgentes(agentsDir)
		if err != nil {
			return err
		}
		metaPath := filepath.Join(dir, "logs", args[0]+".meta.json")
		logPath := filepath.Join(dir, "logs", args[0]+".log")
		if _, metaErr := os.Stat(metaPath); os.IsNotExist(metaErr) {
			if _, logErr := os.Stat(logPath); os.IsNotExist(logErr) {
				return fmt.Errorf("agente %q não encontrado", args[0])
			}
		}
		orfao, err := orchestrator.PararAgente(dir, args[0], time.Now())
		if err != nil {
			return err
		}
		if orfao {
			fmt.Printf("Agente %s era órfão (processo já morto): meta fechado (código -1), nenhum sinal enviado\n", args[0])
			return nil
		}
		fmt.Printf("Agente %s parado (código 130)\n", args[0])
		return nil
	}})
	var forcar bool
	desfazer := &cobra.Command{Use: "desfazer <nome> [n]", Aliases: []string{"undo"}, Short: "Volta a worktree do agente a um checkpoint (sem n: o anterior ao último)", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := pastaAgentes(agentsDir)
		if err != nil {
			return err
		}
		n := 0
		if len(args) == 2 {
			if n, err = strconv.Atoi(args[1]); err != nil || n <= 0 {
				return fmt.Errorf("n inválido: %q", args[1])
			}
		}
		cp, err := orchestrator.DesfazerAgente(dir, args[0], n, forcar)
		if err != nil {
			return err
		}
		fmt.Printf("Agente %s restaurado ao checkpoint %d (%s). O estado anterior ficou em um checkpoint %q: para refazer, use 'agentes desfazer %s <n>' com o n dele (veja 'agentes checkpoints %s').\n", args[0], cp.N, cp.Motivo, orchestrator.MotivoAntesDeDesfazer, args[0], args[0])
		return nil
	}}
	desfazer.Flags().BoolVar(&forcar, "forcar", false, "Restaura mesmo com o agente rodando")
	root.AddCommand(desfazer)
	root.AddCommand(&cobra.Command{Use: "checkpoints <nome>", Short: "Lista os checkpoints git do agente (n, quando, motivo, resumo do diff)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := pastaAgentes(agentsDir)
		if err != nil {
			return err
		}
		wt := filepath.Join(dir, args[0])
		itens, err := orchestrator.ResumoCheckpoints(wt, args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			b, err := json.MarshalIndent(itens, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		for _, c := range itens {
			fmt.Printf("%3d  %s  %-18s %s\n", c.N, c.Em, c.Motivo, c.Resumo)
		}
		return nil
	}})
	return root
}

func newDocsCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:     "docs",
		Aliases: []string{"documentacao"},
		Short:   "Gera ou verifica o manual do CLI em docs/09-cli.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := filepath.Abs(filepath.Join("docs", "09-cli.md"))
			if err != nil {
				return err
			}
			data, err := renderCLIDoc(newRootCmd())
			if err != nil {
				return err
			}
			if check {
				existing, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				if !bytes.Equal(existing, data) {
					return fmt.Errorf("%s está desatualizado; execute 'openheinerss docs'", path)
				}
				return nil
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
			fmt.Printf("Manual gerado: %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Falha se docs/09-cli.md não corresponder ao --help atual")
	return cmd
}

func renderCLIDoc(root *cobra.Command) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("# Manual do CLI — referência gerada\n\n")
	out.WriteString("Este arquivo é gerado pelo comando `openheinerss docs` a partir do `--help` real. Não edite manualmente.\n\n")
	commands := []*cobra.Command{root}
	for i := 0; i < len(commands); i++ {
		commands = append(commands, commands[i].Commands()...)
	}
	for _, command := range commands {
		var section bytes.Buffer
		section.WriteString("## `openheinerss")
		if command != root {
			section.WriteString(" " + command.CommandPath()[len("openheinerss "):])
		}
		section.WriteString(" --help`\n\n```text\n")
		var helpOut bytes.Buffer
		command.SetOut(&helpOut)
		command.SetErr(&helpOut)
		if err := command.Help(); err != nil {
			return nil, err
		}
		section.Write(helpOut.Bytes())
		section.WriteString("\n```\n\n")
		out.Write(section.Bytes())
	}
	return out.Bytes(), nil
}

func newServeCmd() *cobra.Command {
	var (
		useStdio    bool
		port        int
		host        string
		maxAgentes  int
		negarEnc    bool
		classRisco  bool
		limitesCada int
	)

	cmd := &cobra.Command{
		Use:           "serve",
		Aliases:       []string{"servir"},
		Short:         "Inicia o servidor de orquestração Openheinerss (STDIO ou WebSocket)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := session.NewManager()
			defer manager.Close()
			manager.SetClassificarRisco(classRisco)
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if useStdio {
				stdioServer := server.NewStdioServerWithMaxAgents(manager, os.Stdin, os.Stdout, maxAgentes)
				stdioServer.SetNegarEncerra(negarEnc)
				stdioServer.Router().IniciarAtualizacaoLimites(ctx, time.Duration(limitesCada)*time.Minute)
				return stdioServer.Run(ctx)
			}

			addr := fmt.Sprintf("%s:%d", host, port)
			wsServer := server.NewWSServerWithMaxAgents(manager, maxAgentes)
			wsServer.SetNegarEncerra(negarEnc)
			wsServer.Router().IniciarAtualizacaoLimites(ctx, time.Duration(limitesCada)*time.Minute)
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("abrir porta %s: %w", addr, err)
			}
			fmt.Fprintf(os.Stderr, "🎼 Openheinerss escutando em ws://%s (pressione Ctrl+C para parar)\n", addr)

			go func() {
				<-ctx.Done()
				// Rede de segurança: se algo travar no desligamento (sessão, agente, conexão), não fica processo preso.
				time.AfterFunc(10*time.Second, func() {
					fmt.Fprintln(os.Stderr, "encerramento demorou mais de 10 s; saindo à força")
					os.Exit(1)
				})
				shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer sCancel()
				_ = wsServer.Shutdown(shutdownCtx)
			}()

			if err := wsServer.Serve(listener); err != nil && err.Error() != "http: Server closed" {
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useStdio, "stdio", false, "Executa via pipes padrão STDIO (JSON-RPC / NDJSON)")
	defaultPort := configuredPort()
	cmd.Flags().IntVarP(&port, "port", "p", defaultPort, "Porta para o servidor WebSocket (alias de --porta)")
	cmd.Flags().IntVar(&port, "porta", defaultPort, "Porta para o servidor WebSocket")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host de vinculação do WebSocket")
	cmd.Flags().StringVar(&host, "hospedeiro", "127.0.0.1", "Alias em português de --host")
	cmd.Flags().IntVar(&maxAgentes, "max-agentes", 0, "Máximo de agentes simultâneos no servidor")
	cmd.Flags().IntVar(&maxAgentes, "max-agents", 0, "Alias em inglês de --max-agentes")
	cmd.Flags().BoolVar(&negarEnc, "negar-encerra", false, "Negar em rodar.decidir encerra a execução (código 3, motivo negado) sem nova tentativa")
	cmd.Flags().BoolVar(&negarEnc, "deny-ends", false, "Alias em inglês de --negar-encerra")
	cmd.Flags().IntVar(&limitesCada, "limites-a-cada", 0, "Atualiza limites em segundo plano a cada N minutos (0 desliga)")
	addRiscoFlag(cmd, &classRisco)

	return cmd
}

func configuredPort() int {
	for _, name := range []string{"OPENHEINERSS_PORTA", "OPENHEINERSS_PORT"} {
		if value, ok := os.LookupEnv(name); ok {
			var port int
			if _, err := fmt.Sscanf(value, "%d", &port); err == nil && port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return 4820
}

func newDoctorCmd() *cobra.Command {
	var targetHarness string

	cmd := &cobra.Command{
		Use:     "doctor",
		Aliases: []string{"diagnostico"},
		Short:   "Verifica ferramentas, dependências e pré-requisitos do sistema",
		Run: func(cmd *cobra.Command, args []string) {
			res := doctor.CheckEnvironment(targetHarness)
			fmt.Print(doctor.FormatDoctorReport(res))
		},
	}

	cmd.Flags().StringVar(&targetHarness, "harness", "", "Harness específico para validar pré-requisitos")
	return cmd
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "init",
		Aliases: []string{"inicializar"},
		Short:   "Inicializa o diretório .openheinerss no repositório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := projetoAtual()
			if err != nil {
				return err
			}

			if err := config.InitWorkspace(cwd); err != nil {
				return err
			}

			fmt.Println("✅ Estrutura .openheinerss inicializada com sucesso!")
			fmt.Println("   - .openheinerss/config.yaml (configurações do projeto)")
			fmt.Println("   - .openheinerss/mcp.json    (servidores MCP)")
			fmt.Println("   - .openheinerss/sessions/   (histórico de sessões)")
			fmt.Println("   - .openheinerss/checkpoints/ (pontos de restauração)")
			return nil
		},
	}
}

func newRunCmd() *cobra.Command {
	var (
		role        string
		motorName   string
		harnessName string
		modeName    string
		provider    string
		model       string
		effort      string
		resumeID    string
		harnessArgs []string
		interactive bool
		semMCP      bool
		mcpNomes    []string
		classRisco  bool
	)

	cmd := &cobra.Command{
		Use:     "run [prompt]",
		Aliases: []string{"executar"},
		Short:   "Executa um prompt interativo no terminal usando o harness escolhido",
		Args:    cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptText := strings.Join(args, " ")
			if promptText == "" && !interactive {
				return fmt.Errorf("run exige um prompt; com --retomar informe o novo prompt depois do ID (ou use --interativo)")
			}
			cwd, _ := projetoAtual()
			var env map[string]string
			var extra map[string]interface{}
			if role != "" {
				roles, path, err := motor.FindRoles(cwd)
				if err != nil {
					return err
				}
				p, ok := roles[role]
				if !ok {
					return fmt.Errorf("papel '%s' não encontrado em %s", role, path)
				}
				harnessName, modeName, provider, model, env, effort = p.Harness, p.Mode, p.Provider, p.Model, p.Env, p.Effort
			} else if motorName != "" {
				p, err := motor.Resolve(motorName, model, effort)
				if err != nil {
					return err
				}
				harnessName, modeName, provider, model, env, effort = p.Harness, p.Mode, p.Provider, p.Model, p.Env, p.Effort
			}
			if effort != "" {
				extra = map[string]interface{}{"effort": effort}
			}

			manager := session.NewManager()
			defer manager.Close()
			manager.SetClassificarRisco(classRisco)
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			var sessRes *protocol.SessionCreateResult
			var err error
			// --retomar aceita o id de uma sessão do openheinerss (com o histórico gravado) ou o id nativo
			// da conversa do harness; neste caso o harness vem de --harness/--motor/--papel.
			if resumeID != "" {
				sessRes, err = manager.ResumeSession(ctx, protocol.SessionResumeParams{SessionID: resumeID, CWD: cwd})
				var rpcErr *protocol.RPCError
				if errors.As(err, &rpcErr) && rpcErr.Code == protocol.CodeSessionNotFound && harnessName != "" {
					sessRes, err = manager.CreateSession(ctx, protocol.SessionCreateParams{
						Harness: harnessName, Mode: modeName, CWD: cwd, Provider: provider, Model: model, Env: env, Retomar: resumeID,
						Options: protocol.SessionOptions{Extra: extra, Effort: effort, HarnessArgs: harnessArgs, SemMCP: semMCP, MCP: mcpNomes},
					})
				}
			} else {
				sessRes, err = manager.CreateSession(ctx, protocol.SessionCreateParams{
					Harness:  harnessName,
					Mode:     modeName,
					CWD:      cwd,
					Provider: provider,
					Model:    model,
					Env:      env,
					Options:  protocol.SessionOptions{Extra: extra, Effort: effort, HarnessArgs: harnessArgs, SemMCP: semMCP, MCP: mcpNomes},
				})
			}
			if err != nil {
				return fmt.Errorf("falha ao criar sessão: %v", err)
			}

			fmt.Printf("🚀 Sessão iniciada: %s (Harness: %s, Modo: %s)\n\n", sessRes.SessionID, sessRes.Harness, sessRes.Mode)

			// Com buffer: o fim do turno pode chegar antes de o laço principal começar a esperar.
			doneChan := make(chan bool, 1)
			reader := bufio.NewReader(os.Stdin)

			manager.SubscribeEvents(func(notification protocol.Notification) {
				switch notification.Method {
				case protocol.EventAgentThinking:
					var p protocol.ThinkingParams
					remarshal(notification.Params, &p)
					fmt.Printf("\033[90m🤔 [Pensando] %s\033[0m\n", p.Delta)

				case protocol.EventAgentText:
					var p protocol.TextParams
					remarshal(notification.Params, &p)
					fmt.Printf("🤖 %s\n", p.Delta)

				case protocol.EventAgentPermissionRequest:
					var p protocol.PermissionRequestParams
					remarshal(notification.Params, &p)
					fmt.Printf("\n⚠️  [PERMISSÃO NECESSÁRIA] Ferramenta: %s | Comando: %s\n", p.Tool, p.Command)
					if p.Risco != "" {
						fmt.Printf("   Risco: %s (%s)\n", p.Risco, p.MotivoRisco)
					}
					fmt.Print("   Deseja autorizar a execução? [s/N]: ")
					resp, _ := reader.ReadString('\n')
					resp = strings.TrimSpace(strings.ToLower(resp))
					decision := "deny"
					if resp == "s" || resp == "sim" || resp == "y" || resp == "yes" {
						decision = "allow"
					}
					_, _ = manager.RespondPermission(ctx, protocol.PermissionRespondParams{
						SessionID: p.SessionID,
						RequestID: p.RequestID,
						Decision:  decision,
					})

				case protocol.EventAgentError:
					var p protocol.ErrorParams
					remarshal(notification.Params, &p)
					fmt.Fprintf(os.Stderr, "❌ %s\n", p.Message)
					if p.SuggestedFix != "" {
						fmt.Fprintf(os.Stderr, "   Como resolver: %s\n", p.SuggestedFix)
					}

				case protocol.EventAgentToolCall:
					var p protocol.ToolCallParams
					remarshal(notification.Params, &p)
					if p.Risco != "" {
						fmt.Printf("\033[36m⚡ [Ferramenta Executando] %s (Call: %s) [risco %s: %s]\033[0m\n", p.Tool, p.CallID, p.Risco, p.MotivoRisco)
					} else {
						fmt.Printf("\033[36m⚡ [Ferramenta Executando] %s (Call: %s)\033[0m\n", p.Tool, p.CallID)
					}

				case protocol.EventAgentToolResult:
					var p protocol.ToolResultParams
					remarshal(notification.Params, &p)
					fmt.Printf("\033[32m✔ [Resultado] %s: %s\033[0m\n", p.Status, p.Output)

				case protocol.EventAgentRaw:
					var p protocol.RawParams
					remarshal(notification.Params, &p)
					fmt.Printf("\033[90m[raw %s] %s\033[0m\n", p.Stream, p.Line)

				case protocol.EventAgentComplete:
					fmt.Println("\n🏁 [Turno Finalizado]")
					select {
					case doneChan <- true:
					default:
					}
				}
			})

			// A CLI não intercepta nenhuma "/": a linha vai literalmente ao harness.
			send := func(text string) error {
				_, err := manager.PromptSession(ctx, protocol.SessionPromptParams{SessionID: sessRes.SessionID, Text: text})
				return err
			}
			wait := func() error {
				select {
				case <-doneChan:
					return nil
				case <-ctx.Done():
					return fmt.Errorf("interrompido")
				}
			}
			if promptText != "" {
				if err := send(promptText); err != nil {
					if !interactive {
						return err
					}
					fmt.Fprintf(os.Stderr, "❌ %v\n", err)
				} else if err := wait(); err != nil {
					return err
				}
			}
			if !interactive {
				return nil
			}
			for {
				fmt.Print("> ")
				line, rerr := reader.ReadString('\n')
				line = strings.TrimRight(line, "\r\n")
				if strings.TrimSpace(line) != "" {
					if err := send(line); err != nil {
						// erro como "comando sem equivalente" aparece e a sessão continua
						fmt.Fprintf(os.Stderr, "❌ %v\n", err)
					} else if err := wait(); err != nil {
						return err
					}
				}
				if rerr != nil {
					return nil
				}
			}
		},
	}

	cmd.Flags().StringVar(&role, "papel", "", "Papel definido em .openheinerss/motores.yaml")
	cmd.Flags().StringVar(&role, "role", "", "Alias em inglês de --papel")
	cmd.Flags().StringVar(&motorName, "motor", "", "Harness base ou instância custom definida pelo usuário")
	cmd.Flags().StringVar(&motorName, "engine", "", "Alias em inglês de --motor")
	cmd.Flags().StringVar(&harnessName, "harness", "", "Nome do harness (obrigatório ou default_harness em config.yaml)")
	cmd.Flags().StringVar(&modeName, "mode", "", "Modo do harness (cli ou sdk; a configuração pode definir default_mode)")
	cmd.Flags().StringVar(&modeName, "modo", "", "Alias em português de --mode")
	cmd.Flags().StringVar(&provider, "provider", "", "Provedor do modelo")
	cmd.Flags().StringVar(&provider, "provedor", "", "Alias em português de --provider")
	cmd.Flags().StringVar(&model, "model", "", "Nome do modelo")
	cmd.Flags().StringVar(&model, "modelo", "", "Alias em português de --model")
	cmd.Flags().StringVar(&effort, "effort", "", "Alias em inglês de --esforco")
	cmd.Flags().StringVar(&effort, "esforco", "", "Esforço de raciocínio do motor")
	cmd.Flags().StringVar(&resumeID, "resume", "", "Alias em inglês de --retomar")
	cmd.Flags().StringVar(&resumeID, "retomar", "", "Retoma pelo ID: sessão persistida do openheinerss ou id nativo da conversa (com --harness/--motor)")
	addHarnessArgFlags(cmd, &harnessArgs)
	cmd.Flags().BoolVarP(&interactive, "interativo", "i", false, "Sessão interativa: lê um prompt por linha; linhas com / vão literalmente ao harness")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Alias em inglês de --interativo")
	addMCPFlags(cmd, &semMCP, &mcpNomes)
	addRiscoFlag(cmd, &classRisco)

	return cmd
}

// addMCPFlags liga --sem-mcp e --mcp (servidores de mcp.json entregues ao harness nesta execução).
func addMCPFlags(cmd *cobra.Command, sem *bool, nomes *[]string) {
	cmd.Flags().BoolVar(sem, "sem-mcp", false, "Não entrega ao harness os servidores de mcp.json (global e do projeto)")
	cmd.Flags().BoolVar(sem, "no-mcp", false, "Alias em inglês de --sem-mcp")
	cmd.Flags().StringSliceVar(nomes, "mcp", nil, "Entrega só estes servidores de mcp.json (repita ou separe por vírgula; padrão: todos)")
}

// addRiscoFlag liga o classificador de risco opcional (desligado por padrão: a ponte é só túnel).
func addRiscoFlag(cmd *cobra.Command, on *bool) {
	cmd.Flags().BoolVar(on, "classificar-risco", false, "Acrescenta risco (baixo|medio|alto) e motivo a tool_call e permission_request; só informa, nunca bloqueia")
	cmd.Flags().BoolVar(on, "classify-risk", false, "Alias em inglês de --classificar-risco")
}

// argsHarness acumula cada ocorrência de --harness-arg/--arg sem separar por vírgula,
// e os dois nomes compartilham a lista para manter a ordem da linha de comando.
type argsHarness struct{ p *[]string }

func (a argsHarness) Set(v string) error { *a.p = append(*a.p, v); return nil }
func (a argsHarness) String() string {
	if a.p == nil {
		return "[]"
	}
	return "[" + strings.Join(*a.p, " ") + "]"
}
func (a argsHarness) Type() string { return "stringArray" }

func addHarnessArgFlags(cmd *cobra.Command, dst *[]string) {
	cmd.Flags().Var(argsHarness{dst}, "harness-arg", "Argumento nativo extra para o harness, intacto e na ordem (repetível)")
	cmd.Flags().Var(argsHarness{dst}, "arg", "Alias de --harness-arg")
}

func newRodarCmd() *cobra.Command {
	var modelo, esforco, modo, prompt, texto, pasta, branchBase, conta, regras, arquivoChaves string
	var retomar, semRegras, seco, semTroca, jsonOutput bool
	var sessao string
	var carga, cargaAbaixo float64
	var maxAgentes, tentativas int
	var cotaMax float64
	var eventosLog string
	var harnessArgs []string
	var esperarFilhos string
	var rodadasFilhos int
	var filhosObrigatorios bool
	var semMCP bool
	var mcpNomes []string
	var limiteContexto int
	var acaoContexto string
	var paradoAviso, paradoParar time.Duration
	cmd := &cobra.Command{
		Use:     "rodar <nome> <instância|harness>",
		Aliases: []string{"launch", "dispatch"},
		Short:   "Executa uma missão com worktree, log e retomada",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true // a partir daqui um erro não é de uso
			if os.Getenv("RETOMAR") == "1" {
				retomar = true
			}
			cwd, err := projetoAtual()
			if err != nil {
				return err
			}
			if cargaAbaixo > 0 {
				carga = cargaAbaixo
			}
			espera, err := orchestrator.ParseEsperarFilhos(esperarFilhos)
			if err != nil {
				return err
			}
			// A escolha de MCP chega aos adaptadores pelo ambiente (OPENHEINERSS_MCP).
			if semMCP {
				_ = os.Setenv(mcp.EnvSelecao, "nenhum")
			} else if len(mcpNomes) > 0 {
				_ = os.Setenv(mcp.EnvSelecao, strings.Join(mcpNomes, ","))
			}
			if os.Getenv(orchestrator.EnvPai) != "" && !seco {
				// Agente filho: sessão própria, para não morrer junto com o grupo do harness do pai.
				orchestrator.DesligarDoGrupo()
			}
			var limiteCtx *int
			if cmd.Flags().Changed("limite-contexto") || cmd.Flags().Changed("context-limit") {
				limiteCtx = &limiteContexto
			}
			var pAviso, pParar *time.Duration
			if cmd.Flags().Changed("parado-aviso") {
				pAviso = &paradoAviso
			}
			if cmd.Flags().Changed("parado-parar") {
				pParar = &paradoParar
			}
			opts := orchestrator.Options{ParadoAviso: pAviso, ParadoParar: pParar, LimiteContexto: limiteCtx, AcaoContexto: acaoContexto, Name: args[0], Motor: args[1], Model: modelo, Effort: esforco, Mode: modo, Conta: conta, PromptFile: prompt, PromptText: texto, Regras: regras, SemRegras: semRegras, Seco: seco, KeysFile: arquivoChaves, Retomar: retomar, SessaoNativa: sessao, SemTrocaConta: semTroca, AgentsDir: pasta, BranchBase: branchBase, MaxLoad: carga, WhenLoadBelow: cargaAbaixo, MaxAgents: maxAgentes, Attempts: tentativas, QuotaMax: cotaMax, EventLog: eventosLog, HarnessArgs: harnessArgs, EsperarFilhos: espera, RodadasFilhos: rodadasFilhos, FilhosObrigatorios: filhosObrigatorios, Pai: os.Getenv(orchestrator.EnvPai), PaiLogs: os.Getenv(orchestrator.EnvPaiLogs)}
			if seco && jsonOutput {
				dry, e := orchestrator.Seco(cmd.Context(), cwd, opts)
				if e != nil {
					return e
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(dry)
			}
			res, err := orchestrator.Run(cmd.Context(), cwd, opts)
			if err != nil && res.Name == "" {
				return err
			}
			fim := fmt.Sprintf("FIM %s código %d", res.Name, res.Code)
			if res.Code != 0 && res.Causa != "" {
				fim += ": " + res.Causa
			}
			fmt.Println(fim)
			if res.LogFile != "" {
				fmt.Printf("log: %s\n", res.LogFile)
			}
			if res.Resumo != "" {
				fmt.Printf("resumo: %s\n", res.Resumo)
			}
			if err != nil {
				return err // interrompido ou falha: o log já tem o FIM; o main escolhe o código de saída
			}
			if res.Code != 0 {
				return codigoSaida(res.Code)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modelo, "modelo", "", "Modelo a usar")
	cmd.Flags().StringVar(&modelo, "model", "", "Alias em inglês de --modelo")
	cmd.Flags().StringVar(&esforco, "esforco", "", "Esforço de raciocínio")
	cmd.Flags().StringVar(&esforco, "effort", "", "Alias em inglês de --esforco")
	cmd.Flags().StringVar(&modo, "modo", "", "Modo do harness (cli ou sdk; a instância pode definir o padrão)")
	cmd.Flags().StringVar(&modo, "mode", "", "Alias em inglês de --modo")
	cmd.Flags().StringVar(&conta, "conta", "", "Conta/provedor da instância")
	cmd.Flags().StringVar(&conta, "account", "", "Alias em inglês de --conta")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Arquivo de prompt alternativo")
	cmd.Flags().StringVar(&texto, "texto", "", "Prompt em texto, no lugar do arquivo prompts/<nome>.md")
	cmd.Flags().StringVar(&texto, "text", "", "Alias em inglês de --texto")
	cmd.Flags().StringVar(&sessao, "sessao", "", "Retoma a conversa existente do harness (id nativo ou id de sessão do openheinerss); não vale para aider")
	cmd.Flags().StringVar(&sessao, "retomar-sessao", "", "Alias de --sessao")
	cmd.Flags().BoolVar(&semTroca, "sem-troca-conta", false, "Acima de --cota-max só pula a instância, sem procurar outra conta da mesma base")
	cmd.Flags().BoolVar(&retomar, "retomar", false, "Acrescenta o texto de continuação e preserva o log")
	cmd.Flags().BoolVar(&retomar, "resume", false, "Alias em inglês de --retomar")
	cmd.Flags().StringVar(&pasta, "pasta-agentes", "", "Pasta dos agentes (padrão .claude/agentes)")
	cmd.Flags().StringVar(&pasta, "agentes", "", "Alias de --pasta-agentes")
	cmd.Flags().StringVar(&pasta, "agents-dir", "", "Alias em inglês de --pasta-agentes")
	cmd.Flags().StringVar(&branchBase, "branch-base", "", "Branch base da worktree (padrão: main, ou a branch atual se não houver main)")
	cmd.Flags().StringVar(&branchBase, "base-branch", "", "Alias em inglês de --branch-base")
	cmd.Flags().Float64Var(&carga, "carga-maxima", 0, "Carga máxima de 1 minuto; 0 desativa")
	cmd.Flags().Float64Var(&carga, "max-load", 0, "Alias em inglês de --carga-maxima")
	cmd.Flags().Float64Var(&cargaAbaixo, "quando-carga-abaixo", 0, "Só começa quando a carga numérica ficar abaixo deste valor")
	cmd.Flags().Float64Var(&cargaAbaixo, "when-load-below", 0, "Alias em inglês de --quando-carga-abaixo")
	cmd.Flags().IntVar(&maxAgentes, "max-agentes", 0, "Máximo de agentes simultâneos")
	cmd.Flags().IntVar(&maxAgentes, "max-agents", 0, "Alias em inglês de --max-agentes")
	cmd.Flags().IntVar(&tentativas, "tentativas", 0, "Máximo de tentativas")
	cmd.Flags().IntVar(&tentativas, "retries", 0, "Alias em inglês de --tentativas")
	cmd.Flags().Float64Var(&cotaMax, "cota-max", 0, "Limiar de cota (%): acima dele pula a instância e troca para outra conta da mesma base com cota (0 desativa; config cota_max)")
	cmd.Flags().Float64Var(&cotaMax, "quota-max", 0, "Alias em inglês de --cota-max")
	cmd.Flags().StringVar(&eventosLog, "eventos-log", "", "Acrescenta FIM ao arquivo de eventos (desligado por padrão)")
	cmd.Flags().StringVar(&eventosLog, "events-log", "", "Alias em inglês de --eventos-log")
	cmd.Flags().IntVar(&limiteContexto, "limite-contexto", 0, "Limite de tokens de contexto da sessão (0 desliga); vence contexto: do config.yaml")
	cmd.Flags().IntVar(&limiteContexto, "context-limit", 0, "Alias em inglês de --limite-contexto")
	cmd.Flags().StringVar(&acaoContexto, "acao-contexto", "", "O que fazer ao passar do limite: aviso ou nova-sessao; vence contexto: do config.yaml")
	cmd.Flags().StringVar(&acaoContexto, "context-action", "", "Alias em inglês de --acao-contexto")
	cmd.Flags().DurationVar(&paradoAviso, "parado-aviso", 0, "Avisa se log, worktree e último evento ficarem parados por este tempo (0 desliga; padrão 10m); vence parado: do config.yaml")
	cmd.Flags().DurationVar(&paradoParar, "parado-parar", 0, "Interrompe (SIGTERM no motor, código 5, retomável) depois deste tempo parado (0 desliga; padrão 20m); vence parado: do config.yaml")
	cmd.Flags().BoolVar(&seco, "seco", false, "Mostra o comando sem executá-lo")
	cmd.Flags().BoolVar(&seco, "dry-run", false, "Alias em inglês de --seco")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Exibe o plano seco como JSON")
	cmd.Flags().StringVar(&regras, "regras", "", "Arquivo de regras do prompt")
	cmd.Flags().StringVar(&regras, "rules", "", "Alias em inglês de --regras")
	cmd.Flags().BoolVar(&semRegras, "sem-regras", false, "Não acrescenta regras padrão ao prompt")
	cmd.Flags().BoolVar(&semRegras, "no-rules", false, "Alias em inglês de --sem-regras")
	cmd.Flags().BoolVar(&semRegras, "sem-regras-padrao", false, "Desliga as regras padrão do prompt")
	cmd.Flags().BoolVar(&semRegras, "no-default-rules", false, "Alias em inglês de --sem-regras-padrao")
	cmd.Flags().StringVar(&arquivoChaves, "arquivo-chaves", "", "Arquivo opcional de variáveis secretas (não imprime valores)")
	cmd.Flags().StringVar(&arquivoChaves, "keys-file", "", "Alias em inglês de --arquivo-chaves")
	cmd.Flags().StringVar(&esperarFilhos, "esperar-filhos", "", "Espera os agentes filhos e retoma a sessão: sim (padrão, até 2h), nao, ou a espera máxima (ex.: 30m)")
	cmd.Flags().StringVar(&esperarFilhos, "wait-children", "", "Alias em inglês de --esperar-filhos")
	cmd.Flags().IntVar(&rodadasFilhos, "rodadas-filhos", 0, "Máximo de retomadas automáticas depois dos filhos (padrão 5)")
	cmd.Flags().IntVar(&rodadasFilhos, "child-rounds", 0, "Alias em inglês de --rodadas-filhos")
	cmd.Flags().BoolVar(&filhosObrigatorios, "filhos-obrigatorios", false, "Falha (código 4, motivo \"filho falhou\") se algum agente filho terminou com código ≠ 0, sem FIM ou ainda rodando")
	cmd.Flags().BoolVar(&filhosObrigatorios, "require-children", false, "Alias em inglês de --filhos-obrigatorios")
	addMCPFlags(cmd, &semMCP, &mcpNomes)
	addHarnessArgFlags(cmd, &harnessArgs)
	return cmd
}

func newLimitesCmd() *cobra.Command {
	var jsonOutput, atualizar, forcar bool
	var intervalo time.Duration
	cmd := &cobra.Command{Use: "limites", Aliases: []string{"limits"}, Short: "Mostra as cotas locais das instâncias Codex e Claude", RunE: func(cmd *cobra.Command, args []string) error {
		resultado := limites.Obter()
		var err error
		if atualizar {
			resultado, err = limites.Atualizar(cmd.Context(), limites.AtualizarOpcoes{Forcar: forcar, Intervalo: intervalo})
			if err != nil {
				return err
			}
		}
		if jsonOutput {
			b, err := json.MarshalIndent(resultado, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("Limites em %s\n", resultado.Agora)
		for _, i := range resultado.Instancias {
			fmt.Printf("%s (%s)", i.Nome, i.Base)
			if i.DadoEm != "" {
				fmt.Printf(" — dado %s, idade %ds", i.DadoEm, i.IdadeSegundos)
			}
			if i.Nota != "" {
				fmt.Printf(" — %s", i.Nota)
			}
			fmt.Printf(" — fonte %s\n", i.Fonte)
			for _, j := range i.Janelas {
				fmt.Printf("  %s: %.1f%%", j.Nome, j.Percentual)
				voltaEm := j.VoltaEm
				if voltaEm == "" {
					voltaEm = j.ReiniciaEm
				}
				if voltaEm != "" {
					fmt.Printf("; voltaEm %s", voltaEm)
				}
				fmt.Println()
			}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emite JSON")
	cmd.Flags().BoolVar(&atualizar, "atualizar", false, "Consulta ativamente cada conta, respeitando o intervalo mínimo")
	cmd.Flags().BoolVar(&forcar, "forcar", false, "Ignora o intervalo mínimo por conta")
	cmd.Flags().DurationVar(&intervalo, "intervalo", 5*time.Minute, "Intervalo mínimo entre consultas por conta")
	return cmd
}

func newMotorsCmd() *cobra.Command {
	return &cobra.Command{Use: "motores", Aliases: []string{"engines"}, Short: "Lista perfis de motores e papéis configurados", RunE: func(cmd *cobra.Command, args []string) error {
		for _, p := range motor.Perfis() {
			fmt.Printf("%-16s harness=%-12s modelo=%s", p.Nome, p.Harness, p.Model)
			if p.Provider != "" {
				fmt.Printf(" provedor=%s", p.Provider)
			}
			fmt.Println()
		}
		cwd, _ := projetoAtual()
		roles, path, err := motor.FindRoles(cwd)
		if err != nil {
			return err
		}
		if len(roles) == 0 {
			fmt.Printf("Nenhum papel em %s\n", path)
			return nil
		}
		fmt.Println("Papéis:")
		for name, p := range roles {
			fmt.Printf("%-16s %s/%s\n", name, p.Nome, p.Model)
		}
		return nil
	}}
}

func newMcpCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Gerencia os servidores MCP (Model Context Protocol) do projeto",
	}
	mcpCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cwd, err := projetoAtual()
		if err != nil {
			return err
		}
		return listarMcpEfetivos(cwd)
	}

	mcpCmd.AddCommand(&cobra.Command{
		Use:     "list",
		Aliases: []string{"listar"},
		Short:   "Lista os servidores MCP configurados em .openheinerss/mcp.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := projetoAtual()
			servers, err := mcp.GetHub().ListServers(cwd)
			if err != nil {
				return err
			}
			if len(servers) == 0 {
				fmt.Println("Nenhum servidor MCP configurado no projeto (.openheinerss/mcp.json).")
				return nil
			}
			fmt.Println("🔌 Servidores MCP Configurados:")
			for _, s := range servers {
				target := s.Command
				if s.URL != "" {
					target = s.URL
				}
				fmt.Printf("   • %-15s [%s] %s\n", s.Name, s.Type, target)
			}
			return nil
		},
	})

	mcpCmd.AddCommand(&cobra.Command{
		Use:     "efetivos",
		Aliases: []string{"effective"},
		Short:   "Lista os servidores que cada harness recebe nesta pasta (global + projeto; valores de env/headers ocultos)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := projetoAtual()
			if err != nil {
				return err
			}
			return listarMcpEfetivos(cwd)
		},
	})

	mcpCmd.AddCommand(&cobra.Command{
		Use:                "add [nome] [comando] [argumentos...]",
		Aliases:            []string{"adicionar"},
		Short:              "Registra um novo servidor MCP local no projeto",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := projetoAtual()
			name := args[0]
			command := args[1]
			mcpArgs := args[2:]

			cfg := mcp.ServerConfig{
				Command: command,
				Args:    mcpArgs,
			}
			if err := mcp.GetHub().RegisterServer(cwd, name, cfg); err != nil {
				return err
			}
			fmt.Printf("✅ Servidor MCP '%s' registrado com sucesso em .openheinerss/mcp.json!\n", name)
			return nil
		},
	})

	return mcpCmd
}

func listarMcpEfetivos(cwd string) error {
	servs, err := mcp.Efetivos(cwd)
	if err != nil {
		return err
	}
	if len(servs) == 0 {
		fmt.Println("Nenhum servidor MCP em mcp.json (global ou do projeto).")
		return nil
	}
	for _, name := range mcp.Nomes(servs) {
		s := mcp.Mascarar(servs[name])
		target := strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
		if s.URL != "" {
			target = s.URL
		}
		var env []string
		for k := range s.Env {
			env = append(env, k+"=***")
		}
		sort.Strings(env)
		fmt.Printf("   • %-15s %s %s\n", name, target, strings.Join(env, " "))
	}
	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"versao"},
		Short:   "Exibe a versão do Openheinerss",
		Run: func(cmd *cobra.Command, args []string) {
			versao, commit, data := buildVersion()
			fmt.Printf("openheinerss %s (commit %s, data %s)\n", versao, commit, data)
		},
	}
}

// buildVersion combina os valores de release (ldflags) com a informação que o
// Go grava automaticamente em um build direto feito dentro de um checkout Git.
func buildVersion() (string, string, string) {
	versao, commit, data := Version, Commit, Date
	commitDoBuild := false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "desconhecido" || commit == "" {
					commit = setting.Value
					commitDoBuild = true
				}
			case "vcs.time":
				if data == "desconhecida" || data == "" {
					data = setting.Value
				}
			}
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.modified" && setting.Value == "true" && commitDoBuild {
				commit += "-modificado"
				break
			}
		}
	}
	return versao, commit, data
}

func newHarnessCmd() *cobra.Command {
	root := &cobra.Command{Use: "harness", Short: "Lista, instala e testa harnesses"}
	root.AddCommand(&cobra.Command{Use: "list", Aliases: []string{"listar"}, Short: "Lista harnesses embutidos e custom", RunE: func(cmd *cobra.Command, args []string) error {
		for _, item := range harness.ListCatalog() {
			origin := item.Origin
			if origin == "" {
				origin = "embutido"
			}
			fmt.Printf("%-24s %-10s %s\n", item.ID, origin, item.DisplayName)
		}
		return nil
	}})
	var global bool
	add := &cobra.Command{Use: "add <arquivo>", Aliases: []string{"adicionar"}, Short: "Valida e copia um arquivo de harness para o projeto ou global", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := harness.LoadCustomFile(args[0]); err != nil {
			return err
		}
		cwd, err := projetoAtual()
		if err != nil {
			return err
		}
		dir, err := pastaHarnesses(cwd)
		if global {
			dir, err = config.UserHarnessDir()
		}
		if err != nil {
			return err
		}
		// Instâncias levam env (chaves, URLs de provedor): pasta 0700 e arquivo 0600.
		if err := config.PastaPrivada(dir); err != nil {
			return err
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.Base(args[0]))
		base := filepath.Base(args[0])
		if strings.HasSuffix(base, ".yaml.yaml") {
			target = filepath.Join(dir, strings.TrimSuffix(base, ".yaml"))
		}
		if strings.HasSuffix(base, ".yml.yml") {
			target = filepath.Join(dir, strings.TrimSuffix(base, ".yml"))
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return err
		}
		fmt.Printf("Harness copiado para %s\n", target)
		return nil
	}}
	add.Flags().BoolVar(&global, "global", false, "Grava na configuração global do usuário")
	root.AddCommand(add)
	var prompt string
	var todos, jsonOutput, incluirPrincipal bool
	var retomar bool
	var modo string
	var pular string
	var timeout time.Duration
	test := &cobra.Command{Use: "test [nome]", Aliases: []string{"testar"}, Short: "Executa um prompt curto e mostra eventos", Args: func(cmd *cobra.Command, args []string) error {
		if todos && len(args) != 0 {
			return fmt.Errorf("--todos não aceita nome")
		}
		if !todos && len(args) != 1 {
			return fmt.Errorf("informe <nome> ou use --todos")
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		if todos {
			return runHarnessTodos(cmd, prompt, jsonOutput, pular, timeout, incluirPrincipal)
		}
		if retomar {
			return runHarnessResume(cmd.Context(), args[0], prompt, timeout)
		}
		testMode := harness.Mode(modo)
		if testMode == "" {
			testMode = harness.ModeCLI
		}
		h, err := harness.Create(args[0], testMode)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		if r := h.ValidatePrerequisites(ctx); !r.Satisfied {
			return fmt.Errorf("pré-requisito ausente: %s", strings.Join(r.MissingItems, ", "))
		}
		cwd, _ := projetoAtual()
		if err := h.Start(ctx, harness.SessionConfig{SessionID: "harness-test", CWD: cwd}); err != nil {
			return err
		}
		defer stopHarnessWithLimit(h)
		if prompt == "" {
			prompt = "responda apenas OK"
		}
		if err := h.SendPrompt(ctx, prompt, nil); err != nil {
			return err
		}
		failure := ""
		for {
			select {
			case e := <-h.Events():
				fmt.Printf("%s %v\n", e.Type, e.Payload)
				if p, ok := e.Payload.(protocol.PermissionRequestParams); ok {
					// O teste é só de conectividade: nega pedidos de ferramenta para o turno acabar em vez de esperar.
					_ = h.RespondPermission(ctx, p.RequestID, false, "harness test não autoriza ferramentas")
				}
				if p, ok := e.Payload.(protocol.ErrorParams); ok && e.Type == harness.EventError && failure == "" {
					failure = p.Message
				}
				if e.Type == harness.EventComplete {
					if failure != "" {
						return fmt.Errorf("%s: %s", classifyLabel(failure), failure)
					}
					return nil
				}
			case <-ctx.Done():
				if failure != "" {
					return fmt.Errorf("tempo esgotado testando harness '%s' (último erro: %s)", args[0], failure)
				}
				return fmt.Errorf("tempo esgotado testando harness '%s'", args[0])
			}
		}
	}}
	test.Flags().StringVar(&prompt, "prompt", "", "Prompt curto para o teste")
	test.Flags().BoolVar(&todos, "todos", false, "Testa todos os harnesses e instâncias em sequência")
	test.Flags().BoolVar(&todos, "all", false, "Alias em inglês de --todos")
	test.Flags().BoolVar(&jsonOutput, "json", false, "Emite a matriz em JSON (com --todos)")
	test.Flags().StringVar(&pular, "pular", "", "Nomes a pular, separados por vírgula")
	test.Flags().StringVar(&pular, "skip", "", "Alias em inglês de --pular")
	test.Flags().DurationVar(&timeout, "timeout", 120*time.Second, "Tempo máximo de cada teste")
	test.Flags().BoolVar(&incluirPrincipal, "incluir-principal", false, "Inclui claude-code (conta principal)")
	test.Flags().BoolVar(&retomar, "retomar", false, "Envia um segundo prompt na mesma sessão")
	test.Flags().BoolVar(&retomar, "resume", false, "Alias em inglês de --retomar")
	test.Flags().StringVar(&modo, "modo", "cli", "Modo do teste individual (cli ou sdk)")
	test.Flags().StringVar(&modo, "mode", "cli", "Alias em inglês de --modo")
	root.AddCommand(test)
	return root
}

// remarshal converte os parâmetros de um evento (struct ou mapa) no tipo de destino.
func remarshal(src interface{}, dst interface{}) {
	data, err := json.Marshal(src)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, dst)
}

func newComandosCmd() *cobra.Command {
	var jsonOutput bool
	var cwd string
	pasta := func() string {
		if cwd != "" {
			return cwd
		}
		d, _ := projetoAtual()
		return d
	}
	mostrar := func(v interface{}) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	mostrarComando := func(c comandos.Comando) error {
		if jsonOutput {
			return mostrar(c)
		}
		fmt.Printf("%s (%s) anotação=%q confirmado=%v\n", c.Nome, c.Repasse, c.Anotacao, c.Confirmado)
		return nil
	}
	root := &cobra.Command{
		Use:     "comandos <harness>",
		Aliases: []string{"commands"},
		Short:   "Lista os comandos nativos (/compact, /model…) de um harness ou instância e como são repassados",
		Long: `Lista os comandos nativos do harness (catálogo embutido da base, mais comandos e skills achados
nos arquivos do harness) mesclados com as anotações do usuário em comandos.yaml.
Repasse: literal (vai como está), traduzido (a ponte troca por flag/opção), sem_equivalente (só existe na tela).
Anotações ficam em ~/.config/openheinerss/comandos.yaml; com --config/` + config.EnvConfigDir + `, em <pasta>/comandos.yaml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := comandos.Listar(args[0], pasta())
			if err != nil {
				return err
			}
			if jsonOutput {
				return mostrar(l)
			}
			fmt.Printf("Harness %s (base %s); /x fora da lista: %s; anotações em %s\n", l.Harness, l.Base, l.Desconhecido, l.Arquivo)
			for _, linha := range comandos.Linhas(l.Comandos) {
				fmt.Println(linha)
			}
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Emite JSON")
	root.PersistentFlags().StringVar(&cwd, "cwd", "", "Pasta do projeto onde procurar comandos/skills do harness (padrão: pasta atual)")
	root.AddCommand(&cobra.Command{Use: "anotar <harness> </comando> <texto>", Aliases: []string{"annotate"}, Short: "Grava uma anotação livre para o comando (vale para as instâncias que herdam do harness)", Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := comandos.Anotar(args[0], args[1], args[2], pasta())
		if err != nil {
			return err
		}
		return mostrarComando(c)
	}})
	root.AddCommand(&cobra.Command{Use: "confirmar <harness> </comando>", Aliases: []string{"confirm"}, Short: "Marca o primeiro uso do comando como já confirmado", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := comandos.Confirmar(args[0], args[1], pasta())
		if err != nil {
			return err
		}
		return mostrarComando(c)
	}})
	return root
}
