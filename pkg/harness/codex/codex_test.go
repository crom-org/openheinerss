package codex_test

import (
	"context"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/codex"
)

func TestCodexHarnessRegistration(t *testing.T) {
	hAPI, err := harness.Create("codex", harness.ModeAPI)
	if err != nil {
		t.Fatalf("falha ao criar harness codex modo API: %v", err)
	}
	if hAPI.Name() != "codex" {
		t.Errorf("nome esperado 'codex', obteve '%s'", hAPI.Name())
	}
	if hAPI.Mode() != harness.ModeAPI {
		t.Errorf("modo esperado 'api', obteve '%s'", hAPI.Mode())
	}

	ctx := context.Background()
	res := hAPI.ValidatePrerequisites(ctx)
	if !res.Satisfied {
		t.Logf("pré-requisitos codex: %+v", res)
	}
}
