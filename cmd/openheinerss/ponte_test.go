package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHarnessArgIntactoNaOrdemMesmoMisturandoAlias(t *testing.T) {
	var got []string
	cmd := &cobra.Command{Use: "x"}
	addHarnessArgFlags(cmd, &got)
	if err := cmd.ParseFlags([]string{"--harness-arg", "--x=a,b", "--arg", "/compact", "--harness-arg=--y", "--arg", "c d"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "--x=a,b|/compact|--y|c d" {
		t.Fatalf("args = %q", got)
	}
}

func TestRunERodarTemHarnessArg(t *testing.T) {
	for name, c := range map[string]*cobra.Command{"run": newRunCmd(), "rodar": newRodarCmd()} {
		for _, f := range []string{"harness-arg", "arg"} {
			if c.Flags().Lookup(f) == nil {
				t.Fatalf("%s sem --%s", name, f)
			}
		}
	}
	if newRunCmd().Flags().Lookup("interativo") == nil {
		t.Fatal("run sem --interativo")
	}
}
