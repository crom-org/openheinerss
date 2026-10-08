package opencode

import (
	"context"
	"encoding/json"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAmostraOpenCode(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var tipos []harness.EventType
	for _, linha := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(linha), &raw); err != nil {
			t.Fatal(err)
		}
		for _, ev := range parseOpenCodeEvent(raw, "sess-local") {
			tipos = append(tipos, ev.Type)
		}
	}
	want := []harness.EventType{harness.EventText, harness.EventToolCall, harness.EventToolResult, harness.EventUsage}
	if len(tipos) != len(want) {
		t.Fatalf("tipos: %v", tipos)
	}
	for i := range want {
		if tipos[i] != want[i] {
			t.Fatalf("tipo %d: %s", i, tipos[i])
		}
	}
	usage := parseOpenCodeEvent(map[string]interface{}{"type": "step_finish", "part": map[string]interface{}{"tokens": map[string]interface{}{"total": float64(12), "input": float64(10), "output": float64(2)}}}, "s")[0].Payload.(protocol.UsageParams)
	if usage.TotalTokens != 12 {
		t.Fatalf("uso: %#v", usage)
	}
}

func TestOpenCodeCLIFluxoCompleto(t *testing.T) {
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-opencode.sh"), filepath.Join(d, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	o := NewOpenCodeHarness(harness.ModeCLI)
	if !o.ValidatePrerequisites(context.Background()).Satisfied {
		t.Fatal("opencode ausente")
	}
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "o1", CWD: t.TempDir(), Model: "deepseek", Env: map[string]string{"OPENCODE_TEST": "ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.SendPrompt(context.Background(), "faça", nil); err != nil {
		t.Fatal(err)
	}
	text, done := false, false
	deadline := time.After(2 * time.Second)
	for !done {
		select {
		case ev := <-o.Events():
			text = text || ev.Type == harness.EventText
			done = ev.Type == harness.EventComplete
		case <-deadline:
			t.Fatal("timeout")
		}
	}
	if !text {
		t.Fatal("sem texto")
	}
	if o.ResumeID() != "ses_fake" {
		t.Fatalf("sessionID não capturado: %q", o.ResumeID())
	}
	if err := o.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := o.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeLeitorEErro(t *testing.T) {
	o := NewOpenCodeHarness(harness.ModeCLI)
	o.cfg.SessionID = "r"
	o.readEvents(strings.NewReader("um\n\ndois\n"))
	if len(o.events) != 3 {
		t.Fatalf("eventos: %d", len(o.events))
	}
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-opencode.sh"), filepath.Join(d, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FAIL", "1")
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "e", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := o.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-o.Events():
			if ev.Type == harness.EventComplete {
				return
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}

func TestOpenCodeStderrInformativoEProcessError(t *testing.T) {
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-opencode.sh"), filepath.Join(d, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_STDERR", "1")
	o := NewOpenCodeHarness(harness.ModeCLI)
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "ok", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := o.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	var info, complete bool
	// Espera o estado complete; o timer é apenas uma rede de segurança para não travar a suíte.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for !complete {
		select {
		case ev := <-o.Events():
			if ev.Type == harness.EventText {
				if p, ok := ev.Payload.(protocol.TextParams); ok && strings.Contains(p.Delta, "stderr") {
					info = true
				}
			}
			complete = ev.Type == harness.EventComplete
		case <-timer.C:
			t.Fatal("OpenCode não alcançou o estado complete")
		}
	}
	if !info {
		t.Fatal("stderr de sucesso não virou texto informativo")
	}
	t.Setenv("FAKE_FAIL", "1")
	o = NewOpenCodeHarness(harness.ModeCLI)
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "falha", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := o.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	complete = false
	var failed bool
	// A falha esperada é observada pelo par EventError + complete/process_error.
	timer = time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for !complete {
		select {
		case ev := <-o.Events():
			if ev.Type == harness.EventError {
				failed = true
			}
			if ev.Type == harness.EventComplete {
				if p, ok := ev.Payload.(protocol.CompleteParams); ok && p.Reason != "process_error" {
					t.Fatalf("fim: %s", p.Reason)
				}
				complete = true
			}
		case <-timer.C:
			t.Fatal("OpenCode não alcançou o estado de falha")
		}
	}
	if !failed {
		t.Fatal("falha sem EventError")
	}
}
