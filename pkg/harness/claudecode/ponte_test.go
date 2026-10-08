package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// fakeClaude instala um "claude" falso no PATH que grava o argv (um por linha) e imprime linhas do stream.
func fakeClaude(t *testing.T) (argvFile string) {
	t.Helper()
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"echo '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s1\"}'\n" +
		"echo '{\"type\":\"coisa_nova\",\"x\":1}'\n" +
		"echo 'aviso no stderr' >&2\n" +
		"echo '{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"s1\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

func coletar(t *testing.T, h *ClaudeCodeHarness, ate harness.EventType) []harness.Event {
	t.Helper()
	var evs []harness.Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-h.Events():
			evs = append(evs, ev)
			if ev.Type == ate {
				return evs
			}
		case <-timeout:
			t.Fatalf("sem %s; eventos: %+v", ate, evs)
		}
	}
}

func iniciar(t *testing.T, cfg harness.SessionConfig) *ClaudeCodeHarness {
	t.Helper()
	cfg.SessionID = "sess"
	cfg.CWD = t.TempDir()
	h := NewClaudeCodeHarness(harness.ModeCLI)
	if err := h.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop() })
	return h
}

func lerArgv(t *testing.T, f string) []string {
	t.Helper()
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestPonteHarnessArgsTipadasERawDoClaude(t *testing.T) {
	argv := fakeClaude(t)
	h := iniciar(t, harness.SessionConfig{
		SystemPrompt: "seja breve",
		Options: map[string]interface{}{
			"effort": "high", "add_dirs": []string{"/a", "/b"}, "mcp_config": "m.json",
			"allowed_tools": []string{"Bash", "Read"}, "disallowed_tools": "Edit",
			"harness_args": []interface{}{"--bare", "--name", "x y", "--settings", "{\"a\":1}"},
		},
	})
	// "/compact" segue literal: é do claude, não da ponte.
	if err := h.SendPrompt(context.Background(), "/compact foco", nil); err != nil {
		t.Fatal(err)
	}
	evs := coletar(t, h, harness.EventComplete)
	// O stderr é lido em paralelo e pode chegar depois do complete do "result".
	espera := time.After(3 * time.Second)
	for temStderr := false; !temStderr; {
		for _, ev := range evs {
			if r, ok := ev.Payload.(protocol.RawParams); ok && r.Stream == "stderr" {
				temStderr = true
			}
		}
		if temStderr {
			break
		}
		select {
		case ev := <-h.Events():
			evs = append(evs, ev)
		case <-espera:
			temStderr = true
		}
	}
	got := strings.Join(lerArgv(t, argv), "\x00")
	want := strings.Join([]string{"--print", "--output-format", "stream-json", "--verbose",
		"--effort", "high", "--append-system-prompt", "seja breve", "--add-dir", "/a", "--add-dir", "/b",
		"--mcp-config", "m.json", "--allowed-tools", "Bash,Read", "--disallowed-tools", "Edit",
		"--bare", "--name", "x y", "--settings", "{\"a\":1}", "--", "/compact foco"}, "\x00")
	if got != want {
		t.Fatalf("argv:\n%q\nquero:\n%q", strings.Split(got, "\x00"), strings.Split(want, "\x00"))
	}
	var raws []protocol.RawParams
	for _, ev := range evs {
		if r, ok := ev.Payload.(protocol.RawParams); ok && ev.Type == harness.EventRaw {
			raws = append(raws, r)
		}
	}
	var coisa, aviso bool
	for _, r := range raws {
		coisa = coisa || (r.Stream == "stdout" && strings.Contains(r.Line, "coisa_nova"))
		aviso = aviso || (r.Stream == "stderr" && r.Line == "aviso no stderr")
	}
	if !coisa || !aviso {
		t.Fatalf("raw sem stdout/stderr esperados: %+v", raws)
	}
}

func TestPonteModelEEffortSaoTraduzidosSemChamarProcesso(t *testing.T) {
	argv := fakeClaude(t)
	h := iniciar(t, harness.SessionConfig{})
	for _, cmd := range []string{"/model opus", "/effort max"} {
		if err := h.SendPrompt(context.Background(), cmd, nil); err != nil {
			t.Fatal(err)
		}
		evs := coletar(t, h, harness.EventComplete)
		if len(evs) != 2 || evs[0].Type != harness.EventText {
			t.Fatalf("%s: eventos %+v", cmd, evs)
		}
	}
	if _, err := os.Stat(argv); err == nil {
		t.Fatal("o processo não devia ter sido chamado")
	}
	if err := h.SendPrompt(context.Background(), "oi", nil); err != nil {
		t.Fatal(err)
	}
	coletar(t, h, harness.EventComplete)
	got := strings.Join(lerArgv(t, argv), " ")
	if !strings.Contains(got, "--model opus") || !strings.Contains(got, "--effort max") {
		t.Fatalf("argv: %s", got)
	}
}

func TestPonteContinueSemRetomada(t *testing.T) {
	got := buildCLIArgs(harness.SessionConfig{Options: map[string]interface{}{"continue": true}}, "", "", "p")
	if got[0] != "--continue" {
		t.Fatalf("%v", got)
	}
}

func TestPonteSDKExtras(t *testing.T) {
	ex := sdkExtras(harness.SessionConfig{SystemPrompt: "s", Options: map[string]interface{}{
		"effort": "low", "harness_args": []string{"--bare", "--name", "n", "--x=y"}, "add_dirs": []string{"/d"}}})
	extra := ex["extraArgs"].(map[string]interface{})
	if extra["effort"] != "low" || extra["name"] != "n" || extra["x"] != "y" {
		t.Fatalf("%v", extra)
	}
	if v, ok := extra["bare"]; !ok || v != nil {
		t.Fatalf("bare: %v", extra)
	}
	if ex["appendSystemPrompt"] != "s" || ex["additionalDirectories"] == nil {
		t.Fatalf("%v", ex)
	}
}
