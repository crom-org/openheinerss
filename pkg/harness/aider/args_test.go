package aider

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestArgsAiderTabela(t *testing.T) {
	for _, prompt := range []string{"faça um arquivo", "--- prompt com hífen"} {
		got := buildArgs(harness.SessionConfig{Model: "modelo-teste"}, prompt)
		want := []string{"--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--no-browser", "--message=" + prompt, "--model", "modelo-teste"}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("prompt %q: %#v", prompt, got)
		}
	}
}

func TestArgsAiderExistemNoHelpReal(t *testing.T) {
	path, err := exec.LookPath("aider")
	if err != nil {
		t.Skip("aider não instalado")
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("aider --help: %v", err)
	}
	help := string(out)
	for _, flag := range []string{"--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--no-browser", "--message", "--model"} {
		if !strings.Contains(help, flag) {
			t.Errorf("aider --help não documenta %s", flag)
		}
	}
}
