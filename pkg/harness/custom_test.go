package harness

import (
	"context"
	"os"
	"path/filepath"
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

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
