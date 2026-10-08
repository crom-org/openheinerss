package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// fakeOpencode grava o argv (um por linha) em argv.txt e imprime uma linha mapeada, uma sem mapeamento
// (JSON de tipo desconhecido) e uma linha no stderr.
func fakeOpencode(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_OUT\"\n" +
		`echo '{"type":"text","sessionID":"ses_1","part":{"text":"oi"}}'` + "\n" +
		`echo '{"type":"step_start","part":{}}'` + "\n" +
		"echo 'aviso do stderr' >&2\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(t.TempDir(), "argv.txt")
}

func rodar(t *testing.T, o *OpenCodeHarness, text string) []harness.Event {
	t.Helper()
	if err := o.SendPrompt(context.Background(), text, nil); err != nil {
		t.Fatal(err)
	}
	var out []harness.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e := <-o.Events():
			out = append(out, e)
			if e.Type == harness.EventComplete {
				return out
			}
		case <-timeout:
			t.Fatalf("sem complete: %v", out)
		}
	}
}

func iniciar(t *testing.T, opts map[string]interface{}) (*OpenCodeHarness, string) {
	argv := fakeOpencode(t)
	o := NewOpenCodeHarness(harness.ModeCLI)
	if err := o.Start(context.Background(), harness.SessionConfig{SessionID: "s1", CWD: t.TempDir(), Model: "p/m", Env: map[string]string{"FAKE_OUT": argv}, Options: opts}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Stop() })
	return o, argv
}

func lerArgv(t *testing.T, path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func TestPonteOpenCodeArgsNaOrdemERaw(t *testing.T) {
	o, argv := iniciar(t, map[string]interface{}{
		"harness_args": []interface{}{"--pure", "--log-level", "DEBUG"},
		"effort":       "high", "agent": "build", "continue": true, "fork": true,
		"files": []string{"/tmp/a.txt"},
	})
	events := rodar(t, o, "faça")
	got := lerArgv(t, argv)
	exp := []string{"run", "--format", "json", "--continue", "--fork", "-m", "p/m", "--variant", "high", "--agent", "build", "--file", "/tmp/a.txt", "--pure", "--log-level", "DEBUG", "--", "faça"}
	if strings.Join(got, "|") != strings.Join(exp, "|") {
		t.Fatalf("argv:\n got %q\nwant %q", got, exp)
	}
	var raws []protocol.RawParams
	for _, e := range events {
		if r, ok := e.Payload.(protocol.RawParams); ok && e.Type == harness.EventRaw {
			raws = append(raws, r)
		}
	}
	var sawStdout, sawStderr bool
	for _, r := range raws {
		if r.Stream == "stdout" && strings.Contains(r.Line, "step_start") {
			sawStdout = true
		}
		if r.Stream == "stderr" && r.Line == "aviso do stderr" {
			sawStderr = true
		}
	}
	if !sawStdout || !sawStderr {
		t.Fatalf("raw ausente: %+v", raws)
	}
}

func TestPonteOpenCodeSlashViraCommand(t *testing.T) {
	o, argv := iniciar(t, map[string]interface{}{"harness_args": []string{"--pure"}})
	rodar(t, o, "/review a b")
	got := lerArgv(t, argv)
	exp := []string{"run", "--format", "json", "-m", "p/m", "--command", "review", "--pure", "--", "a b"}
	if strings.Join(got, "|") != strings.Join(exp, "|") {
		t.Fatalf("argv: %q", got)
	}
	// Sem argumentos: só --command, sem mensagem.
	rodar(t, o, "/review")
	got = lerArgv(t, argv)
	if got[len(got)-1] != "--pure" || got[len(got)-2] != "review" {
		t.Fatalf("argv sem mensagem: %q", got)
	}
}

func TestPonteOpenCodeTraducoesESemEquivalente(t *testing.T) {
	o, argv := iniciar(t, nil)
	ev := rodar(t, o, "/model deepseek/chat")
	if len(ev) != 2 || ev[0].Type != harness.EventText {
		t.Fatalf("eventos do /model: %v", ev)
	}
	if _, err := os.Stat(argv); err == nil {
		t.Fatal("/model não deve executar o opencode")
	}
	rodar(t, o, "olá")
	if got := lerArgv(t, argv); got[4] != "deepseek/chat" {
		t.Fatalf("modelo não mudou: %q", got)
	}
	// /new esquece a sessão (ses_1 aprendida na chamada anterior).
	rodar(t, o, "de novo")
	if got := lerArgv(t, argv); got[3] != "--session" {
		t.Fatalf("deveria retomar ses_1: %q", got)
	}
	rodar(t, o, "/new")
	rodar(t, o, "outra")
	if got := lerArgv(t, argv); strings.Contains(strings.Join(got, "|"), "--session") {
		t.Fatalf("sessão não foi esquecida: %q", got)
	}
	err := o.SendPrompt(context.Background(), "/exit", nil)
	var ne *harness.NoEquivalentError
	if !errors.As(err, &ne) || !strings.Contains(err.Error(), "/exit") {
		t.Fatalf("esperava NoEquivalent: %v", err)
	}
}
