package codex

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// fakeCodex instala um "codex" falso no PATH que grava argv (um por linha) e o conteúdo dos arquivos de -i.
func fakeCodex(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"for a in \"$@\"; do case \"$a\" in --image=*) cat \"${a#--image=}\" > \"" + dir + "/img.bin\";; esac; done\n" +
		"echo '{\"type\":\"thread.started\",\"thread_id\":\"th-1\"}'\n" +
		"echo '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"oi\"}}'\n" +
		"echo 'aviso' >&2\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func iniciar(t *testing.T, cfg harness.SessionConfig) *CodexHarness {
	t.Helper()
	cfg.SessionID = "sess"
	cfg.CWD = t.TempDir()
	h := NewCodexHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop() })
	return h
}

func coletar(t *testing.T, h *CodexHarness) []harness.Event {
	t.Helper()
	var evs []harness.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-h.Events():
			evs = append(evs, ev)
			if ev.Type == harness.EventComplete {
				return evs
			}
		case <-timeout:
			t.Fatalf("sem complete; eventos: %+v", evs)
		}
	}
}

func argvDe(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestPonteCodexArgsOrdemRawEImagem(t *testing.T) {
	dir := fakeCodex(t)
	h := iniciar(t, harness.SessionConfig{Model: "o3", Options: map[string]interface{}{
		"effort": "high", "sandbox": "read-only", "profile": "p1", "config": []string{"a=1", "b=\"x\""},
		"harness_args": []string{"--skip-git-repo-check", "--color", "never"},
	}})
	img := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	if err := h.SendPrompt(context.Background(), "faça", []protocol.Attachment{{MediaType: "image/png", Data: img}}); err != nil {
		t.Fatal(err)
	}
	evs := coletar(t, h)
	argv := argvDe(t, dir)
	// o arquivo temporário tem nome aleatório: confere o prefixo e normaliza.
	var imgArg string
	for i, a := range argv {
		if strings.HasPrefix(a, "--image=") {
			imgArg = a
			argv[i] = "--image=<tmp>"
		}
	}
	want := []string{"exec", "--json", "-m", "o3", "-c", "model_reasoning_effort=high", "--sandbox", "read-only",
		"--profile", "p1", "-c", "a=1", "-c", "b=\"x\"", "--image=<tmp>", "--skip-git-repo-check", "--color", "never", "--", "faça"}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv: %q", argv)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "img.bin")); string(data) != "PNGDATA" {
		t.Fatalf("conteúdo da imagem: %q", data)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(strings.TrimPrefix(imgArg, "--image=")); err == nil {
		t.Fatal("arquivo temporário da imagem não foi removido")
	}
	var rawOut, rawErr bool
	for _, ev := range evs {
		if r, ok := ev.Payload.(protocol.RawParams); ev.Type == harness.EventRaw && ok {
			rawOut = rawOut || (r.Stream == "stdout" && strings.Contains(r.Line, "thread.started"))
			rawErr = rawErr || (r.Stream == "stderr" && r.Line == "aviso")
		}
	}
	if !rawOut || !rawErr {
		t.Fatalf("raw ausente: %+v", evs)
	}
}

func TestPonteCodexRetomadaESlash(t *testing.T) {
	dir := fakeCodex(t)
	h := iniciar(t, harness.SessionConfig{Options: map[string]interface{}{"sandbox": "workspace-write"}})
	if err := h.SendPrompt(context.Background(), "um", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h)
	if h.ResumeID() != "th-1" {
		t.Fatalf("thread: %q", h.ResumeID())
	}
	if err := h.SendPrompt(context.Background(), "/model o4", nil); err != nil {
		t.Fatal(err)
	}
	if evs := coletar(t, h); evs[0].Type != harness.EventText {
		t.Fatalf("%+v", evs)
	}
	_ = os.Remove(filepath.Join(dir, "argv.txt"))
	if err := h.SendPrompt(context.Background(), "dois", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h)
	got := strings.Join(argvDe(t, dir), " ")
	want := "exec resume --json -m o4 -c model_reasoning_effort=medium -c sandbox_mode=\"workspace-write\" th-1 -- dois"
	if got != want {
		t.Fatalf("argv: %s", got)
	}
	// /new esquece a thread.
	if err := h.SendPrompt(context.Background(), "/new", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h)
	if h.ResumeID() != "" {
		t.Fatalf("thread não esquecida: %q", h.ResumeID())
	}
}

func TestPonteCodexSlashSemEquivalente(t *testing.T) {
	dir := fakeCodex(t)
	h := iniciar(t, harness.SessionConfig{})
	for _, cmd := range []string{"/compact", "/review algo", "/status"} {
		err := h.SendPrompt(context.Background(), cmd, nil)
		var ne *harness.NoEquivalentError
		if !errors.As(err, &ne) {
			t.Fatalf("%s: esperava NoEquivalent, veio %v", cmd, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "argv.txt")); err == nil {
		t.Fatal("o processo não devia ter sido chamado")
	}
}
