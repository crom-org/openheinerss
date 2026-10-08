package aider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestAiderCLIFluxoCompleto(t *testing.T) {
	dir := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-aider.sh"), filepath.Join(dir, "aider")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Chave falsa: o teste não pode depender das chaves da máquina (no CI não há nenhuma).
	t.Setenv("OPENROUTER_API_KEY", "teste")
	a := NewAiderHarness(harness.ModeCLI)
	if got := a.ValidatePrerequisites(context.Background()); !got.Satisfied {
		t.Fatalf("pré-requisito: %+v", got)
	}
	if err := a.Start(context.Background(), harness.SessionConfig{SessionID: "a1", CWD: t.TempDir(), Model: "sonnet", Env: map[string]string{"AIDER_TEST": "ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "faça", nil); err != nil {
		t.Fatal(err)
	}
	seenText, seenComplete := false, false
	deadline := time.After(2 * time.Second)
	for !seenComplete {
		select {
		case ev := <-a.Events():
			seenText = seenText || ev.Type == harness.EventText
			seenComplete = ev.Type == harness.EventComplete
		case <-deadline:
			t.Fatal("timeout")
		}
	}
	if !seenText {
		t.Fatal("sem texto")
	}
	if err := a.RespondPermission(context.Background(), "r", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "depois", nil); err == nil {
		t.Fatal("prompt após stop deveria falhar")
	}
}

func TestAiderLeitorEProcessoComErro(t *testing.T) {
	a := NewAiderHarness(harness.ModeCLI)
	a.cfg.SessionID = "r1"
	a.readEvents(strings.NewReader("linha um\n\nlinha dois\n"))
	if len(a.events) != 3 {
		t.Fatalf("eventos do leitor: %d", len(a.events))
	}
	dir := t.TempDir()
	root, _ := os.Getwd()
	if err := os.Symlink(filepath.Join(root, "testdata", "fake-aider.sh"), filepath.Join(dir, "aider")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Chave falsa: o teste não pode depender das chaves da máquina (no CI não há nenhuma).
	t.Setenv("OPENROUTER_API_KEY", "teste")
	t.Setenv("FAKE_FAIL", "1")
	for len(a.events) > 0 {
		<-a.events
	}
	if err := a.Start(context.Background(), harness.SessionConfig{SessionID: "err", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := a.SendPrompt(context.Background(), "falhe", nil); err != nil {
		t.Fatal(err)
	}
	seenError := false
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-a.Events():
			if ev.Type == harness.EventError {
				seenError = true
			}
			if ev.Type == harness.EventComplete {
				if !seenError {
					t.Fatal("processo com erro não gerou erro")
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}
