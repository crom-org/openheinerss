package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/contas"
	"github.com/crom-org/openheinerss/pkg/identidade"
	"github.com/crom-org/openheinerss/pkg/limites"
	"github.com/spf13/cobra"
)

func contasDirs() ([]string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	if dir, err := config.ConfigDir(); err != nil {
		return nil, "", err
	} else if dir != "" {
		h := filepath.Join(dir, "harnesses")
		return []string{h}, h, nil
	}
	globais, err := config.UserHarnessDirs()
	if err != nil {
		return nil, "", err
	}
	return globais, filepath.Join(cwd, config.WorkspaceDirName, "harnesses"), nil
}

func confirmarConta(pergunta string, in *bufio.Reader) (bool, error) {
	fmt.Printf("%s [digite SIM]: ", pergunta)
	resposta, err := in.ReadString('\n')
	if err != nil && len(resposta) == 0 {
		return false, err
	}
	return strings.TrimSpace(strings.ToUpper(resposta)) == "SIM", nil
}

func exibirContas(items []contas.Conta, jsonOutput bool) error {
	if jsonOutput {
		b, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	for _, c := range items {
		fmt.Printf("%s origem=%s base=%s contaId=%s contaDir=%s login=%t\n", c.Instancia, c.Origem, c.Base, c.ContaID, c.ContaDir, c.TemLogin)
		if c.MesmaContaQue != "" {
			fmt.Printf("  mesma conta que %s\n", c.MesmaContaQue)
		}
	}
	return nil
}

func novaContasCmd() *cobra.Command {
	var jsonOutput bool
	root := &cobra.Command{Use: "contas", Aliases: []string{"accounts"}, Short: "Cria e administra contas de login dos harnesses"}
	listar := &cobra.Command{Use: "listar", Aliases: []string{"list"}, Short: "Lista instâncias de conta sem ler credenciais", RunE: func(cmd *cobra.Command, args []string) error {
		globais, projeto, err := contasDirs()
		if err != nil {
			return err
		}
		items, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return err
		}
		porID := map[string]string{}
		for n := range items {
			if items[n].ContaID == "" {
				continue
			}
			if anterior, ok := porID[items[n].ContaID]; ok {
				items[n].MesmaContaQue = anterior
			} else {
				porID[items[n].ContaID] = items[n].Instancia
			}
		}
		return exibirContas(items, jsonOutput)
	}}
	listar.Flags().BoolVar(&jsonOutput, "json", false, "Emite JSON")
	root.AddCommand(listar)
	var semLogin bool
	var projeto bool
	adicionar := &cobra.Command{Use: "adicionar <harness> [nome]", Aliases: []string{"add"}, Short: "Cria a pasta e a instância e abre o login nativo", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		nome := "principal"
		if len(args) == 2 {
			nome = args[1]
		}
		globais, projetoDir, err := contasDirs()
		if err != nil {
			return err
		}
		dir := globais[0]
		if projeto {
			dir = projetoDir
		}
		if err := config.PastaPrivada(dir); err != nil {
			return err
		}
		res, err := contas.Adicionar(dir, args[0], nome)
		if err != nil {
			return err
		}
		fmt.Printf("Criada %s em %s\n", res.Conta.Instancia, res.Conta.ContaDir)
		if !semLogin {
			fmt.Printf("Login nativo: %s\n", strings.Join(res.ComandoLogin, " "))
			c := exec.CommandContext(cmd.Context(), res.ComandoLogin[0], res.ComandoLogin[1:]...)
			c.Env = append(os.Environ(), contas.LoginEnv(res.Base)+"="+res.ContaDir)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("login de %s: %w", res.Base, err)
			}
		}
		id, err := identidade.Para(res.Instancia, nil)
		if err != nil {
			return err
		}
		fmt.Printf("identidade: instancia=%s base=%s contaId=%s contaDir=%s\n", id.Instancia, id.Base, id.ContaID, id.ContaDir)
		fmt.Println("limites:")
		return exibirLimites(limites.Obter())
	}}
	adicionar.Flags().BoolVar(&semLogin, "sem-login", false, "Só cria a pasta e a instância")
	adicionar.Flags().BoolVar(&projeto, "projeto", false, "Grava a instância no projeto atual (por padrão, grava no global)")
	root.AddCommand(adicionar)
	renomear := &cobra.Command{Use: "renomear <antigo> <novo>", Aliases: []string{"rename"}, Args: cobra.ExactArgs(2), Short: "Renomeia a instância sem mover o login", RunE: func(cmd *cobra.Command, args []string) error {
		globais, projeto, err := contasDirs()
		if err != nil {
			return err
		}
		items, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return err
		}
		var dir string
		for _, item := range items {
			if item.Instancia == args[0] {
				dir = filepath.Dir(item.Arquivo)
				break
			}
		}
		if dir == "" {
			return fmt.Errorf("conta %q não encontrada", args[0])
		}
		c, err := contas.Renomear(dir, args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Renomeada para %s; contaId=%s; pasta mantida em %s\n", c.Instancia, c.ContaID, c.ContaDir)
		return nil
	}}
	root.AddCommand(renomear)
	var sim, apagar bool
	remover := &cobra.Command{Use: "remover <nome>", Aliases: []string{"remove"}, Args: cobra.ExactArgs(1), Short: "Remove a instância e, opcionalmente, sua pasta de login", RunE: func(cmd *cobra.Command, args []string) error {
		globais, projeto, err := contasDirs()
		if err != nil {
			return err
		}
		items, err := contas.ListarCamadas(globais, projeto)
		if err != nil {
			return err
		}
		var alvo contas.Conta
		for _, c := range items {
			if c.Instancia == args[0] {
				alvo = c
			}
		}
		if alvo.Instancia == "" {
			return fmt.Errorf("conta %q não encontrada", args[0])
		}
		in := bufio.NewReader(os.Stdin)
		if !sim {
			ok, err := confirmarConta("Remover somente a instância "+alvo.Instancia+"?", in)
			if err != nil || !ok {
				return fmt.Errorf("remoção cancelada")
			}
		}
		if apagar {
			ok, err := confirmarConta("APAGAR também a pasta de login "+alvo.ContaDir+"?", in)
			if err != nil || !ok {
				return fmt.Errorf("remoção cancelada")
			}
		}
		if _, err := contas.Remover(filepath.Dir(alvo.Arquivo), alvo.Instancia); err != nil {
			return err
		}
		if apagar {
			if err := os.RemoveAll(filepath.Clean(alvo.ContaDir)); err != nil {
				return err
			}
			fmt.Printf("Pasta de login apagada: %s\n", alvo.ContaDir)
		}
		fmt.Printf("Instância removida: %s\n", alvo.Instancia)
		return nil
	}}
	remover.Flags().BoolVar(&sim, "sim", false, "Confirma a remoção sem perguntar")
	remover.Flags().BoolVar(&apagar, "apagar-pasta", false, "Apaga também a pasta de login (exige confirmação extra)")
	root.AddCommand(remover)
	var migrarSim bool
	migrar := &cobra.Command{Use: "migrar", Short: "Copia as instâncias do projeto atual para o global", RunE: func(cmd *cobra.Command, args []string) error {
		globais, projeto, err := contasDirs()
		if err != nil {
			return err
		}
		items, err := contas.Listar(projeto)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("Nenhuma instância do projeto para migrar.")
			return nil
		}
		if !migrarSim {
			ok, err := confirmarConta(fmt.Sprintf("Copiar %d instância(s) para o global sem apagar o projeto?", len(items)), bufio.NewReader(os.Stdin))
			if err != nil || !ok {
				return fmt.Errorf("migração cancelada")
			}
		}
		n, err := contas.Migrar(projeto, globais[0])
		if err != nil {
			return err
		}
		fmt.Printf("Migradas %d instância(s) para %s; as do projeto foram preservadas.\n", n, globais[0])
		return nil
	}}
	migrar.Flags().BoolVar(&migrarSim, "sim", false, "Confirma a migração sem perguntar")
	root.AddCommand(migrar)
	return root
}

func exibirLimites(resultado limites.Resultado) error {
	for _, i := range resultado.Instancias {
		fmt.Printf("  %s (%s)", i.Nome, i.Base)
		if i.Nota != "" {
			fmt.Printf(" — %s", i.Nota)
		}
		fmt.Println()
	}
	return nil
}
