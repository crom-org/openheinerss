package cco

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestCCOComandoArgumentosEAmbiente(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "invocacao.log")
	script := filepath.Join(dir, "cco")
	content := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$CCO_TEST_LOG\"\nprintf 'modelo=%s\\n' \"$CCO_MODEL\" >> \"$CCO_TEST_LOG\"\nprintf 'ok\\n'\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CCO_TEST_LOG", log)
	c := New(harness.ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx, harness.SessionConfig{SessionID: "s1", CWD: dir, Provider: "openrouter", Model: "modelo-gratis", Env: map[string]string{"CCO_TEST_LOG": log}, Options: map[string]interface{}{"effort": "low"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.SendPrompt(ctx, "teste", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-c.Events():
			if evt.Type == harness.EventComplete {
				data, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				got := string(data)
				for _, want := range []string{"--provider openrouter", "--effort low", "-p teste", "modelo=modelo-gratis"} {
					if !strings.Contains(got, want) {
						t.Fatalf("invocação não contém %q: %s", want, got)
					}
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout esperando CCO falso")
		}
	}
}
