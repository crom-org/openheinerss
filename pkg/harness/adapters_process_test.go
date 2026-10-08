package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	_ "github.com/crom-org/openheinerss/pkg/harness/agy"
	_ "github.com/crom-org/openheinerss/pkg/harness/aider"
	_ "github.com/crom-org/openheinerss/pkg/harness/claudecode"
	_ "github.com/crom-org/openheinerss/pkg/harness/opencode"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestAdaptadoresCLIComBinariosFalsos(t *testing.T) {
	dir := t.TempDir()
	for _, nome := range []string{"agy", "aider", "claude", "opencode"} {
		path := filepath.Join(dir, nome)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'resposta falsa\\n'\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, nome := range []string{"agy", "aider", "claude-code", "opencode"} {
		t.Run(nome, func(t *testing.T) {
			h, err := harness.Create(nome, harness.ModeCLI)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := h.Start(ctx, harness.SessionConfig{SessionID: "fake-" + nome, CWD: t.TempDir()}); err != nil {
				t.Fatal(err)
			}
			if err := h.SendPrompt(ctx, "teste", nil); err != nil {
				t.Fatal(err)
			}
			completo := false
			for !completo {
				select {
				case ev := <-h.Events():
					completo = ev.Type == harness.EventComplete
				case <-ctx.Done():
					t.Fatal("adaptador não concluiu")
				}
			}
			if err := h.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
	_ = protocol.EventAgentText
}

func TestAdaptadoresDetectamPrerequisitoAusente(t *testing.T) {
	old := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	for _, nome := range []string{"agy", "aider", "claude-code", "opencode"} {
		h, err := harness.Create(nome, harness.ModeCLI)
		if err != nil {
			t.Fatal(err)
		}
		if got := h.ValidatePrerequisites(context.Background()); got.Satisfied || len(got.MissingItems) != 1 {
			t.Errorf("%s: %+v", nome, got)
		}
	}
	t.Setenv("PATH", old)
}
