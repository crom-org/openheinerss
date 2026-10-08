package codex

import (
	"os/exec"
	"testing"
)

func TestConfiguraGrupoDeProcessosParaEncerrarFilhos(t *testing.T) {
	cmd := exec.Command("true")
	configureProcessGroup(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("o codex deve executar em grupo próprio para o timeout encerrar filhos")
	}
}
