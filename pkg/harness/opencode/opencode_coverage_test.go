package opencode

import (
	"context"
	"github.com/crom-org/openheinerss/pkg/harness"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
