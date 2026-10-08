package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

func TestCustomPonteArgsSlashEStderrRaw(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "argv.txt")
	script := filepath.Join(dir, "fake.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "\nread prompt\necho \"stdin=$prompt\" >> " + out +
		"\necho 'aviso stderr' >&2\nprintf '%s\\n' '{\"type\":\"desconhecido\",\"x\":1}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "ponte-custom-" + filepath.Base(dir)
	if err := RegisterCustom(CustomSpec{Name: name, Command: script, Args: []string{"--do-spec"}, Prompt: "stdin"}); err != nil {
		t.Fatal(err)
	}
	h, _ := Create(name, ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := map[string]interface{}{OptionHarnessArgs: []interface{}{"--extra", "valor com espaço"}}
	if err := h.Start(ctx, SessionConfig{SessionID: "s", CWD: dir, Options: opts}); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := h.SendPrompt(ctx, "/x a b", nil); err != nil {
		t.Fatal(err)
	}
	var stderrRaw, stdoutRaw bool
	deadline := time.After(5 * time.Second)
	for done := false; !done; {
		select {
		case e := <-h.Events():
			if r, ok := e.Payload.(protocol.RawParams); ok && e.Type == EventRaw {
				stderrRaw = stderrRaw || (r.Stream == "stderr" && r.Line == "aviso stderr")
				stdoutRaw = stdoutRaw || (r.Stream == "stdout" && strings.Contains(r.Line, "desconhecido"))
			}
			done = e.Type == EventComplete
		case <-deadline:
			t.Fatal("sem complete")
		}
	}
	b, _ := os.ReadFile(out)
	if got, want := string(b), "--do-spec\n--extra\nvalor com espaço\nstdin=/x a b\n"; got != want {
		t.Fatalf("argv/stdin %q, esperado %q", got, want)
	}
	if !stderrRaw || !stdoutRaw {
		t.Fatalf("raw stderr=%v stdout=%v", stderrRaw, stdoutRaw)
	}
}
