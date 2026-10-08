package doctor

import (
	"strings"
	"testing"
)

func TestRelatorioDoDoctorTemResumoEGit(t *testing.T) {
	res := CheckEnvironment("")
	out := FormatDoctorReport(res)
	if !strings.Contains(out, "Git") || !strings.Contains(out, "Resumo") {
		t.Fatalf("relatório inesperado:\n%s", out)
	}
}
