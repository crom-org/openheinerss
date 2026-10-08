package agy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func iniciar(t *testing.T, opts map[string]interface{}) (*AGYHarness, string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_OUT\"\necho 'resposta do agy'\necho 'aviso stderr' >&2\n"
	if err := os.WriteFile(filepath.Join(dir, "agy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argv := filepath.Join(t.TempDir(), "argv.txt")
	a := NewAGYHarness(harness.ModeCLI)
	cfg := harness.SessionConfig{SessionID: "s1", CWD: t.TempDir(), Model: "gemini-x", Env: map[string]string{"FAKE_OUT": argv}, Options: opts}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop() })
	return a, argv
}

func rodar(t *testing.T, a *AGYHarness, text string) []harness.Event {
	t.Helper()
	if err := a.SendPrompt(context.Background(), text, nil); err != nil {
		t.Fatal(err)
	}
	var out []harness.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e := <-a.Events():
			out = append(out, e)
			if e.Type == harness.EventComplete {
				return out
			}
		case <-timeout:
			t.Fatalf("sem complete: %v", out)
		}
	}
}

func lerArgv(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(b), "\n")
}

func TestPonteAgyArgsNaOrdemSlashLiteralERaw(t *testing.T) {
	a, argv := iniciar(t, map[string]interface{}{
		"harness_args": []interface{}{"--sandbox", "--log-file", "/tmp/x.log"},
		"effort":       "xhigh", "agent": "rev", "continue": true, "add_dirs": []string{"/srv/a"},
	})
	events := rodar(t, a, "/skill-x args")
	got := lerArgv(t, argv)
	want := strings.Join([]string{"--model", "gemini-x", "--dangerously-skip-permissions", "--output-format", "stream-json",
		"--effort", "xhigh", "--agent", "rev", "--continue", "--add-dir", "/srv/a", "--sandbox", "--log-file", "/tmp/x.log", "-p=/skill-x args"}, "\n")
	if got != want {
		t.Fatalf("argv:\n%s\n--- esperado:\n%s", got, want)
	}
	var stderr bool
	for _, e := range events {
		if r, ok := e.Payload.(protocol.RawParams); ok && e.Type == harness.EventRaw && r.Stream == "stderr" && r.Line == "aviso stderr" {
			stderr = true
		}
	}
	if !stderr {
		t.Fatalf("stderr não chegou como raw: %v", events)
	}
}

func TestPonteAgyTraducoes(t *testing.T) {
	a, argv := iniciar(t, map[string]interface{}{"continue": true})
	rodar(t, a, "/model gemini-y")
	rodar(t, a, "/effort low")
	rodar(t, a, "/new")
	rodar(t, a, "oi")
	got := lerArgv(t, argv)
	if !strings.Contains(got, "--model\ngemini-y") || !strings.Contains(got, "--effort\nlow") || strings.Contains(got, "--continue") {
		t.Fatalf("argv: %s", got)
	}
	if err := a.SendPrompt(context.Background(), "/exit", nil); err == nil || !strings.Contains(err.Error(), "não aceita /exit") {
		t.Fatalf("esperava NoEquivalent: %v", err)
	}
}
