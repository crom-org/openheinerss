package harness

import (
	"context"
	"fmt"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustomHerdarESobrescrever(t *testing.T) {
	if err := RegisterCustom(CustomSpec{Name: "base-teste", Command: "sh", Args: []string{"-c", "cat"}, Env: map[string]string{"A": "1", "B": "2"}}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCustom(CustomSpec{Name: "filho-teste", Base: "base-teste", Args: []string{"-c", "cat"}, Env: map[string]string{"B": "novo", "C": "3"}}); err != nil {
		t.Fatal(err)
	}
	customMu.RLock()
	got := customSpecs["filho-teste"]
	customMu.RUnlock()
	if got.Env["A"] != "1" || got.Env["B"] != "novo" || got.Env["C"] != "3" {
		t.Fatalf("merge de env: %#v", got.Env)
	}
}

func TestCustomNDJSONFake(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nread prompt\nprintf '%s\\n' '{\"type\":\"text\",\"text\":\"OK\"}'\nprintf '%s\\n' '{\"type\":\"usage\",\"totalTokens\":3}'\nprintf '%s\\n' '{\"type\":\"end\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	name := "fake-ndjson-" + filepath.Base(dir)
	if err := RegisterCustom(CustomSpec{Name: name, Command: script}); err != nil {
		t.Fatal(err)
	}
	h, _ := Create(name, ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.Start(ctx, SessionConfig{SessionID: "s", CWD: dir}); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := h.SendPrompt(ctx, "oi", nil); err != nil {
		t.Fatal(err)
	}
	seenText, seenUsage, seenEnd := false, false, false
	deadline := time.After(2 * time.Second)
	for !seenEnd {
		select {
		case e := <-h.Events():
			switch e.Type {
			case EventText:
				seenText = true
			case EventUsage:
				seenUsage = true
			case EventComplete:
				seenEnd = true
			}
		case <-deadline:
			t.Fatal("fake NDJSON não concluiu")
		}
	}
	if !seenText || !seenUsage {
		t.Fatalf("eventos: texto=%v uso=%v", seenText, seenUsage)
	}
}

func TestCustomErrosClaros(t *testing.T) {
	if err := RegisterCustom(CustomSpec{Name: "sem-base", Base: "nao-existe"}); err == nil || !contains(err.Error(), "não existe") {
		t.Fatalf("erro de base: %v", err)
	}
	if err := RegisterCustom(CustomSpec{Name: "prompt-invalido", Command: "sh", Prompt: "arquivo"}); err == nil || !contains(err.Error(), "stdin ou argument") {
		t.Fatalf("erro de prompt: %v", err)
	}
}

func TestLoadCustomFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instancia.yaml")
	if err := os.WriteFile(path, []byte("name: instancia-arquivo\ncommand: sh\nargs: [-c, 'printf \\\"{\\\\\"type\\\\\":\\\\\"end\\\\\"}\\\\n\\\"']\nprompt: stdin\nmodelo: modelo-teste\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadCustomFile(path); err != nil {
		t.Fatal(err)
	}
	customMu.RLock()
	spec := customSpecs["instancia-arquivo"]
	customMu.RUnlock()
	if spec.Model != "modelo-teste" {
		t.Fatalf("modelo não carregado: %+v", spec)
	}
}

func TestCustomAceitaModoSDKEErroRegexEmPortugues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instancia-sdk.yaml")
	conteudo := "name: instancia-sdk\nbase: claude-code\nmodo: sdk\nerro_regex: 'ServiceUnavailableError|429'\n"
	if err := os.WriteFile(path, []byte(conteudo), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadCustomFile(path); err != nil {
		t.Fatal(err)
	}
	s, ok := CustomSpecFor("instancia-sdk")
	if !ok || s.Mode != "sdk" || s.ErrorRegex != "ServiceUnavailableError|429" {
		t.Fatalf("instância efetiva: %+v", s)
	}
	h, err := Create("instancia-sdk", ModeSDK)
	if err != nil {
		t.Fatal(err)
	}
	if h.Mode() != ModeSDK {
		t.Fatalf("modo do harness: %s", h.Mode())
	}
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func TestExpandHomeNoEnv(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := expandHome("~/.codex-compartilhado"); got != home+"/.codex-compartilhado" {
		t.Fatalf("expandHome: %s", got)
	}
	if got := expandHome("/abs/~x"); got != "/abs/~x" {
		t.Fatalf("expandHome mexeu em caminho absoluto: %s", got)
	}
}

// coletaFim lê eventos até o fim e devolve o motivo e as mensagens de erro.
func coletaFim(t *testing.T, h Harness) (string, []string) {
	t.Helper()
	var erros []string
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-h.Events():
			switch p := e.Payload.(type) {
			case protocol.ErrorParams:
				erros = append(erros, p.Message)
			case protocol.CompleteParams:
				return p.Reason, erros
			}
		case <-deadline:
			t.Fatal("sem evento de fim")
		}
	}
}

func rodaScript(t *testing.T, corpo string, spec CustomSpec) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+corpo), 0755); err != nil {
		t.Fatal(err)
	}
	spec.Name = "fake-" + strings.ReplaceAll(t.Name(), "/", "-")
	spec.Command = script
	if err := RegisterCustom(spec); err != nil {
		t.Fatal(err)
	}
	h, _ := Create(spec.Name, ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.Start(ctx, SessionConfig{SessionID: "s", CWD: dir}); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := h.SendPrompt(ctx, "oi", nil); err != nil {
		t.Fatal(err)
	}
	return coletaFim(t, h)
}

func TestCustomSaidaComErroNaoEhSucesso(t *testing.T) {
	motivo, erros := rodaScript(t, "echo falhou feio >&2\nexit 3\n", CustomSpec{})
	if motivo != "process_error" {
		t.Fatalf("motivo = %q", motivo)
	}
	if len(erros) != 1 || !strings.Contains(erros[0], "falhou feio") {
		t.Fatalf("erros = %v", erros)
	}
}

func TestCustomFimAntesDoCodigoDeSaidaFalho(t *testing.T) {
	motivo, erros := rodaScript(t, "printf '%s\\n' '{\"type\":\"end\"}'\nexit 3\n", CustomSpec{})
	if motivo != "process_error" || len(erros) == 0 {
		t.Fatalf("fim seguido de código 3: motivo=%q erros=%v", motivo, erros)
	}
}

func TestCustomTabelaSubstituiPromptExato(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "args.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\"\nprintf '%s\\n' '{\\\"type\\\":\\\"end\\\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	name := "args-custom-" + filepath.Base(dir)
	if err := RegisterCustom(CustomSpec{Name: name, Command: script, Prompt: "argument", Args: []string{"--prompt={{prompt}}"}}); err != nil {
		t.Fatal(err)
	}
	h, _ := Create(name, ModeCLI)
	if err := h.Start(context.Background(), SessionConfig{SessionID: "args", CWD: dir}); err != nil {
		t.Fatal(err)
	}
	if err := h.SendPrompt(context.Background(), "--- prompt exato", nil); err != nil {
		t.Fatal(err)
	}
	var seen bool
	for deadline := time.After(2 * time.Second); !seen; {
		select {
		case e := <-h.Events():
			if p, ok := e.Payload.(protocol.TextParams); ok && strings.Contains(p.Delta, "--prompt=--- prompt exato") {
				seen = true
			}
		case <-deadline:
			t.Fatal("prompt custom não chegou nos args")
		}
	}
	_ = h.Stop()
}

func TestCustomCotaNoStderr(t *testing.T) {
	motivo, erros := rodaScript(t, "echo 'You hit the limit, resets 9am' >&2\nexit 1\n", CustomSpec{QuotaRegex: "hit the limit"})
	if motivo != "process_error" || len(erros) != 1 || !strings.HasPrefix(erros[0], "limite de cota detectado") {
		t.Fatalf("motivo %q erros %v", motivo, erros)
	}
}

func TestCustomEnvExpandeHomeSemBase(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sem HOME")
	}
	if err := RegisterCustom(CustomSpec{Name: "env-home-teste", Command: "sh", Env: map[string]string{"CONFIG": "~/conta"}}); err != nil {
		t.Fatal(err)
	}
	s, _ := CustomSpecFor("env-home-teste")
	if s.Env["CONFIG"] != home+"/conta" {
		t.Fatalf("env = %v", s.Env)
	}
}

func TestCustomNaoDeixaZumbi(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\necho '{\"type\":\"end\"}'\n"), 0755)
	if err := RegisterCustom(CustomSpec{Name: "zumbi-teste", Command: script}); err != nil {
		t.Fatal(err)
	}
	h, _ := Create("zumbi-teste", ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = h.Start(ctx, SessionConfig{SessionID: "s", CWD: dir})
	_ = h.SendPrompt(ctx, "oi", nil)
	coletaFim(t, h)
	c := h.(*customHarness)
	time.Sleep(100 * time.Millisecond)
	c.mu.Lock()
	state := c.cmd.ProcessState
	c.mu.Unlock()
	if state == nil {
		t.Fatal("processo não foi colhido (Wait não chamado)")
	}
}

func TestCustomStopMataOsFilhosDoProcesso(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "filho.pid")
	script := filepath.Join(dir, "pai.sh")
	body := "#!/bin/sh\ncat >/dev/null\nsleep 60 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCustom(CustomSpec{Name: "pai-filho-teste", Command: script}); err != nil {
		t.Fatal(err)
	}
	h, _ := Create("pai-filho-teste", ModeCLI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = h.Start(ctx, SessionConfig{SessionID: "s", CWD: dir})
	if err := h.SendPrompt(ctx, "oi", nil); err != nil {
		t.Fatal(err)
	}
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		if b, err := os.ReadFile(pidFile); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
		}
	}
	if pid == 0 {
		t.Fatal("o filho não registrou o PID")
	}
	cancel() // mesmo caminho do timeout do rodar
	_ = h.Stop()
	for i := 0; i < 50; i++ {
		if !processAliveForTest(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = killForTest(pid)
	t.Fatalf("o filho %d sobrou depois do Stop", pid)
}

func TestOpcoesRetomadaPorBase(t *testing.T) {
	casos := map[string]string{"claude-code": "claude_session_id", "codex": "codex_session_id", "opencode": "opencode_session_id", "agy": "conversation"}
	for base, chave := range casos {
		o, err := OpcoesRetomada(base, "abc")
		if err != nil || o[chave] != "abc" {
			t.Errorf("%s: %v %v", base, o, err)
		}
	}
	if _, err := OpcoesRetomada("aider", "abc"); err == nil {
		t.Error("aider deveria recusar")
	}
	if _, err := OpcoesRetomada("codex", ""); err == nil {
		t.Error("id vazio deveria falhar")
	}
}
