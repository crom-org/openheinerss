package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/orchestrator"
)

func newConfigCmd() *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Mostra a configuração efetiva (global + projeto)"}
	var harnessNome string
	mostrar := func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repo, err := orchestrator.RaizDoRepo(cwd)
		if err != nil {
			repo = cwd
		}
		um := func(nome string) error {
			c, err := config.ContextoEfetivo(repo, nome, orchestrator.BaseDe(nome))
			if err != nil {
				return err
			}
			rot := nome
			if rot == "" {
				rot = "(padrão)"
			}
			fmt.Printf("%s: %s\n", rot, c.Descrever())
			return nil
		}
		if harnessNome != "" {
			return um(harnessNome)
		}
		if err := um(""); err != nil {
			return err
		}
		nomes, err := config.NomesHarnessContexto(repo)
		if err != nil {
			return err
		}
		for _, nome := range nomes {
			if err := um(nome); err != nil {
				return err
			}
		}
		return nil
	}
	root.RunE = mostrar
	cmd := &cobra.Command{
		Use:   "contexto",
		Short: "Mostra o limite de contexto efetivo e de onde vem cada campo",
		Long: `Mostra a regra de contexto (contexto: no config.yaml) que o rodar usaria.
Ordem: projeto.harnesses[instância] > projeto.harnesses[base] > projeto.padrao >
global.harnesses[instância] > global.harnesses[base] > global.padrao, campo a campo.
O projeto é <repo>/.openheinerss/config.yaml; o global é --config/` + config.EnvConfigDir + ` ou, sem eles,
~/.config/openheinerss/config.yaml e ~/.openheinerss/config.yaml (este vence).`,
		Args: cobra.NoArgs,
		RunE: mostrar,
	}
	cmd.Flags().StringVar(&harnessNome, "harness", "", "Instância ou harness a resolver (padrão: o padrão e os citados no config)")
	root.AddCommand(cmd)
	return root
}
