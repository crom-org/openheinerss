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

func contasDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return pastaHarnesses(cwd)
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
		fmt.Printf("%s base=%s contaId=%s contaDir=%s login=%t\n", c.Instancia, c.Base, c.ContaID, c.ContaDir, c.TemLogin)
	}
	return nil
}

func novaContasCmd() *cobra.Command {
	var jsonOutput bool
	root := &cobra.Command{Use: "contas", Aliases: []string{"accounts"}, Short: "Cria e administra contas de login dos harnesses"}
	listar := &cobra.Command{Use: "listar", Aliases: []string{"list"}, Short: "Lista instâncias de conta sem ler credenciais", RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := contasDir()
		if err != nil {
			return err
		}
		items, err := contas.Listar(dir)
		if err != nil {
			return err
		}
		return exibirContas(items, jsonOutput)
	}}
	listar.Flags().BoolVar(&jsonOutput, "json", false, "Emite JSON")
	root.AddCommand(listar)
	var semLogin bool
	adicionar := &cobra.Command{Use: "adicionar <harness> [nome]", Aliases: []string{"add"}, Short: "Cria a pasta e a instância e abre o login nativo", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		nome := "principal"
		if len(args) == 2 {
			nome = args[1]
		}
		dir, err := contasDir()
		if err != nil {
			return err
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
	root.AddCommand(adicionar)
	renomear := &cobra.Command{Use: "renomear <antigo> <novo>", Aliases: []string{"rename"}, Args: cobra.ExactArgs(2), Short: "Renomeia a instância sem mover o login", RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := contasDir()
		if err != nil {
			return err
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
		dir, err := contasDir()
		if err != nil {
			return err
		}
		items, err := contas.Listar(dir)
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
		if _, err := contas.Remover(dir, alvo.Instancia); err != nil {
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
