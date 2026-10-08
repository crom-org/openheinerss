package aider

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

func iniciar(t *testing.T, opts map[string]interface{}) (*AiderHarness, string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_OUT\"\necho 'resposta do aider'\necho 'aviso stderr' >&2\n"
	if err := os.WriteFile(filepath.Join(dir, "aider"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argv := filepath.Join(t.TempDir(), "argv.txt")
	a := NewAiderHarness(harness.ModeCLI)
	cfg := harness.SessionConfig{SessionID: "s1", CWD: t.TempDir(), Model: "ollama/x", Env: map[string]string{"FAKE_OUT": argv}, Options: opts}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop() })
	return a, argv
}

func rodar(t *testing.T, a *AiderHarness, text string) []harness.Event {
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

func TestPonteAiderArgsNaOrdemSlashLiteralERaw(t *testing.T) {
	a, argv := iniciar(t, map[string]interface{}{
		"harness_args": []interface{}{"--auto-commits", "--map-tokens", "0"},
		"effort":       "high", "files": []string{"a.go"}, "read_files": []string{"LEIAME.md"}, "continue": true,
	})
	events := rodar(t, a, "/ask o que é isso?")
	got := lerArgv(t, argv)
	want := strings.Join([]string{"--yes-always", "--no-pretty", "--no-stream", "--no-check-update", "--no-analytics", "--no-show-model-warnings", "--no-browser",
		"--model", "ollama/x", "--reasoning-effort", "high", "--read", "LEIAME.md", "--file", "a.go", "--restore-chat-history",
		"--auto-commits", "--map-tokens", "0", "--message=/ask o que é isso?"}, "\n")
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

func TestPonteAiderModelEEffortViramFlags(t *testing.T) {
	a, argv := iniciar(t, nil)
	if ev := rodar(t, a, "/model ollama/outro"); len(ev) != 2 {
		t.Fatalf("eventos: %v", ev)
	}
	rodar(t, a, "/reasoning-effort low")
	rodar(t, a, "oi")
	got := lerArgv(t, argv)
	if !strings.Contains(got, "--model\nollama/outro") || !strings.Contains(got, "--reasoning-effort\nlow") {
		t.Fatalf("argv: %s", got)
	}
	if err := a.SendPrompt(context.Background(), "/model", nil); err == nil || !strings.Contains(err.Error(), "não aceita") {
		t.Fatalf("esperava NoEquivalent: %v", err)
	}
}
