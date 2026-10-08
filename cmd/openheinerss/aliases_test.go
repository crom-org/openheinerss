package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAliasesDeComandosDoCLI(t *testing.T) {
	tests := []struct {
		path  string
		alias string
	}{
		{"serve", "servir"},
		{"doctor", "diagnostico"},
		{"init", "inicializar"},
		{"run", "executar"},
		{"rodar", "launch"},
		{"rodar", "dispatch"},
		{"limites", "limits"},
		{"motores", "engines"},
		{"docs", "documentacao"},
		{"version", "versao"},
		{"agentes", "agents"},
		{"agentes listar", "agentes list"},
		{"agentes ver", "agentes show"},
		{"agentes parar", "agentes stop"},
		{"agentes desfazer", "agentes undo"},
		{"harness list", "harness listar"},
		{"harness add", "harness adicionar"},
		{"harness test", "harness testar"},
		{"mcp list", "mcp listar"},
		{"mcp add", "mcp adicionar"},
	}

	root := newRootCmd()
	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			canonical, _, err := root.Find(strings.Fields(tc.path))
			if err != nil {
				t.Fatalf("comando principal %q: %v", tc.path, err)
			}
			aliased, _, err := root.Find(strings.Fields(tc.alias))
			if err != nil {
				t.Fatalf("apelido %q não resolveu: %v", tc.alias, err)
			}
			if aliased != canonical {
				t.Fatalf("%q resolveu %q; esperado %q", tc.alias, aliased.CommandPath(), canonical.CommandPath())
			}
		})
	}
}

func TestAliasesDeFlagsCompartilhamVariavel(t *testing.T) {
	tests := []struct {
		nome string
		cmd  func() *cobra.Command
		pt   string
		en   string
		val  string
	}{
		{"serve porta", newServeCmd, "porta", "port", "4911"},
		{"serve host", newServeCmd, "hospedeiro", "host", "exemplo.local"},
		{"serve agentes", newServeCmd, "max-agentes", "max-agents", "3"},
		{"run papel", newRunCmd, "papel", "role", "revisor"},
		{"run motor", newRunCmd, "motor", "engine", "codex"},
		{"run modo", newRunCmd, "modo", "mode", "cli"},
		{"run provedor", newRunCmd, "provedor", "provider", "openrouter"},
		{"run modelo", newRunCmd, "modelo", "model", "gpt"},
		{"run esforço", newRunCmd, "esforco", "effort", "high"},
		{"run retomar", newRunCmd, "retomar", "resume", "sessao-1"},
		{"rodar modelo", newRodarCmd, "modelo", "model", "gpt"},
		{"rodar esforço", newRodarCmd, "esforco", "effort", "high"},
		{"rodar retomar", newRodarCmd, "retomar", "resume", "true"},
		{"rodar pasta", newRodarCmd, "pasta-agentes", "agents-dir", ".agentes"},
		{"rodar branch", newRodarCmd, "branch-base", "base-branch", "main"},
		{"rodar carga", newRodarCmd, "carga-maxima", "max-load", "2.5"},
		{"rodar agentes", newRodarCmd, "max-agentes", "max-agents", "3"},
		{"rodar tentativas", newRodarCmd, "tentativas", "retries", "4"},
		{"rodar cota", newRodarCmd, "cota-max", "quota-max", "95"},
		{"rodar carga abaixo", newRodarCmd, "quando-carga-abaixo", "when-load-below", "10"},
		{"rodar conta", newRodarCmd, "conta", "account", "conta2"},
		{"rodar seco", newRodarCmd, "seco", "dry-run", "true"},
		{"rodar regras", newRodarCmd, "regras", "rules", "regras.md"},
		{"rodar sem regras", newRodarCmd, "sem-regras", "no-rules", "true"},
		{"rodar chaves", newRodarCmd, "arquivo-chaves", "keys-file", "chaves.env"},
		{"harness test todos", func() *cobra.Command { return newHarnessCmd().Commands()[2] }, "todos", "all", "true"},
		{"harness test pular", func() *cobra.Command { return newHarnessCmd().Commands()[2] }, "pular", "skip", "codex"},
		{"harness test retomar", func() *cobra.Command { return newHarnessCmd().Commands()[2] }, "retomar", "resume", "true"},
		{"harness test modo", func() *cobra.Command { return newHarnessCmd().Commands()[2] }, "modo", "mode", "sdk"},
	}

	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			cmd := tc.cmd()
			if err := cmd.ParseFlags([]string{"--" + tc.pt, tc.val}); err != nil {
				t.Fatalf("parse de --%s: %v", tc.pt, err)
			}
			portugues := cmd.Flag(tc.pt)
			ingles := cmd.Flag(tc.en)
			if portugues == nil || ingles == nil {
				t.Fatalf("flags não registradas: --%s/--%s", tc.pt, tc.en)
			}
			if portugues.Value.String() != ingles.Value.String() {
				t.Fatalf("--%s=%q e --%s=%q não compartilham valor", tc.pt, portugues.Value.String(), tc.en, ingles.Value.String())
			}
		})
	}
}
