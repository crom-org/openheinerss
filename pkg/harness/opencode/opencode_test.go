package opencode_test

import (
	"context"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
)

func TestOpenCodeHarnessRegistration(t *testing.T) {
	hCLI, err := harness.Create("opencode", harness.ModeCLI)
	if err != nil {
		t.Fatalf("falha ao criar harness opencode: %v", err)
	}
	if hCLI.Name() != "opencode" {
		t.Errorf("nome esperado 'opencode', obteve '%s'", hCLI.Name())
	}
	if hCLI.Mode() != harness.ModeCLI {
		t.Errorf("modo esperado 'cli', obteve '%s'", hCLI.Mode())
	}

	ctx := context.Background()
	res := hCLI.ValidatePrerequisites(ctx)
	if !res.Satisfied {
		t.Logf("Aviso: pré-requisito não satisfeito: %+v", res)
	}
}
