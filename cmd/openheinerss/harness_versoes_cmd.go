package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/versoes"
	"github.com/spf13/cobra"
)

func newHarnessVersoesCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "versoes [harness]",
		Aliases: []string{"versions", "versões"},
		Short:   "Mostra a versão instalada de cada harness base, como foi instalada e a última publicada",
		Long: `Para cada base (claude-code, codex, opencode, aider, agy): a versão de '<cli> --version',
o método de instalação (npm, uv, pipx, pip, brew, script, binario — pelo caminho real do executável)
e a última versão pela fonte oficial (npm, PyPI ou releases do GitHub). Sem rede a última vira
"desconhecida", com o motivo. Não altera nada.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			env := versoes.Padrao()
			var bases []string
			if len(args) == 1 {
				b := versoes.BaseDe(args[0])
				if b == "" {
					return fmt.Errorf("harness %q não é uma base nem uma instância de base conhecida", args[0])
				}
				bases = []string{b}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			infos := env.Versoes(ctx, bases...)
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]interface{}{"harnesses": infos})
			}
			imprimirVersoes(cmd.OutOrStdout(), infos)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Saída em JSON")
	return cmd
}

func imprimirVersoes(w io.Writer, infos []versoes.Info) {
	fmt.Fprintf(w, "%-12s %-12s %-12s %-10s %-14s %s\n", "harness", "instalada", "disponível", "instalação", "estado", "detalhe")
	for _, i := range infos {
		inst, disp := i.Instalada, i.Disponivel
		if inst == "" {
			inst = "-"
		}
		det := i.Caminho
		if i.Motivo != "" {
			det = i.Motivo
		}
		if i.Fonte != "" {
			det += " [fonte: " + i.Fonte + "]"
		}
		fmt.Fprintf(w, "%-12s %-12s %-12s %-10s %-14s %s\n", i.Harness, inst, disp, i.Instalacao, i.Estado, det)
	}
}

// Códigos de saída de `harness atualizar`.
const (
	saidaOcupado    = 2 // há agente/sessão usando o harness
	saidaVoltou     = 3 // o teste falhou e a versão anterior foi restaurada
	saidaVoltaFalha = 4 // o teste falhou e a volta não foi confirmada
)

func newHarnessAtualizarCmd() *cobra.Command {
	var (
		o     versoes.Opcoes
		todos bool
		jsonO bool
	)
	cmd := &cobra.Command{
		Use:     "atualizar <harness>|--todos",
		Aliases: []string{"update", "upgrade"},
		Short:   "Atualiza um harness base pelo mesmo gerenciador que o instalou, testa e volta sozinho se falhar",
		Long: `Fluxo: prévia (--seco) → recusa/espera se algum 'rodar' ou 'serve' usa o harness → instala pelo
mesmo gerenciador que instalou (npm i -g, uv tool, pipx, pip --user, autoatualização do CLI) guardando
a versão anterior → roda 'harness test' REAL com a instância grátis da base (sem ela, --version + --help)
→ se o teste falhar, volta à versão anterior e confirma. Registra o evento harness.atualizado.

Códigos de saída: 0 ok/já na última/prévia · 1 erro · 2 em uso (sem --esperar) · 3 testou, falhou e
VOLTOU · 4 testou, falhou e a volta não foi confirmada.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if todos && len(args) != 0 {
				return fmt.Errorf("--todos não aceita nome")
			}
			if !todos && len(args) != 1 {
				return fmt.Errorf("informe <harness> ou use --todos")
			}
			if todos && o.Para != "" {
				return fmt.Errorf("--para vale para um harness só")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env := versoes.Padrao()
			o.EventLog = os.Getenv("OPENHEINERSS_EVENTOS_LOG")
			if o.EventLog == "" {
				if cwd, err := os.Getwd(); err == nil {
					if cfg, err := config.LoadProject(cwd); err == nil {
						o.EventLog = cfg.EventosLog
					}
				}
			}
			var bases []string
			if todos {
				bases = versoes.Bases()
			} else {
				b := versoes.BaseDe(args[0])
				if b == "" {
					return fmt.Errorf("harness %q não é uma base nem uma instância de base conhecida (bases: %v)", args[0], versoes.Bases())
				}
				bases = []string{b}
			}
			var rels []*versoes.Resultado
			codigo := 0
			for _, b := range bases {
				r, err := env.Atualizar(cmd.Context(), b, o)
				if err != nil {
					return err
				}
				rels = append(rels, r)
				c := codigoAtualizar(r)
				if r.Resultado == versoes.ResSemCLI && !todos {
					c = 1 // pediu um harness específico que não está instalado
				}
				if c > codigo {
					codigo = c
				}
			}
			if jsonO {
				var v interface{} = rels
				if !todos {
					v = rels[0]
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(v); err != nil {
					return err
				}
			} else {
				for _, r := range rels {
					imprimirAtualizarHarness(cmd.OutOrStdout(), r)
				}
			}
			if codigo != 0 {
				return codigoSaida(codigo)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Seco, "seco", false, "Mostra o plano sem alterar nada")
	f.BoolVar(&todos, "todos", false, "Atualiza todas as bases instaladas, uma por vez")
	f.BoolVar(&o.Esperar, "esperar", false, "Espera os agentes/sessões que usam o harness terminarem em vez de recusar")
	f.DurationVar(&o.Espera, "espera", 15*time.Minute, "Quanto esperar com --esperar")
	f.StringVar(&o.Para, "para", "", "Instala esta versão em vez da última (também serve para voltar de propósito)")
	f.BoolVar(&o.Forcar, "forcar", false, "Reinstala mesmo já estando na versão alvo")
	f.BoolVar(&o.SemVolta, "sem-volta", false, "Aceita atualizar métodos sem volta automática (brew, instalador sem versões guardadas)")
	f.BoolVar(&o.SimularFalhaTeste, "simular-falha-teste", false, "Trata o teste pós-instalação como falho (exercita a volta automática)")
	f.DurationVar(&o.TesteTimeout, "timeout", 3*time.Minute, "Tempo máximo do harness test")
	f.BoolVar(&jsonO, "json", false, "Saída em JSON")
	return cmd
}

func codigoAtualizar(r *versoes.Resultado) int {
	switch r.Resultado {
	case versoes.ResOcupado:
		return saidaOcupado
	case versoes.ResVoltou:
		return saidaVoltou
	case versoes.ResVoltaFalhou:
		return saidaVoltaFalha
	case versoes.ResFalhou, versoes.ResDesconhec:
		return 1
	}
	return 0
}

func imprimirAtualizarHarness(w io.Writer, r *versoes.Resultado) {
	titulo := "Atualização de " + r.Harness
	if r.Seco {
		titulo += " (seco: nada foi alterado)"
	}
	fmt.Fprintln(w, titulo)
	if r.Instalacao != "" {
		fmt.Fprintf(w, "  instalado por: %s", r.Instalacao)
		if r.Pacote != "" {
			fmt.Fprintf(w, " (%s)", r.Pacote)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  instalada: %s   disponível: %s\n", valorOu(r.Antes, "-"), valorOu(r.Disponivel, "desconhecida"))
	for _, p := range r.Passos {
		fmt.Fprintf(w, "  • %s\n", p)
	}
	for _, u := range r.Ocupado {
		fmt.Fprintf(w, "  em uso: %s pid %d: %s\n", u.Tipo, u.PID, u.Comando)
	}
	if t := r.Teste; t != nil {
		fmt.Fprintf(w, "  teste (%s%s): ok=%v %s\n", t.Modo, instSufixo(t.Instancia), t.OK, t.Detalhe)
	}
	if r.Motivo != "" {
		fmt.Fprintf(w, "  motivo: %s\n", r.Motivo)
	}
	fmt.Fprintf(w, "  resultado: %s", r.Resultado)
	if r.Resultado == versoes.ResVoltou {
		fmt.Fprintf(w, "   instalou %s, testou, falhou e voltou a %s", r.Tentada, r.Depois)
	} else if r.Depois != "" && !r.Seco {
		fmt.Fprintf(w, "   %s → %s", r.Antes, r.Depois)
	}
	fmt.Fprintln(w)
}

func valorOu(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func instSufixo(i string) string {
	if i == "" {
		return ""
	}
	return ": " + i
}
