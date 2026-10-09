package orchestrator

import "testing"

func TestErroPastaExternaExtraiCaminhoDoRawDoOpenCode(t *testing.T) {
	line := "permission requested: external_directory (/home/j/fora/*); auto-rejecting"
	if !erroPastaExterna(line) {
		t.Fatal("raw do OpenCode não foi reconhecido")
	}
	if got := pastaNoErro(line); got != "/home/j/fora/*" {
		t.Fatalf("caminho=%q", got)
	}
}
