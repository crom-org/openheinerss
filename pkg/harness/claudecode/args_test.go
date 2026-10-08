package claudecode

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestArgsClaudeTabela(t *testing.T) {
	for _, prompt := range []string{"faça um arquivo", "--- prompt com hífen"} {
		got := buildCLIArgs(harness.SessionConfig{Model: "sonnet"}, "sess-1", "bypassPermissions", prompt)
		want := []string{"--resume", "sess-1", "--print", "--output-format", "stream-json", "--verbose", "--model", "sonnet", "--permission-mode", "bypassPermissions", "--", prompt}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("prompt %q: %#v", prompt, got)
		}
	}
}

func TestArgsClaudeExistemNoHelpReal(t *testing.T) {
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude não instalado")
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("claude --help: %v", err)
	}
	help := string(out)
	for _, flag := range []string{"--print", "--output-format", "--verbose", "--model", "--resume", "--permission-mode"} {
		if !strings.Contains(help, flag) {
			t.Errorf("claude --help não documenta %s", flag)
		}
	}
}
