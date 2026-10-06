package agy_test

import (
	"context"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
)

func TestAGYHarnessRegistration(t *testing.T) {
	hCLI, err := harness.Create("agy", harness.ModeCLI)
	if err != nil {
		t.Fatalf("falha ao criar harness agy: %v", err)
	}
	if hCLI.Name() != "agy" {
		t.Errorf("nome esperado 'agy', obteve '%s'", hCLI.Name())
	}
	if hCLI.Mode() != harness.ModeCLI {
		t.Errorf("modo esperado 'cli', obteve '%s'", hCLI.Mode())
	}

	ctx := context.Background()
	res := hCLI.ValidatePrerequisites(ctx)
	if !res.Satisfied {
		t.Logf("pré-requisito agy: %+v", res)
	}
}
