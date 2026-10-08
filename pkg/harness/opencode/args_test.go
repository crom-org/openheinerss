package opencode

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestArgsOpenCodeTabela(t *testing.T) {
	for _, prompt := range []string{"faça um arquivo", "--- prompt com hífen"} {
		got := buildArgs(harness.SessionConfig{Model: "openai/modelo"}, "sess-1", prompt)
		want := []string{"run", "--format", "json", "--session", "sess-1", "-m", "openai/modelo", "--", prompt}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("prompt %q: %#v", prompt, got)
		}
	}
}

func TestArgsOpenCodeExistemNoHelpReal(t *testing.T) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode não instalado")
	}
	out, err := exec.Command(path, "run", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("opencode run --help: %v", err)
	}
	help := string(out)
	for _, flag := range []string{"--format", "--session", "--model"} {
		if !strings.Contains(help, flag) {
			t.Errorf("opencode run --help não documenta %s", flag)
		}
	}
}
