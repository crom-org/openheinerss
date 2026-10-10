package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPastasPermitidasMesclaExpandeECano(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := t.TempDir()
	ext := filepath.Join(repo, "externa")
	if err := os.MkdirAll(ext, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, WorkspaceDirName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, WorkspaceDirName, ConfigFileName), []byte("pastas_permitidas:\n  - ~/liberada\n  - "+ext+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := PastasPermitidasEfetivas(repo, []string{ext, "~/outra"})
	if err != nil {
		t.Fatal(err)
	}
	extCanon, err := filepath.EvalSymlinks(ext)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("pastas=%v", got)
	}
	for _, want := range []string{filepath.Join(home, "liberada"), extCanon, filepath.Join(home, "outra")} {
		found := false
		for _, p := range got {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("não encontrou %q em %v", want, got)
		}
	}
}

func TestPastasLeituraEfetivasInclui(t *testing.T) {
	d := t.TempDir()
	got, err := PastasLeituraEfetivas([]string{d})
	if err != nil {
		t.Fatal(err)
	}
	dr, _ := filepath.EvalSymlinks(d)
	_, errProc := os.Stat("/proc")
	proc := errProc != nil // sem /proc (macOS/Windows) não há o que conferir
	var extra, home bool
	for _, p := range got {
		switch {
		case p == "/proc":
			proc = true
		case p == d || p == dr:
			extra = true
		}
		if h, _ := os.UserHomeDir(); h != "" && strings.HasPrefix(p, filepath.Join(h, ".openheinerss")) {
			home = true
		}
	}
	if !proc || !extra || home {
		t.Fatalf("proc=%v extra=%v home=%v lista=%v", proc, extra, home, got)
	}
}
