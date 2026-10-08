package server

import (
	"net/http"
	"testing"
)

func TestOrigemPermitida(t *testing.T) {
	t.Setenv("OPENHEINERSS_ORIGENS", "https://central.exemplo.org")
	casos := map[string]bool{
		"":                            true,
		"http://localhost:3000":       true,
		"http://127.0.0.1:5173":       true,
		"tauri://localhost":           true,
		"https://central.exemplo.org": true,
		"https://site-mal.example":    false,
		"null":                        false,
	}
	for origem, quer := range casos {
		r, _ := http.NewRequest("GET", "http://127.0.0.1:4820/", nil)
		if origem != "" {
			r.Header.Set("Origin", origem)
		}
		if got := origemPermitida(r); got != quer {
			t.Errorf("origem %q: %v, esperado %v", origem, got, quer)
		}
	}
	t.Setenv("OPENHEINERSS_ORIGENS", "*")
	r, _ := http.NewRequest("GET", "http://127.0.0.1:4820/", nil)
	r.Header.Set("Origin", "https://qualquer.example")
	if !origemPermitida(r) {
		t.Error("* deveria liberar tudo")
	}
}
