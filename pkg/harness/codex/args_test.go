package codex

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestArgsCodexTabela(t *testing.T) {
	for _, prompt := range []string{"faça um arquivo", "--- prompt com hífen"} {
		got := buildExecArgs(harness.SessionConfig{Model: "o3", Options: map[string]interface{}{"effort": "high"}}, "", prompt)
		want := []string{"exec", "--json", "-m", "o3", "-c", "model_reasoning_effort=high", "--dangerously-bypass-approvals-and-sandbox", "--", prompt}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("prompt %q: %#v", prompt, got)
		}
	}
}

func TestArgsCodexExistemNoHelpReal(t *testing.T) {
	path, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex não instalado")
	}
	out, err := exec.Command(path, "exec", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("codex exec --help: %v", err)
	}
	help := string(out)
	for _, flag := range []string{"--json", "--model", "--config", "--dangerously-bypass-approvals-and-sandbox"} {
		if !strings.Contains(help, flag) {
			t.Errorf("codex exec --help não documenta %s", flag)
		}
	}
}
