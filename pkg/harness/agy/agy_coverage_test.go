package agy

import (
	"context"
	"github.com/crom-org/openheinerss/pkg/harness"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAGYCLIFluxoCompleto(t *testing.T) {
	dir := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-agy.sh"), filepath.Join(dir, "agy")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := NewAGYHarness(harness.ModeCLI)
	if !a.ValidatePrerequisites(context.Background()).Satisfied {
		t.Fatal("agy ausente")
	}
	if err := a.Start(context.Background(), harness.SessionConfig{SessionID: "a1", CWD: t.TempDir(), Model: "gemini", Env: map[string]string{"AGY_TEST": "ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "faça", nil); err != nil {
		t.Fatal(err)
	}
	text, done := false, false
	deadline := time.After(2 * time.Second)
	for !done {
		select {
		case ev := <-a.Events():
			text = text || ev.Type == harness.EventText
			done = ev.Type == harness.EventComplete
		case <-deadline:
			t.Fatal("timeout")
		}
	}
	if !text {
		t.Fatal("sem texto")
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAGYLeitorECaminhos(t *testing.T) {
	a := NewAGYHarness(harness.ModeCLI)
	a.cfg.SessionID = "r1"
	a.readEvents(strings.NewReader("uma\n\nduas\n"))
	if len(a.events) != 3 {
		t.Fatalf("eventos: %d", len(a.events))
	}
	if err := a.RespondPermission(context.Background(), "", true, ""); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-agy.sh"), filepath.Join(d, "agy")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FAIL", "1")
	if err := a.Start(context.Background(), harness.SessionConfig{SessionID: "e", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-a.Events():
			if ev.Type == harness.EventComplete {
				return
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}
