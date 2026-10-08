package claudecode_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/harness/claudecode"
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

func TestPrerequisitosSDKUsamEnvDaInstancia(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("sem node")
	}
	sdk := t.TempDir()
	if err := os.WriteFile(filepath.Join(sdk, "package.json"), []byte(`{"name":"falso-sdk","main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "index.js"), []byte("module.exports = {}"), 0644); err != nil {
		t.Fatal(err)
	}
	h := claudecode.NewClaudeCodeHarness(harness.ModeSDK)
	r := h.ValidatePrerequisitesEnv(context.Background(), map[string]string{"OPENHEINERSS_CLAUDE_SDK_PATH": sdk})
	if !r.Satisfied {
		t.Fatalf("o caminho do SDK vindo da instância deveria satisfazer o pré-requisito: %+v", r)
	}
}
