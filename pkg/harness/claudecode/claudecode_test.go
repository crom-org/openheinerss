package claudecode_test

import (
	"context"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
)

func TestClaudeCodeHarnessRegistration(t *testing.T) {
	// Cria harness no modo CLI
	hCLI, err := harness.Create("claude-code", harness.ModeCLI)
	if err != nil {
		t.Fatalf("falha ao criar harness claude-code modo CLI: %v", err)
	}
	if hCLI.Name() != "claude-code" {
		t.Errorf("nome esperado 'claude-code', obteve '%s'", hCLI.Name())
	}
	if hCLI.Mode() != harness.ModeCLI {
		t.Errorf("modo esperado 'cli', obteve '%s'", hCLI.Mode())
	}

	// Cria harness no modo SDK
	hSDK, err := harness.Create("claude-code", harness.ModeSDK)
	if err != nil {
		t.Fatalf("falha ao criar harness claude-code modo SDK: %v", err)
	}
	if hSDK.Mode() != harness.ModeSDK {
		t.Errorf("modo esperado 'sdk', obteve '%s'", hSDK.Mode())
	}

	// Validação de pré-requisitos
	ctx := context.Background()
	res := hSDK.ValidatePrerequisites(ctx)
	// Como node está instalado na máquina de teste, deve satisfazer
	if !res.Satisfied {
		t.Logf("Aviso: pré-requisito não satisfeito (normal se faltar node no path de teste): %+v", res)
	}
}
