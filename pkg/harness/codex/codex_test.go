package codex_test

import (
	"context"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

func TestCodexHarnessRegistration(t *testing.T) {
	hCLI, err := harness.Create("codex", harness.ModeCLI)
	if err != nil {
		t.Fatalf("falha ao criar harness codex modo CLI: %v", err)
	}
	if hCLI.Name() != "codex" {
		t.Errorf("nome esperado 'codex', obteve '%s'", hCLI.Name())
	}
	if hCLI.Mode() != harness.ModeCLI {
		t.Errorf("modo esperado 'cli', obteve '%s'", hCLI.Mode())
	}

	ctx := context.Background()
	res := hCLI.ValidatePrerequisites(ctx)
	if !res.Satisfied {
		t.Logf("pré-requisitos codex: %+v", res)
	}
}
