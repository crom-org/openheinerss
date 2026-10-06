package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/doctor"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
	_ "github.com/crom-org/openheinerss/pkg/harness/mock"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/mcp"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/server"
	"github.com/crom-org/openheinerss/pkg/session"
)

var (
	version = "v0.1.0-alpha"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "openheinerss",
		Short: "Openheinerss - O maestro universal de orquestração de AI Coding Agents",
		Long: `🎼 Openheinerss (crom-org)
Regendo a orquestra universal de agentes e harnesses de IA.
Unifica Claude Code, OpenCode, Codex e outros sob um único protocolo JSON-RPC de alta performance.`,
	}

	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newMcpCmd())
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
}

func newServeCmd() *cobra.Command {
	var (
		useStdio bool
		port     int
		host     string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Inicia o servidor de orquestração Openheinerss (STDIO ou WebSocket)",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := session.NewManager()
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if useStdio {
				stdioServer := server.NewStdioServer(manager, os.Stdin, os.Stdout)
				return stdioServer.Run(ctx)
			}

			addr := fmt.Sprintf("%s:%d", host, port)
			wsServer := server.NewWSServer(manager)

			fmt.Fprintf(os.Stderr, "🎼 Openheinerss escutando em ws://%s (pressione Ctrl+C para parar)\n", addr)

			go func() {
				<-ctx.Done()
				shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer sCancel()
				_ = wsServer.Shutdown(shutdownCtx)
			}()

			if err := wsServer.ListenAndServe(addr); err != nil && err.Error() != "http: Server closed" {
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useStdio, "stdio", false, "Executa via pipes padrão STDIO (JSON-RPC / NDJSON)")
	cmd.Flags().IntVarP(&port, "port", "p", 4799, "Porta para o servidor WebSocket")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host de vinculação do WebSocket")

	return cmd
}

func newDoctorCmd() *cobra.Command {
	var targetHarness string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verifica ferramentas, dependências e pré-requisitos do sistema",
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
		Use:   "init",
		Short: "Inicializa o diretório .openheinerss no repositório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
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
		harnessName string
		modeName    string
		provider    string
		model       string
	)

	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Executa um prompt interativo no terminal usando o harness escolhido",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptText := strings.Join(args, " ")
			cwd, _ := os.Getwd()

			manager := session.NewManager()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sessRes, err := manager.CreateSession(ctx, protocol.SessionCreateParams{
				Harness:  harnessName,
				Mode:     modeName,
				CWD:      cwd,
				Provider: provider,
				Model:    model,
			})
			if err != nil {
				return fmt.Errorf("falha ao criar sessão: %v", err)
			}

			fmt.Printf("🚀 Sessão iniciada: %s (Harness: %s, Modo: %s)\n\n", sessRes.SessionID, sessRes.Harness, sessRes.Mode)

			doneChan := make(chan bool)
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

				case protocol.EventAgentToolCall:
					var p protocol.ToolCallParams
					remarshal(notification.Params, &p)
					fmt.Printf("\033[36m⚡ [Ferramenta Executando] %s (Call: %s)\033[0m\n", p.Tool, p.CallID)

				case protocol.EventAgentToolResult:
					var p protocol.ToolResultParams
					remarshal(notification.Params, &p)
					fmt.Printf("\033[32m✔ [Resultado] %s: %s\033[0m\n", p.Status, p.Output)

				case protocol.EventAgentComplete:
					fmt.Println("\n🏁 [Turno Finalizado]")
					select {
					case doneChan <- true:
					default:
					}
				}
			})

			if _, err := manager.PromptSession(ctx, protocol.SessionPromptParams{
				SessionID: sessRes.SessionID,
				Text:      promptText,
			}); err != nil {
				return err
			}

			<-doneChan
			return nil
		},
	}

	cmd.Flags().StringVar(&harnessName, "harness", "mock", "Nome do harness ('mock', 'claude-code', 'opencode')")
	cmd.Flags().StringVar(&modeName, "mode", "mock", "Modo do harness ('mock', 'sdk', 'cli')")
	cmd.Flags().StringVar(&provider, "provider", "", "Provedor do modelo")
	cmd.Flags().StringVar(&model, "model", "", "Nome do modelo")

	return cmd
}

func newMcpCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Gerencia os servidores MCP (Model Context Protocol) do projeto",
	}

	mcpCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Lista os servidores MCP configurados em .openheinerss/mcp.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
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
				if s.Type == "sse" {
					target = s.URL
				}
				fmt.Printf("   • %-15s [%s] %s\n", s.Name, s.Type, target)
			}
			return nil
		},
	})

	mcpCmd.AddCommand(&cobra.Command{
		Use:                "add [nome] [comando] [argumentos...]",
		Short:              "Registra um novo servidor MCP local no projeto",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
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

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Exibe a versão do Openheinerss",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("openheinerss %s (github.com/crom-org/openheinerss)\n", version)
		},
	}
}

func remarshal(src interface{}, dst interface{}) {
	importJSON, _ := src.(map[string]interface{})
	if importJSON != nil {
		// já vem como map
		data, _ := src.([]byte)
		if len(data) == 0 {
			// fallback para serialization rápida
			importJSONBytes, _ := fmt.Sprintf("%v", src), 0
			_ = importJSONBytes
		}
	}
	// Normalização segura
	switch v := src.(type) {
	case protocol.ThinkingParams:
		if p, ok := dst.(*protocol.ThinkingParams); ok {
			*p = v
		}
	case protocol.TextParams:
		if p, ok := dst.(*protocol.TextParams); ok {
			*p = v
		}
	case protocol.PermissionRequestParams:
		if p, ok := dst.(*protocol.PermissionRequestParams); ok {
			*p = v
		}
	case protocol.ToolCallParams:
		if p, ok := dst.(*protocol.ToolCallParams); ok {
			*p = v
		}
	case protocol.ToolResultParams:
		if p, ok := dst.(*protocol.ToolResultParams); ok {
			*p = v
		}
	case protocol.CompleteParams:
		if p, ok := dst.(*protocol.CompleteParams); ok {
			*p = v
		}
	}
}
