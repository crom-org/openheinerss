package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestParseJSONLSamples(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var got []harness.Event
	for _, line := range splitLines(data) {
		got = append(got, parseJSONL(line, "sess-test")...)
	}
	if len(got) != 4 {
		t.Fatalf("eventos esperados: 4, obtidos: %d", len(got))
	}
	if got[0].Type != harness.EventToolCall || got[1].Type != harness.EventToolResult || got[2].Type != harness.EventText || got[3].Type != harness.EventUsage {
		t.Fatalf("tipos inesperados: %+v", got)
	}
	usage, ok := got[3].Payload.(protocol.UsageParams)
	if !ok || usage.TotalTokens != 16 {
		t.Fatalf("uso inesperado: %#v", got[3].Payload)
	}
	bad, err := os.ReadFile(filepath.Join("testdata", "error.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if parseJSONL(string(bad[:len(bad)-1]), "sess-test")[0].Type != harness.EventError {
		t.Fatal("erro JSONL não convertido")
	}
	nested := parseJSONL(`{"type":"turn.failed","error":{"message":"modelo não suportado"}}`, "sess-test")
	if len(nested) != 1 || nested[0].Payload.(protocol.ErrorParams).Message != "modelo não suportado" {
		t.Fatalf("erro aninhado perdido: %#v", nested)
	}
}

func TestBuildExecArgsAndEnv(t *testing.T) {
	cfg := harness.SessionConfig{Model: "o3", Options: map[string]interface{}{"effort": "high"}, Env: map[string]string{"CODEX_HOME": "/tmp/codex-compartilhado"}}
	args := buildExecArgs(cfg, "", "responda só OK")
	expected := []string{"exec", "--json", "-m", "o3", "-c", "model_reasoning_effort=high", "--dangerously-bypass-approvals-and-sandbox", "--", "responda só OK"}
	if len(args) != len(expected) {
		t.Fatalf("args: %#v", args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("args[%d]: %q != %q", i, args[i], expected[i])
		}
	}
	resume := buildExecArgs(cfg, "thread-1", "continua")
	if resume[1] != "resume" || resume[len(resume)-3] != "thread-1" || resume[len(resume)-2] != "--" {
		t.Fatalf("retomada: %#v", resume)
	}
	env := mergedEnv(cfg.Env)
	found := false
	for _, item := range env {
		if item == "CODEX_HOME=/tmp/codex-compartilhado" {
			found = true
		}
	}
	if !found {
		t.Fatal("CODEX_HOME não foi propagado")
	}
}

func TestFakeCodexProcess(t *testing.T) {
	path, _ := os.Getwd()
	fakeDir := t.TempDir()
	if err := os.Symlink(filepath.Join(path, "testdata", "fake-codex.sh"), filepath.Join(fakeDir, "codex")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := NewCodexHarness(harness.ModeCLI)
	if err := c.Start(context.Background(), harness.SessionConfig{SessionID: "sess-fake", Model: "o3"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SendPrompt(context.Background(), "responda só OK", nil); err != nil {
		t.Fatal(err)
	}
	seenText, seenUsage := false, false
	// O estado esperado é o evento complete; o prazo só evita um teste preso.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case evt := <-c.Events():
			switch evt.Type {
			case harness.EventText:
				seenText = true
			case harness.EventUsage:
				seenUsage = true
			case harness.EventComplete:
				goto concluido
			}
		case <-timer.C:
			t.Fatal("fake codex não concluiu o estado esperado")
		}
	}
concluido:
	if !seenText || !seenUsage {
		t.Fatalf("eventos recebidos: texto=%v uso=%v", seenText, seenUsage)
	}
}

func splitLines(data []byte) []string {
	var lines []string
	for len(data) > 0 {
		i := 0
		for i < len(data) && data[i] != '\n' {
			i++
		}
		if i > 0 {
			var v map[string]interface{}
			if json.Unmarshal(data[:i], &v) == nil {
				lines = append(lines, string(data[:i]))
			}
		}
		if i == len(data) {
			break
		}
		data = data[i+1:]
	}
	return lines
}
