package agy

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestArgsAgyProtegemPrompt(t *testing.T) {
	cases := []struct {
		prompt string
		want   []string
	}{
		{"faça um arquivo", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "-p=faça um arquivo"}},
		{"--- comece por hífen", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "-p=--- comece por hífen"}},
	}
	for _, tc := range cases {
		got := buildArgs(harness.SessionConfig{Model: "gemini-teste"}, tc.prompt)
		if strings.Join(got, "\x00") != strings.Join(append([]string{"--model", "gemini-teste"}, tc.want...), "\x00") {
			t.Fatalf("prompt %q: args=%#v", tc.prompt, got)
		}
	}
}

func TestArgsAgyExistemNoHelpReal(t *testing.T) {
	path, err := exec.LookPath("agy")
	if err != nil {
		t.Skip("agy não instalado")
	}
	out, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("agy --help: %v", err)
	}
	help := string(out)
	for _, flag := range []string{"--dangerously-skip-permissions", "--output-format", "-p"} {
		if !strings.Contains(help, flag) {
			t.Errorf("agy --help não documenta %s", flag)
		}
	}
}
